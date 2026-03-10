package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/namebench/ui"
)

var (
	minIdleCheckInterval = 15 * time.Second
	maxIdleCheckInterval = 1 * time.Minute
)

type appRuntime struct {
	idleTimeout time.Duration

	mu           sync.RWMutex
	lastActivity time.Time
	shuttingDown bool
	server       *http.Server
	shutdownDone chan struct{}
}

func newAppRuntime(idleTimeout time.Duration) *appRuntime {
	return &appRuntime{
		idleTimeout:  idleTimeout,
		lastActivity: time.Now(),
		shutdownDone: make(chan struct{}),
	}
}

func (a *appRuntime) attachServer(server *http.Server) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.server = server
}

func (a *appRuntime) wrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.markActivity()
		next.ServeHTTP(w, r)
	})
}

func (a *appRuntime) markActivity() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastActivity = time.Now()
}

func (a *appRuntime) monitorIdle(ctx context.Context) {
	if a.idleTimeout <= 0 {
		return
	}

	interval := a.idleTimeout / 4
	if interval < minIdleCheckInterval {
		interval = minIdleCheckInterval
	}
	if interval > maxIdleCheckInterval {
		interval = maxIdleCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ui.HasRunningJobs() {
				continue
			}
			if time.Since(a.lastSeenActivity()) < a.idleTimeout {
				continue
			}
			a.requestShutdown("idle timeout reached")
			return
		}
	}
}

func (a *appRuntime) lastSeenActivity() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastActivity
}

func (a *appRuntime) requestShutdown(reason string) {
	server := a.beginShutdown()
	if server == nil {
		return
	}

	log.Printf("shutting down namebench: %s", reason)
	ui.CancelAllBenchmarkJobs()

	go func() {
		defer close(a.shutdownDone)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("forced close failed: %v", closeErr)
			}
		}
	}()
}

func (a *appRuntime) beginShutdown() *http.Server {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shuttingDown {
		return nil
	}
	a.shuttingDown = true
	return a.server
}

func (a *appRuntime) waitForShutdown() {
	a.mu.RLock()
	shuttingDown := a.shuttingDown
	done := a.shutdownDone
	a.mu.RUnlock()
	if !shuttingDown {
		return
	}
	<-done
}
