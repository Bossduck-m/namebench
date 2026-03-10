package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/namebench/history"
	"github.com/google/uuid"
)

const benchmarkJobRetention = 30 * time.Minute

type benchmarkProgress struct {
	CompletedSteps int     `json:"completed_steps"`
	TotalSteps     int     `json:"total_steps"`
	Percent        float64 `json:"percent"`
	CurrentServer  string  `json:"current_server,omitempty"`
	CurrentPhase   string  `json:"current_phase,omitempty"`
	ElapsedSeconds int64   `json:"elapsed_seconds,omitempty"`
	ETASeconds     int64   `json:"eta_seconds,omitempty"`
}

type benchmarkJobSnapshot struct {
	JobID    string             `json:"job_id"`
	Status   string             `json:"status"`
	Progress benchmarkProgress  `json:"progress"`
	Result   *benchmarkResponse `json:"result,omitempty"`
	Error    string             `json:"error,omitempty"`
}

type benchmarkJob struct {
	ID     string
	cancel context.CancelFunc

	mu       sync.RWMutex
	status   string
	progress benchmarkProgress
	result   *benchmarkResponse
	errText  string
	started  time.Time
	finished time.Time
}

var benchmarkJobs sync.Map

func startBenchmarkJob(cfg requestConfig) *benchmarkJob {
	cleanupExpiredBenchmarkJobs(time.Now())

	ctx, cancel := context.WithCancel(context.Background())
	job := &benchmarkJob{
		ID:     uuid.NewString(),
		cancel: cancel,
		status: "queued",
		progress: benchmarkProgress{
			CompletedSteps: 0,
			TotalSteps:     1,
			Percent:        0,
		},
	}
	benchmarkJobs.Store(job.ID, job)
	go job.run(ctx, cfg)
	return job
}

func (j *benchmarkJob) run(ctx context.Context, cfg requestConfig) {
	j.setStatus("running")

	queryCount := cfg.QueryCount
	includeGlobal := cfg.IncludeGlobal
	includeRegional := cfg.IncludeRegional
	location := cfg.Location

	rawNameservers := cfg.Nameservers
	dataSource, dataSourceWarning := resolveDataSource(cfg.DataSource)
	manualTokenCount := len(splitNameserverTokens(rawNameservers))
	nameServers := parseNameServers(rawNameservers)
	systemServers := []string{}
	systemWarning := ""
	if cfg.IncludeSystem {
		systemServers, systemWarning = discoverSystemResolvers()
	}
	candidateServers := mergeNameServers(nameServers, systemServers, includeGlobal, includeRegional, location)
	truncatedResolvers := 0
	candidateServers, truncatedResolvers = limitResolvers(candidateServers, MAX_RESOLVERS)
	warnings := make([]string, 0, 4)
	if dataSourceWarning != "" {
		warnings = append(warnings, dataSourceWarning)
	}
	if systemWarning != "" {
		warnings = append(warnings, systemWarning)
	}
	if truncatedResolvers > 0 {
		warnings = append(warnings, fmt.Sprintf("Resolver list truncated by %d entries (limit %d).", truncatedResolvers, MAX_RESOLVERS))
	}
	if rawNameservers == "" {
		switch {
		case len(systemServers) > 0:
			warnings = append(warnings, fmt.Sprintf("Discovered %d system DNS resolvers on this machine.", len(systemServers)))
		case includeGlobal || includeRegional:
			warnings = append(warnings, "No manual nameserver provided, using default provider pools.")
		default:
			warnings = append(warnings, "No manual or system nameserver available, falling back to the built-in public resolver.")
		}
	} else if manualTokenCount > len(nameServers) {
		warnings = append(warnings, fmt.Sprintf("%d manual nameserver entries were ignored (invalid or private/local IP).", manualTokenCount-len(nameServers)))
	} else if len(nameServers) == 0 {
		warnings = append(warnings, "Manual nameservers could not be parsed. Use one public IPv4/IPv6 per line.")
	}

	records, err := loadHistoryRecords(dataSource, HISTORY_DAYS)
	if err != nil {
		j.fail(err)
		return
	}

	hostnames := history.Random(queryCount, history.Uniq(history.ExternalHostnames(records)))
	if len(hostnames) == 0 {
		hostnames = fallbackHostnames(queryCount)
		warnings = append(warnings, "No eligible hostnames found in Chromium browser history. Using fallback public domain set.")
	}
	if len(hostnames) == 0 {
		j.complete(&benchmarkResponse{
			RequestedQueries: queryCount,
			ExecutedQueries:  0,
			ServerCount:      len(candidateServers),
			Winner:           "",
			Results:          []serverBenchmark{},
			Warnings:         append(warnings, "No eligible hostnames available for benchmark."),
		})
		return
	}

	results := make([]serverBenchmark, 0, len(candidateServers))
	totalSteps := len(candidateServers) * ((len(hostnames) * 2) + INTEGRITY_PROBE_COUNT)
	if totalSteps == 0 {
		totalSteps = 1
	}
	j.beginProgress(totalSteps, "", "preparing")
	progressStep := func(server, phase string, delta int) {
		j.advanceProgress(delta, server, phase)
	}

	for _, server := range candidateServers {
		if err := ctx.Err(); err != nil {
			j.cancelled()
			return
		}
		j.setCurrentWork(server, "benchmarking")
		result, err := benchmarkSingleServerWithContext(ctx, server, hostnames, progressStep)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				j.cancelled()
				return
			}
			warnings = append(warnings, fmt.Sprintf("Benchmark for %s failed: %v", server, err))
			continue
		}
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

	if cfg.IncludeMetadata {
		warnings = append(warnings, enrichServerMetadata(results)...)
	}

	winner := selectWinner(results)
	if winner == "" && len(results) > 0 {
		winner = results[0].Server
		warnings = append(warnings, "No server returned both successful and integrity-safe answers. Ranking falls back to penalty score.")
	}

	j.complete(&benchmarkResponse{
		RequestedQueries: queryCount,
		ExecutedQueries:  len(results) * ((len(hostnames) * 2) + INTEGRITY_PROBE_COUNT),
		ServerCount:      len(candidateServers),
		Winner:           winner,
		Results:          results,
		Warnings:         warnings,
	})
}

func (j *benchmarkJob) snapshot() benchmarkJobSnapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return benchmarkJobSnapshot{
		JobID:    j.ID,
		Status:   j.status,
		Progress: j.progress,
		Result:   j.result,
		Error:    j.errText,
	}
}

func (j *benchmarkJob) setStatus(status string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = status
}

func (j *benchmarkJob) beginProgress(total int, server, phase string) {
	if total <= 0 {
		total = 1
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "running"
	j.started = time.Now()
	j.progress = benchmarkProgress{
		CompletedSteps: 0,
		TotalSteps:     total,
		Percent:        0,
		CurrentServer:  server,
		CurrentPhase:   phase,
	}
}

func (j *benchmarkJob) setCurrentWork(server, phase string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.progress.CurrentServer = server
	j.progress.CurrentPhase = phase
	j.progress.ElapsedSeconds = j.elapsedSecondsLocked()
	j.progress.ETASeconds = j.etaSecondsLocked(j.progress.CompletedSteps, j.progress.TotalSteps)
}

func (j *benchmarkJob) advanceProgress(delta int, server, phase string) {
	if delta < 0 {
		delta = 0
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	completed := j.progress.CompletedSteps + delta
	total := j.progress.TotalSteps
	if total <= 0 {
		total = 1
	}
	if completed > total {
		completed = total
	}

	percent := 0.0
	if total > 0 {
		percent = round2((float64(completed) / float64(total)) * 100)
	}
	j.status = "running"
	j.progress = benchmarkProgress{
		CompletedSteps: completed,
		TotalSteps:     total,
		Percent:        percent,
		CurrentServer:  server,
		CurrentPhase:   phase,
		ElapsedSeconds: j.elapsedSecondsLocked(),
		ETASeconds:     j.etaSecondsLocked(completed, total),
	}
}

func (j *benchmarkJob) complete(result *benchmarkResponse) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "completed"
	j.finished = time.Now()
	j.progress.CurrentPhase = "completed"
	j.progress.CurrentServer = ""
	j.progress.Percent = 100
	j.progress.CompletedSteps = j.progress.TotalSteps
	j.progress.ElapsedSeconds = j.elapsedSecondsLocked()
	j.progress.ETASeconds = 0
	j.result = result
}

func (j *benchmarkJob) fail(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "error"
	j.finished = time.Now()
	j.errText = err.Error()
	j.progress.CurrentPhase = "error"
	j.progress.CurrentServer = ""
	j.progress.ElapsedSeconds = j.elapsedSecondsLocked()
	j.progress.ETASeconds = 0
}

func (j *benchmarkJob) cancelled() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "canceled"
	j.finished = time.Now()
	j.progress.CurrentPhase = "canceled"
	j.progress.CurrentServer = ""
	j.progress.ElapsedSeconds = j.elapsedSecondsLocked()
	j.progress.ETASeconds = 0
}

func (j *benchmarkJob) elapsedSecondsLocked() int64 {
	if j.started.IsZero() {
		return 0
	}
	return int64(time.Since(j.started).Round(time.Second) / time.Second)
}

func (j *benchmarkJob) etaSecondsLocked(completed, total int) int64 {
	if j.started.IsZero() || completed <= 0 || completed >= total || total <= 0 {
		return 0
	}
	elapsed := time.Since(j.started).Seconds()
	if elapsed <= 0 {
		return 0
	}
	remaining := (elapsed / float64(completed)) * float64(total-completed)
	return int64(remaining + 0.5)
}

func Progress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !validateRequestToken(w, r) {
		return
	}
	job, ok := lookupBenchmarkJob(parseJobID(r))
	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}

func Cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !validateRequestToken(w, r) {
		return
	}
	job, ok := lookupBenchmarkJob(parseJobID(r))
	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	job.cancel()
	writeJSON(w, http.StatusOK, job.snapshot())
}

func lookupBenchmarkJob(jobID string) (*benchmarkJob, bool) {
	if jobID == "" {
		return nil, false
	}
	cleanupExpiredBenchmarkJobs(time.Now())
	value, ok := benchmarkJobs.Load(jobID)
	if !ok {
		return nil, false
	}
	job, ok := value.(*benchmarkJob)
	return job, ok
}

func HasRunningJobs() bool {
	hasRunning := false
	benchmarkJobs.Range(func(_, value interface{}) bool {
		job, ok := value.(*benchmarkJob)
		if !ok {
			return true
		}
		if job.isActive() {
			hasRunning = true
			return false
		}
		return true
	})
	return hasRunning
}

func CancelAllBenchmarkJobs() {
	benchmarkJobs.Range(func(_, value interface{}) bool {
		job, ok := value.(*benchmarkJob)
		if ok {
			job.cancel()
		}
		return true
	})
}

func parseJobID(r *http.Request) string {
	if jobID := strings.TrimSpace(r.URL.Query().Get("job_id")); jobID != "" {
		return jobID
	}
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var payload struct {
			JobID string `json:"job_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			return strings.TrimSpace(payload.JobID)
		}
	}
	if err := parseIncomingForm(r); err == nil {
		if jobID := strings.TrimSpace(r.FormValue("job_id")); jobID != "" {
			return jobID
		}
	}
	return ""
}

func cleanupExpiredBenchmarkJobs(now time.Time) {
	benchmarkJobs.Range(func(key, value interface{}) bool {
		job, ok := value.(*benchmarkJob)
		if !ok {
			benchmarkJobs.Delete(key)
			return true
		}
		if job.isExpired(now) {
			benchmarkJobs.Delete(key)
		}
		return true
	})
}

func (j *benchmarkJob) isExpired(now time.Time) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.finished.IsZero() {
		return false
	}
	return now.Sub(j.finished) > benchmarkJobRetention
}

func (j *benchmarkJob) isActive() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status == "queued" || j.status == "running"
}
