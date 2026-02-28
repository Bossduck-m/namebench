// The ui package contains methods for handling UI URL's.
package ui

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/namebench/dnschecks"
	"github.com/google/namebench/dnsqueue"
	"github.com/google/namebench/history"
)

const (
	// How many requests/responses can be queued at once
	QUEUE_LENGTH = 65535

	// Number of workers (same as Chrome's DNS prefetch queue)
	WORKERS = 8

	// Number of tests to run
	COUNT = 50

	// How far back to reach into browser history
	HISTORY_DAYS = 30
)

var (
	indexTmpl = loadTemplate("ui/templates/index.html")
)

// RegisterHandler registers all known handlers.
func RegisterHandlers() {
	http.HandleFunc("/", Index)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("ui/static"))))
	http.HandleFunc("/submit", Submit)
	http.HandleFunc("/dnssec", DnsSec)
}

// loadTemplate loads a set of templates.
func loadTemplate(paths ...string) *template.Template {
	t := template.New(strings.Join(paths, ","))
	_, err := t.ParseFiles(paths...)
	if err != nil {
		panic(err)
	}
	return t
}

// Index handles /
func Index(w http.ResponseWriter, r *http.Request) {
	if err := indexTmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	return
}

// DnsSec handles /dnssec
func DnsSec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	servers := []string{
		"8.8.8.8:53",
		"75.75.75.75:53",
		"4.2.2.1:53",
		"208.67.222.222:53",
	}
	for _, ip := range servers {
		result, err := dnschecks.DnsSec(ip)
		log.Printf("%s DNSSEC: %t (err=%v)", ip, result, err)
		if _, writeErr := fmt.Fprintf(w, "%s dnssec=%t err=%v\n", ip, result, err); writeErr != nil {
			log.Printf("failed to write dnssec response: %v", writeErr)
			return
		}
	}
}

// Submit handles /submit
func Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	queryCount := parsePositiveInt(r.FormValue("query_count"), COUNT)
	nameServers := parseNameServers(r.FormValue("nameservers"))
	targetServer := "8.8.8.8:53"
	if len(nameServers) > 0 {
		targetServer = nameServers[0]
	}

	records, err := history.Chrome(HISTORY_DAYS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	q := dnsqueue.StartQueue(QUEUE_LENGTH, WORKERS)
	hostnames := history.Random(queryCount, history.Uniq(history.ExternalHostnames(records)))
	if len(hostnames) == 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintln(w, "no eligible hostnames found in browsing history")
		return
	}

	for _, record := range hostnames {
		q.Add(targetServer, "A", record+".")
		log.Printf("Added %s", record)
	}
	q.SendCompletionSignal()
	answered := 0
	failures := 0
	for {
		if answered == len(hostnames) {
			break
		}
		result := <-q.Results
		answered += 1
		if result.Error != "" {
			failures += 1
		}
		log.Printf("%+v", result)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "server=%s benchmarked=%d failures=%d\n", targetServer, len(hostnames), failures)
	return
}

func parsePositiveInt(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 2000 {
		return 2000
	}
	return n
}

func parseNameServers(raw string) []string {
	if raw == "" {
		return nil
	}

	chunks := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	servers := make([]string, 0, len(chunks))
	seen := make(map[string]bool, len(chunks))

	for _, chunk := range chunks {
		server, ok := normalizeNameServer(chunk)
		if !ok || seen[server] {
			continue
		}
		seen[server] = true
		servers = append(servers, server)
	}
	return servers
}

func normalizeNameServer(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	// bare IPv4/IPv6 address without explicit port
	if ip := net.ParseIP(raw); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), true
	}

	if !strings.Contains(raw, ":") {
		raw += ":53"
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || host == "" || port == "" {
		return "", false
	}
	if net.ParseIP(host) == nil {
		return "", false
	}
	return net.JoinHostPort(host, port), true
}
