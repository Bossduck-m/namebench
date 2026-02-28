// The ui package contains methods for handling UI URL's.
package ui

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
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
	//go:embed templates static
	uiAssets embed.FS

	indexTmpl = loadTemplate("templates/index.html")

	defaultDnsSecServers = []string{
		"8.8.8.8:53",
		"75.75.75.75:53",
		"4.2.2.1:53",
		"208.67.222.222:53",
	}

	globalResolverPool = []string{
		"8.8.8.8:53",
		"8.8.4.4:53",
		"1.1.1.1:53",
		"1.0.0.1:53",
		"9.9.9.9:53",
		"208.67.222.222:53",
	}

	defaultRegionalResolverPool = []string{
		"64.6.64.6:53",
		"64.6.65.6:53",
		"76.76.2.0:53",
	}

	regionalResolverPoolByLocation = map[string][]string{
		"us": {
			"64.6.64.6:53",
			"64.6.65.6:53",
			"76.76.2.0:53",
			"76.76.10.0:53",
		},
		"eu": {
			"185.228.168.9:53",
			"185.228.169.9:53",
			"9.9.9.11:53",
		},
		"tr": {
			"9.9.9.10:53",
			"149.112.112.10:53",
			"1.1.1.2:53",
		},
	}
)

type latencyBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type serverBenchmark struct {
	Rank           int             `json:"rank"`
	Server         string          `json:"server"`
	Queries        int             `json:"queries"`
	Successes      int             `json:"successes"`
	Failures       int             `json:"failures"`
	FailureRate    float64         `json:"failure_rate"`
	AvgMs          float64         `json:"avg_ms"`
	MedianMs       float64         `json:"median_ms"`
	P95Ms          float64         `json:"p95_ms"`
	MinMs          float64         `json:"min_ms"`
	MaxMs          float64         `json:"max_ms"`
	Score          float64         `json:"score"`
	LatencyBuckets []latencyBucket `json:"latency_buckets"`
}

type benchmarkResponse struct {
	RequestedQueries int               `json:"requested_queries"`
	ExecutedQueries  int               `json:"executed_queries"`
	ServerCount      int               `json:"server_count"`
	Winner           string            `json:"winner"`
	Results          []serverBenchmark `json:"results"`
	Warnings         []string          `json:"warnings,omitempty"`
}

type requestConfig struct {
	QueryCount      int    `json:"query_count"`
	IncludeGlobal   bool   `json:"include_global"`
	IncludeRegional bool   `json:"include_regional"`
	Location        string `json:"location"`
	Nameservers     string `json:"nameservers"`
}

// RegisterHandler registers all known handlers.
func RegisterHandlers() {
	staticFS, err := fs.Sub(uiAssets, "static")
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/", Index)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/submit", Submit)
	http.HandleFunc("/dnssec", DnsSec)
}

// loadTemplate loads a set of templates.
func loadTemplate(paths ...string) *template.Template {
	t := template.New(strings.Join(paths, ","))
	_, err := t.ParseFS(uiAssets, paths...)
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
}

// DnsSec handles /dnssec
func DnsSec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	cfg, err := parseRequestConfig(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nameServers := parseNameServers(cfg.Nameservers)
	includeGlobal := cfg.IncludeGlobal
	includeRegional := cfg.IncludeRegional
	location := cfg.Location

	servers := []string{}
	if r.Method == http.MethodPost {
		servers = mergeNameServers(nameServers, includeGlobal, includeRegional, location)
	} else {
		servers = defaultDnsSecServers
	}
	if len(servers) == 0 {
		servers = defaultDnsSecServers
	}

	if _, err := fmt.Fprintf(w, "checked_servers=%d\n", len(servers)); err != nil {
		log.Printf("failed to write dnssec response header: %v", err)
		return
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
	cfg, err := parseRequestConfig(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	queryCount := cfg.QueryCount
	includeGlobal := cfg.IncludeGlobal
	includeRegional := cfg.IncludeRegional
	location := cfg.Location

	rawNameservers := cfg.Nameservers
	nameServers := parseNameServers(rawNameservers)
	candidateServers := mergeNameServers(nameServers, includeGlobal, includeRegional, location)
	warnings := make([]string, 0, 2)
	if rawNameservers == "" {
		warnings = append(warnings, "No manual nameserver provided, using default provider pools.")
	} else if len(nameServers) == 0 {
		warnings = append(warnings, "Manual nameservers could not be parsed. Use one IPv4/IPv6 per line.")
	}

	records, err := history.Chrome(HISTORY_DAYS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hostnames := history.Random(queryCount, history.Uniq(history.ExternalHostnames(records)))
	if len(hostnames) == 0 {
		warnings = append(warnings, "No eligible hostnames found in browsing history.")
		writeJSON(w, http.StatusOK, benchmarkResponse{
			RequestedQueries: queryCount,
			ExecutedQueries:  0,
			ServerCount:      len(candidateServers),
			Winner:           "",
			Results:          []serverBenchmark{},
			Warnings:         warnings,
		})
		return
	}

	results := benchmarkAllServers(candidateServers, hostnames)
	winner := selectWinner(results)
	if winner == "" && len(results) > 0 {
		winner = results[0].Server
		warnings = append(warnings, "No server returned successful answers. Ranking falls back to penalty score.")
	}

	writeJSON(w, http.StatusOK, benchmarkResponse{
		RequestedQueries: queryCount,
		ExecutedQueries:  len(hostnames),
		ServerCount:      len(candidateServers),
		Winner:           winner,
		Results:          results,
		Warnings:         warnings,
	})
}

func benchmarkAllServers(servers []string, hostnames []string) []serverBenchmark {
	results := make([]serverBenchmark, 0, len(servers))
	for _, server := range servers {
		result := benchmarkSingleServer(server, hostnames)
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Server < results[j].Server
		}
		return results[i].Score < results[j].Score
	})
	for i := range results {
		results[i].Rank = i + 1
	}
	return results
}

func benchmarkSingleServer(server string, hostnames []string) serverBenchmark {
	q := dnsqueue.StartQueue(QUEUE_LENGTH, WORKERS)
	for _, record := range hostnames {
		q.Add(server, "A", ensureFQDN(record))
	}
	q.SendCompletionSignal()

	latencies := make([]float64, 0, len(hostnames))
	failures := 0
	for i := 0; i < len(hostnames); i++ {
		result := <-q.Results
		if result.Error != "" || len(result.Answers) == 0 {
			failures++
			continue
		}
		ms := float64(result.Duration.Microseconds()) / 1000.0
		if ms < 0 {
			ms = 0
		}
		latencies = append(latencies, ms)
	}

	avgMs, medianMs, p95Ms, minMs, maxMs := summarizeLatencies(latencies)
	queries := len(hostnames)
	successes := len(latencies)
	failureRate := 0.0
	if queries > 0 {
		failureRate = float64(failures) / float64(queries)
	}

	return serverBenchmark{
		Server:         server,
		Queries:        queries,
		Successes:      successes,
		Failures:       failures,
		FailureRate:    round2(failureRate),
		AvgMs:          avgMs,
		MedianMs:       medianMs,
		P95Ms:          p95Ms,
		MinMs:          minMs,
		MaxMs:          maxMs,
		Score:          scoreServer(avgMs, p95Ms, failures, queries),
		LatencyBuckets: buildLatencyBuckets(latencies),
	}
}

func summarizeLatencies(samples []float64) (avg, median, p95, min, max float64) {
	if len(samples) == 0 {
		return 0, 0, 0, 0, 0
	}

	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	avg = sum / float64(len(sorted))

	if len(sorted)%2 == 1 {
		median = sorted[len(sorted)/2]
	} else {
		mid := len(sorted) / 2
		median = (sorted[mid-1] + sorted[mid]) / 2
	}

	p95Index := int(math.Ceil(float64(len(sorted))*0.95)) - 1
	if p95Index < 0 {
		p95Index = 0
	}
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	p95 = sorted[p95Index]

	min = sorted[0]
	max = sorted[len(sorted)-1]
	return round2(avg), round2(median), round2(p95), round2(min), round2(max)
}

func buildLatencyBuckets(samples []float64) []latencyBucket {
	buckets := []latencyBucket{
		{Label: "0-20 ms", Count: 0},
		{Label: "20-40 ms", Count: 0},
		{Label: "40-80 ms", Count: 0},
		{Label: "80-160 ms", Count: 0},
		{Label: "160-320 ms", Count: 0},
		{Label: "320+ ms", Count: 0},
	}

	for _, ms := range samples {
		switch {
		case ms < 20:
			buckets[0].Count++
		case ms < 40:
			buckets[1].Count++
		case ms < 80:
			buckets[2].Count++
		case ms < 160:
			buckets[3].Count++
		case ms < 320:
			buckets[4].Count++
		default:
			buckets[5].Count++
		}
	}
	return buckets
}

func selectWinner(results []serverBenchmark) string {
	for _, result := range results {
		if result.Successes > 0 {
			return result.Server
		}
	}
	return ""
}

func scoreServer(avgMs, p95Ms float64, failures, queries int) float64 {
	if queries <= 0 {
		return 0
	}
	if failures >= queries {
		return 999999
	}
	failureRate := float64(failures) / float64(queries)
	score := avgMs + (0.18 * p95Ms) + (failureRate * 900)
	return round2(score)
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

func mergeNameServers(manual []string, includeGlobal, includeRegional bool, location string) []string {
	merged := make([]string, 0, len(manual)+8)
	seen := map[string]bool{}

	appendUnique := func(values []string) {
		for _, value := range values {
			server, ok := normalizeNameServer(value)
			if !ok || seen[server] {
				continue
			}
			seen[server] = true
			merged = append(merged, server)
		}
	}

	appendUnique(manual)
	if includeGlobal {
		appendUnique(globalResolverPool)
	}
	if includeRegional {
		if regional, ok := regionalResolverPoolByLocation[location]; ok {
			appendUnique(regional)
		} else {
			appendUnique(defaultRegionalResolverPool)
		}
	}
	if len(merged) == 0 {
		appendUnique([]string{"8.8.8.8:53"})
	}
	return merged
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

func formEnabled(r *http.Request, key string) bool {
	return strings.TrimSpace(r.FormValue(key)) != ""
}

func parseIncomingForm(r *http.Request) error {
	if err := r.ParseMultipartForm(4 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	return r.ParseForm()
}

func parseRequestConfig(r *http.Request) (requestConfig, error) {
	cfg := requestConfig{
		QueryCount: COUNT,
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			return requestConfig{}, err
		}
		cfg.QueryCount = parsePositiveInt(strconv.Itoa(cfg.QueryCount), COUNT)
		cfg.Location = strings.ToLower(strings.TrimSpace(cfg.Location))
		cfg.Nameservers = strings.TrimSpace(cfg.Nameservers)
		return cfg, nil
	}

	if err := parseIncomingForm(r); err != nil {
		return requestConfig{}, err
	}
	cfg.QueryCount = parsePositiveInt(r.FormValue("query_count"), COUNT)
	cfg.IncludeGlobal = formEnabled(r, "include_global")
	cfg.IncludeRegional = formEnabled(r, "include_regional")
	cfg.Location = strings.ToLower(strings.TrimSpace(r.FormValue("location")))
	cfg.Nameservers = strings.TrimSpace(r.FormValue("nameservers"))
	return cfg, nil
}

func ensureFQDN(record string) string {
	if strings.HasSuffix(record, ".") {
		return record
	}
	return record + "."
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
