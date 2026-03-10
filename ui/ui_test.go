package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestResolveDataSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantSource  string
		wantWarning bool
	}{
		{
			name:       "default chrome",
			input:      "",
			wantSource: "chrome",
		},
		{
			name:       "chrome remains supported",
			input:      "chrome",
			wantSource: "chrome",
		},
		{
			name:        "unsupported source falls back with warning",
			input:       "firefox",
			wantSource:  "chrome",
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSource, gotWarning := resolveDataSource(tt.input)
			if gotSource != tt.wantSource {
				t.Fatalf("resolveDataSource() source = %q, want %q", gotSource, tt.wantSource)
			}
			if (gotWarning != "") != tt.wantWarning {
				t.Fatalf("resolveDataSource() warning present = %t, want %t", gotWarning != "", tt.wantWarning)
			}
		})
	}
}

func TestScoreServerPrefersFasterWarmCache(t *testing.T) {
	t.Parallel()

	fastWarm := scoreServer(
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 40, P95Ms: 60},
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 8, P95Ms: 12},
		4,
		false,
	)
	slowWarm := scoreServer(
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 35, P95Ms: 50},
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 25, P95Ms: 35},
		12,
		false,
	)

	if fastWarm >= slowWarm {
		t.Fatalf("expected faster warm-cache resolver to score better, got fastWarm=%v slowWarm=%v", fastWarm, slowWarm)
	}
}

func TestScoreServerPenalizesHijack(t *testing.T) {
	t.Parallel()

	clean := scoreServer(
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 20, P95Ms: 30},
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 10, P95Ms: 16},
		3,
		false,
	)
	hijacked := scoreServer(
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 20, P95Ms: 30},
		benchmarkPassStats{Queries: 10, Successes: 10, AvgMs: 10, P95Ms: 16},
		3,
		true,
	)

	if hijacked <= clean {
		t.Fatalf("expected hijacked resolver to score worse, got hijacked=%v clean=%v", hijacked, clean)
	}
}

func TestClassifyResolverIntegrity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		observations []integrityObservation
		wantStatus   string
		wantHijacked bool
		wantProbes   int
		wantClean    int
		wantErrors   int
		wantAnomaly  int
	}{
		{
			name: "clean when all probes return nxdomain",
			observations: []integrityObservation{
				{ResponseCode: dns.RcodeNameError, AnswerCount: 0},
				{ResponseCode: dns.RcodeNameError, AnswerCount: 0},
				{ResponseCode: dns.RcodeNameError, AnswerCount: 0},
			},
			wantStatus: "clean",
			wantProbes: 3,
			wantClean:  3,
		},
		{
			name: "hijacked when any probe returns answers",
			observations: []integrityObservation{
				{ResponseCode: dns.RcodeNameError, AnswerCount: 0},
				{ResponseCode: dns.RcodeSuccess, AnswerCount: 1},
			},
			wantStatus:   "hijacked",
			wantHijacked: true,
			wantProbes:   2,
			wantClean:    1,
			wantAnomaly:  1,
		},
		{
			name: "unknown when all probes fail",
			observations: []integrityObservation{
				{Err: "timeout"},
				{Err: "refused"},
			},
			wantStatus: "unknown",
			wantProbes: 2,
			wantErrors: 2,
		},
		{
			name: "suspicious when probes are mixed without hijack",
			observations: []integrityObservation{
				{ResponseCode: dns.RcodeNameError, AnswerCount: 0},
				{ResponseCode: dns.RcodeServerFailure, AnswerCount: 0},
			},
			wantStatus:  "suspicious",
			wantProbes:  2,
			wantClean:   1,
			wantAnomaly: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyResolverIntegrity(tt.observations)
			if got.Status != tt.wantStatus {
				t.Fatalf("classifyResolverIntegrity() status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Hijacked != tt.wantHijacked {
				t.Fatalf("classifyResolverIntegrity() hijacked = %t, want %t", got.Hijacked, tt.wantHijacked)
			}
			if got.Probes != tt.wantProbes || got.Clean != tt.wantClean || got.Errors != tt.wantErrors || got.Anomaly != tt.wantAnomaly {
				t.Fatalf("classifyResolverIntegrity() counts = probes:%d clean:%d errors:%d anomaly:%d, want probes:%d clean:%d errors:%d anomaly:%d",
					got.Probes, got.Clean, got.Errors, got.Anomaly,
					tt.wantProbes, tt.wantClean, tt.wantErrors, tt.wantAnomaly,
				)
			}
		})
	}
}

func TestCleanupExpiredBenchmarkJobs(t *testing.T) {
	oldJob := &benchmarkJob{ID: "expired-job", finished: time.Now().Add(-benchmarkJobRetention - time.Minute)}
	freshJob := &benchmarkJob{ID: "fresh-job", finished: time.Now().Add(-5 * time.Minute)}

	benchmarkJobs.Store(oldJob.ID, oldJob)
	benchmarkJobs.Store(freshJob.ID, freshJob)
	defer benchmarkJobs.Delete(oldJob.ID)
	defer benchmarkJobs.Delete(freshJob.ID)

	cleanupExpiredBenchmarkJobs(time.Now())

	if _, ok := benchmarkJobs.Load(oldJob.ID); ok {
		t.Fatalf("expected expired job to be cleaned up")
	}
	if _, ok := benchmarkJobs.Load(freshJob.ID); !ok {
		t.Fatalf("expected fresh job to remain available")
	}
}

func TestValidateRequestToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("X-Namebench-Token", appRequestToken)
	rec := httptest.NewRecorder()

	if !validateRequestToken(rec, req) {
		t.Fatalf("expected request token to validate")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code %d", rec.Code)
	}
}

func TestLimitResolvers(t *testing.T) {
	t.Parallel()

	input := []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}
	got, truncated := limitResolvers(input, 2)
	if truncated != 1 {
		t.Fatalf("limitResolvers() truncated = %d, want 1", truncated)
	}
	if len(got) != 2 {
		t.Fatalf("limitResolvers() len = %d, want 2", len(got))
	}
}

func TestValidateBenchmarkConfigRequiresHistoryConsent(t *testing.T) {
	t.Parallel()

	if err := validateBenchmarkConfig(requestConfig{HistoryConsent: false}); err == nil {
		t.Fatalf("expected missing history consent to be rejected")
	}
	if err := validateBenchmarkConfig(requestConfig{HistoryConsent: true}); err != nil {
		t.Fatalf("expected consented benchmark config to pass, got %v", err)
	}
}
