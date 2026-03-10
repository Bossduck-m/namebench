// The ui package contains methods for handling UI URL's.
package ui

import (
	"context"
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
	"github.com/google/uuid"
	"github.com/miekg/dns"
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

	// Number of integrity probes per resolver.
	INTEGRITY_PROBE_COUNT = 3

	// Upper bound for resolvers benchmarked in a single run.
	MAX_RESOLVERS = 32
)

var (
	//go:embed templates static
	uiAssets embed.FS

	indexTmpl       = loadTemplate("templates/index.html")
	appRequestToken = uuid.NewString()

	defaultDnsSecServers = []string{
		"1.1.1.1:53",
		"1.0.0.1:53",
		"8.8.8.8:53",
		"8.8.4.4:53",
		"9.9.9.9:53",
		"149.112.112.112:53",
		"208.67.222.222:53",
		"208.67.220.220:53",
	}

	globalResolverPool = []string{
		// Cloudflare
		"1.1.1.1:53",
		"1.0.0.1:53",
		"2606:4700:4700::1111",
		"2606:4700:4700::1001",
		// Google Public DNS
		"8.8.8.8:53",
		"8.8.4.4:53",
		"2001:4860:4860::8888",
		"2001:4860:4860::8844",
		// Quad9
		"9.9.9.9:53",
		"149.112.112.112:53",
		"2620:fe::fe",
		"2620:fe::9",
		// OpenDNS
		"208.67.222.222:53",
		"208.67.220.220:53",
		"2620:119:35::35",
		"2620:119:53::53",
		// AdGuard
		"94.140.14.14:53",
		"94.140.15.15:53",
		// Control D
		"76.76.2.0:53",
		"76.76.10.0:53",
		// DNS.WATCH
		"84.200.69.80:53",
		"84.200.70.40:53",
		// Verisign
		"64.6.64.6:53",
		"64.6.65.6:53",
		// CleanBrowsing Security
		"185.228.168.9:53",
		"185.228.169.9:53",
	}

	defaultRegionalResolverPool = []string{
		"64.6.64.6:53",
		"64.6.65.6:53",
		"76.76.2.0:53",
		"76.76.10.0:53",
		"185.228.168.9:53",
		"185.228.169.9:53",
	}

	regionalResolverPoolByLocation = map[string][]string{
		"us": {
			"64.6.64.6:53",
			"64.6.65.6:53",
			"76.76.2.0:53",
			"76.76.10.0:53",
			"149.112.112.10:53",
		},
		"eu": {
			"185.228.168.9:53",
			"185.228.169.9:53",
			"193.110.81.0:53",
			"185.253.5.0:53",
			"9.9.9.11:53",
		},
		"tr": {
			"9.9.9.10:53",
			"149.112.112.10:53",
			"94.140.14.14:53",
			"94.140.15.15:53",
			"1.1.1.2:53",
		},
	}
)

type latencyBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type serverBenchmark struct {
	Rank                   int             `json:"rank"`
	Server                 string          `json:"server"`
	Queries                int             `json:"queries"`
	Successes              int             `json:"successes"`
	Failures               int             `json:"failures"`
	FailureRate            float64         `json:"failure_rate"`
	AvgMs                  float64         `json:"avg_ms"`
	MedianMs               float64         `json:"median_ms"`
	P95Ms                  float64         `json:"p95_ms"`
	MinMs                  float64         `json:"min_ms"`
	MaxMs                  float64         `json:"max_ms"`
	Score                  float64         `json:"score"`
	LatencyBuckets         []latencyBucket `json:"latency_buckets"`
	UncachedQueries        int             `json:"uncached_queries"`
	UncachedSuccesses      int             `json:"uncached_successes"`
	UncachedFailures       int             `json:"uncached_failures"`
	UncachedAvgMs          float64         `json:"uncached_avg_ms"`
	UncachedMedianMs       float64         `json:"uncached_median_ms"`
	UncachedP95Ms          float64         `json:"uncached_p95_ms"`
	UncachedLatencyBuckets []latencyBucket `json:"uncached_latency_buckets"`
	CachedQueries          int             `json:"cached_queries"`
	CachedSuccesses        int             `json:"cached_successes"`
	CachedFailures         int             `json:"cached_failures"`
	CachedAvgMs            float64         `json:"cached_avg_ms"`
	CachedMedianMs         float64         `json:"cached_median_ms"`
	CachedP95Ms            float64         `json:"cached_p95_ms"`
	CachedLatencyBuckets   []latencyBucket `json:"cached_latency_buckets"`
	JitterMs               float64         `json:"jitter_ms"`
	Integrity              string          `json:"integrity"`
	IntegrityDetail        string          `json:"integrity_detail,omitempty"`
	IntegrityProbeCount    int             `json:"integrity_probe_count"`
	IntegrityCleanCount    int             `json:"integrity_clean_count"`
	IntegrityErrorCount    int             `json:"integrity_error_count"`
	IntegrityAnomalyCount  int             `json:"integrity_anomaly_count"`
	ResolverIP             string          `json:"resolver_ip,omitempty"`
	ResolverASN            string          `json:"resolver_asn,omitempty"`
	ResolverASName         string          `json:"resolver_as_name,omitempty"`
	ResolverCountry        string          `json:"resolver_country,omitempty"`
	ResolverOrganization   string          `json:"resolver_organization,omitempty"`
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
	IncludeSystem   bool   `json:"include_system"`
	IncludeMetadata bool   `json:"include_metadata"`
	IncludeGlobal   bool   `json:"include_global"`
	IncludeRegional bool   `json:"include_regional"`
	HistoryConsent  bool   `json:"history_consent"`
	Location        string `json:"location"`
	Nameservers     string `json:"nameservers"`
	DataSource      string `json:"data_source"`
}

type benchmarkPassStats struct {
	Queries        int
	Successes      int
	Failures       int
	AvgMs          float64
	MedianMs       float64
	P95Ms          float64
	MinMs          float64
	MaxMs          float64
	LatencyBuckets []latencyBucket
	latencies      []float64
}

type benchmarkProgressCallback func(server, phase string, delta int)

type indexTemplateData struct {
	RequestToken string
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
	http.HandleFunc("/progress", Progress)
	http.HandleFunc("/cancel", Cancel)
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
	data := indexTemplateData{RequestToken: appRequestToken}
	if err := indexTmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// DnsSec handles /dnssec
func DnsSec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost && !validateRequestToken(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	cfg, err := parseRequestConfig(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nameServers := parseNameServers(cfg.Nameservers)
	systemServers := []string{}
	if cfg.IncludeSystem {
		systemServers, _ = discoverSystemResolvers()
	}
	includeGlobal := cfg.IncludeGlobal
	includeRegional := cfg.IncludeRegional
	location := cfg.Location

	servers := []string{}
	if r.Method == http.MethodPost {
		servers = mergeNameServers(nameServers, systemServers, includeGlobal, includeRegional, location)
	} else {
		servers = defaultDnsSecServers
	}
	if limited, truncated := limitResolvers(servers, MAX_RESOLVERS); truncated > 0 {
		servers = limited
		if _, err := fmt.Fprintf(w, "warning=resolver_list_truncated count=%d limit=%d\n", truncated, MAX_RESOLVERS); err != nil {
			log.Printf("failed to write dnssec truncation warning: %v", err)
			return
		}
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !validateRequestToken(w, r) {
		return
	}

	cfg, err := parseRequestConfig(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateBenchmarkConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job := startBenchmarkJob(cfg)
	writeJSON(w, http.StatusAccepted, job.snapshot())
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
	result, _ := benchmarkSingleServerWithContext(context.Background(), server, hostnames, nil)
	return result
}

func benchmarkSingleServerWithContext(ctx context.Context, server string, hostnames []string, onProgress benchmarkProgressCallback) (serverBenchmark, error) {
	uncached, err := benchmarkServerPass(ctx, server, hostnames, "cold-cache", onProgress)
	if err != nil {
		return serverBenchmark{}, err
	}
	cached, err := benchmarkServerPass(ctx, server, hostnames, "warm-cache", onProgress)
	if err != nil {
		return serverBenchmark{}, err
	}
	integrity, err := checkResolverIntegrity(ctx, server, onProgress)
	if err != nil {
		return serverBenchmark{}, err
	}
	latencies := append(append(make([]float64, 0, len(uncached.latencies)+len(cached.latencies)), uncached.latencies...), cached.latencies...)

	avgMs, medianMs, p95Ms, minMs, maxMs := summarizeLatencies(latencies)
	queries := uncached.Queries + cached.Queries
	successes := uncached.Successes + cached.Successes
	failures := uncached.Failures + cached.Failures
	failureRate := 0.0
	if queries > 0 {
		failureRate = float64(failures) / float64(queries)
	}

	return serverBenchmark{
		Server:                 server,
		Queries:                queries,
		Successes:              successes,
		Failures:               failures,
		FailureRate:            round2(failureRate),
		AvgMs:                  avgMs,
		MedianMs:               medianMs,
		P95Ms:                  p95Ms,
		MinMs:                  minMs,
		MaxMs:                  maxMs,
		Score:                  scoreServer(uncached, cached, calculateJitter(latencies), integrity.Hijacked),
		LatencyBuckets:         buildLatencyBuckets(latencies),
		UncachedQueries:        uncached.Queries,
		UncachedSuccesses:      uncached.Successes,
		UncachedFailures:       uncached.Failures,
		UncachedAvgMs:          uncached.AvgMs,
		UncachedMedianMs:       uncached.MedianMs,
		UncachedP95Ms:          uncached.P95Ms,
		UncachedLatencyBuckets: uncached.LatencyBuckets,
		CachedQueries:          cached.Queries,
		CachedSuccesses:        cached.Successes,
		CachedFailures:         cached.Failures,
		CachedAvgMs:            cached.AvgMs,
		CachedMedianMs:         cached.MedianMs,
		CachedP95Ms:            cached.P95Ms,
		CachedLatencyBuckets:   cached.LatencyBuckets,
		JitterMs:               calculateJitter(latencies),
		Integrity:              integrity.Status,
		IntegrityDetail:        integrity.Detail,
		IntegrityProbeCount:    integrity.Probes,
		IntegrityCleanCount:    integrity.Clean,
		IntegrityErrorCount:    integrity.Errors,
		IntegrityAnomalyCount:  integrity.Anomaly,
	}, nil
}

func benchmarkServerPass(ctx context.Context, server string, hostnames []string, phase string, onProgress benchmarkProgressCallback) (benchmarkPassStats, error) {
	q := dnsqueue.StartQueue(QUEUE_LENGTH, WORKERS)
	defer q.SendCompletionSignal()

	queued := 0
	for _, record := range hostnames {
		if err := ctx.Err(); err != nil {
			return computePassStats(queued, nil, 0), err
		}
		q.AddWithContext(ctx, server, "A", ensureFQDN(record))
		queued++
	}

	latencies := make([]float64, 0, queued)
	failures := 0
	for i := 0; i < queued; i++ {
		select {
		case <-ctx.Done():
			return computePassStats(queued, latencies, failures), ctx.Err()
		case result := <-q.Results:
			if onProgress != nil {
				onProgress(server, phase, 1)
			}
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
	}

	return computePassStats(queued, latencies, failures), nil
}

func computePassStats(queries int, latencies []float64, failures int) benchmarkPassStats {
	avgMs, medianMs, p95Ms, minMs, maxMs := summarizeLatencies(latencies)
	return benchmarkPassStats{
		Queries:        queries,
		Successes:      len(latencies),
		Failures:       failures,
		AvgMs:          avgMs,
		MedianMs:       medianMs,
		P95Ms:          p95Ms,
		MinMs:          minMs,
		MaxMs:          maxMs,
		LatencyBuckets: buildLatencyBuckets(latencies),
		latencies:      latencies,
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
		if result.Successes > 0 && result.Integrity != "hijacked" {
			return result.Server
		}
	}
	return ""
}

func scoreServer(uncached, cached benchmarkPassStats, jitter float64, hijacked bool) float64 {
	queries := uncached.Queries + cached.Queries
	failures := uncached.Failures + cached.Failures
	if queries <= 0 {
		return 0
	}
	if failures >= queries {
		return 999999
	}
	failureRate := float64(failures) / float64(queries)
	uncachedAvg := scoreLatencyValue(uncached.AvgMs, uncached.Successes)
	uncachedP95 := scoreLatencyValue(uncached.P95Ms, uncached.Successes)
	cachedAvg := scoreLatencyValue(cached.AvgMs, cached.Successes)
	cachedP95 := scoreLatencyValue(cached.P95Ms, cached.Successes)
	score := (0.35 * uncachedAvg) + (0.65 * cachedAvg) + (0.12 * uncachedP95) + (0.08 * cachedP95) + (0.25 * jitter) + (failureRate * 900)
	if hijacked {
		score += 1500
	}
	return round2(score)
}

func scoreLatencyValue(value float64, successes int) float64 {
	if successes == 0 {
		return 500
	}
	return value
}

type resolverIntegrityCheck struct {
	Status   string
	Detail   string
	Hijacked bool
	Probes   int
	Clean    int
	Errors   int
	Anomaly  int
}

type integrityObservation struct {
	ResponseCode int
	AnswerCount  int
	Err          string
}

func checkResolverIntegrity(ctx context.Context, server string, onProgress benchmarkProgressCallback) (resolverIntegrityCheck, error) {
	if err := ctx.Err(); err != nil {
		return resolverIntegrityCheck{}, err
	}

	probes := integrityProbeNames(INTEGRITY_PROBE_COUNT)
	observations := make([]integrityObservation, 0, len(probes))

	for _, probe := range probes {
		result, err := dnsqueue.SendQuery(&dnsqueue.Request{
			Destination: server,
			RecordType:  "A",
			RecordName:  probe,
			Context:     ctx,
		})
		if onProgress != nil {
			onProgress(server, "integrity", 1)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return resolverIntegrityCheck{}, err
			}
			observations = append(observations, integrityObservation{Err: err.Error()})
			continue
		}
		observations = append(observations, integrityObservation{
			ResponseCode: result.ResponseCode,
			AnswerCount:  len(result.Answers),
		})
	}

	return classifyResolverIntegrity(observations), nil
}

func integrityProbeNames(count int) []string {
	if count <= 0 {
		return nil
	}
	probes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		label := strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", ""))
		probes = append(probes, ensureFQDN(label+".namebench-check.invalid"))
	}
	return probes
}

func classifyResolverIntegrity(observations []integrityObservation) resolverIntegrityCheck {
	if len(observations) == 0 {
		return resolverIntegrityCheck{Status: "unknown", Detail: "no integrity probe results"}
	}

	errorCount := 0
	cleanCount := 0
	anomalyCount := 0
	details := make([]string, 0, len(observations))

	for _, observation := range observations {
		if observation.Err != "" {
			errorCount++
			details = append(details, "err="+observation.Err)
			continue
		}
		rcode := dns.RcodeToString[observation.ResponseCode]
		if rcode == "" {
			rcode = fmt.Sprintf("%d", observation.ResponseCode)
		}
		if observation.AnswerCount > 0 || observation.ResponseCode == dns.RcodeSuccess {
			return resolverIntegrityCheck{
				Status:   "hijacked",
				Detail:   fmt.Sprintf("rcode=%s answers=%d", rcode, observation.AnswerCount),
				Hijacked: true,
				Probes:   len(observations),
				Clean:    cleanCount,
				Errors:   errorCount,
				Anomaly:  anomalyCount + 1,
			}
		}
		if observation.ResponseCode == dns.RcodeNameError && observation.AnswerCount == 0 {
			cleanCount++
			continue
		}
		anomalyCount++
		details = append(details, fmt.Sprintf("rcode=%s answers=%d", rcode, observation.AnswerCount))
	}

	if cleanCount == len(observations) {
		return resolverIntegrityCheck{Status: "clean", Probes: len(observations), Clean: cleanCount}
	}
	if errorCount == len(observations) {
		return resolverIntegrityCheck{Status: "unknown", Detail: strings.Join(details, "; "), Probes: len(observations), Errors: errorCount}
	}
	if len(details) == 0 {
		details = append(details, fmt.Sprintf("clean=%d/%d", cleanCount, len(observations)))
	}
	return resolverIntegrityCheck{
		Status:  "suspicious",
		Detail:  strings.Join(details, "; "),
		Probes:  len(observations),
		Clean:   cleanCount,
		Errors:  errorCount,
		Anomaly: anomalyCount,
	}
}

func calculateJitter(samples []float64) float64 {
	if len(samples) < 2 {
		return 0
	}
	mean := 0.0
	for _, sample := range samples {
		mean += sample
	}
	mean /= float64(len(samples))

	variance := 0.0
	for _, sample := range samples {
		diff := sample - mean
		variance += diff * diff
	}
	variance /= float64(len(samples))
	return round2(math.Sqrt(variance))
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
	chunks := splitNameserverTokens(raw)
	if len(chunks) == 0 {
		return nil
	}

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

func splitNameserverTokens(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
}

func mergeNameServers(manual []string, system []string, includeGlobal, includeRegional bool, location string) []string {
	merged := make([]string, 0, len(manual)+len(system)+16)
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
	appendUnique(system)
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
		if !isAllowedResolverIP(ip) {
			return "", false
		}
		return net.JoinHostPort(ip.String(), "53"), true
	}

	if !strings.Contains(raw, ":") {
		raw += ":53"
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || host == "" || port == "" {
		return "", false
	}
	ip := net.ParseIP(host)
	if ip == nil || !isAllowedResolverIP(ip) {
		return "", false
	}
	return net.JoinHostPort(ip.String(), port), true
}

func isAllowedResolverIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		// Carrier-grade NAT range: 100.64.0.0/10
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return false
		}
	}
	return true
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
		QueryCount:      COUNT,
		IncludeSystem:   true,
		IncludeMetadata: false,
		HistoryConsent:  false,
		DataSource:      "chrome",
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			return requestConfig{}, err
		}
		cfg.QueryCount = parsePositiveInt(strconv.Itoa(cfg.QueryCount), COUNT)
		cfg.Location = strings.ToLower(strings.TrimSpace(cfg.Location))
		cfg.Nameservers = strings.TrimSpace(cfg.Nameservers)
		cfg.DataSource = normalizeDataSourceValue(cfg.DataSource)
		return cfg, nil
	}

	if err := parseIncomingForm(r); err != nil {
		return requestConfig{}, err
	}
	cfg.QueryCount = parsePositiveInt(r.FormValue("query_count"), COUNT)
	cfg.IncludeSystem = formEnabled(r, "include_system")
	cfg.IncludeMetadata = formEnabled(r, "include_metadata")
	cfg.IncludeGlobal = formEnabled(r, "include_global")
	cfg.IncludeRegional = formEnabled(r, "include_regional")
	cfg.HistoryConsent = formEnabled(r, "history_consent")
	cfg.Location = strings.ToLower(strings.TrimSpace(r.FormValue("location")))
	cfg.Nameservers = strings.TrimSpace(r.FormValue("nameservers"))
	if raw := strings.TrimSpace(r.FormValue("data_source")); raw != "" {
		cfg.DataSource = normalizeDataSourceValue(raw)
	}
	return cfg, nil
}

func normalizeDataSourceValue(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "chrome"
	}
	return value
}

func resolveDataSource(raw string) (string, string) {
	dataSource := normalizeDataSourceValue(raw)
	if dataSource == "chrome" {
		return dataSource, ""
	}
	return "chrome", fmt.Sprintf("Data source %q is not supported yet. Falling back to Chromium browser history.", dataSource)
}

func loadHistoryRecords(dataSource string, days int) ([]string, error) {
	switch dataSource {
	case "chrome":
		return history.ChromiumFamily(days)
	default:
		return nil, fmt.Errorf("unsupported data source: %s", dataSource)
	}
}

func fallbackHostnames(count int) []string {
	base := []string{
		"google.com",
		"youtube.com",
		"facebook.com",
		"instagram.com",
		"wikipedia.org",
		"x.com",
		"reddit.com",
		"amazon.com",
		"netflix.com",
		"microsoft.com",
		"apple.com",
		"cloudflare.com",
		"openai.com",
		"yahoo.com",
		"bing.com",
		"office.com",
		"github.com",
		"stackoverflow.com",
		"whatsapp.com",
		"tiktok.com",
		"linkedin.com",
		"imdb.com",
		"cnn.com",
		"bbc.com",
		"nytimes.com",
		"mozilla.org",
		"adobe.com",
		"spotify.com",
		"wordpress.com",
		"quora.com",
	}

	if count <= 0 {
		return nil
	}
	result := make([]string, 0, count)
	for len(result) < count {
		for _, domain := range base {
			if len(result) >= count {
				break
			}
			result = append(result, domain)
		}
		if len(base) == 0 {
			break
		}
	}
	return result
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

func validateRequestToken(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("X-Namebench-Token")) != appRequestToken {
		http.Error(w, "invalid request token", http.StatusForbidden)
		return false
	}
	return true
}

func validateBenchmarkConfig(cfg requestConfig) error {
	if !cfg.HistoryConsent {
		return errors.New("history consent is required before benchmarking")
	}
	return nil
}

func limitResolvers(servers []string, max int) ([]string, int) {
	if max <= 0 || len(servers) <= max {
		return servers, 0
	}
	return servers[:max], len(servers) - max
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
