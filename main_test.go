package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestAcquireInstanceGuardEnforcesSingleInstance(t *testing.T) {
	t.Cleanup(func() {
		instanceStateDirOverride = ""
	})
	instanceStateDirOverride = t.TempDir()

	first, _, err := acquireInstanceGuard()
	if err != nil {
		t.Fatalf("first acquireInstanceGuard() error = %v", err)
	}
	defer first.Release()

	if err := first.WriteState(instanceState{PID: 123, URL: "http://127.0.0.1:18110/"}); err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}

	second, state, err := acquireInstanceGuard()
	if second != nil {
		t.Fatalf("expected second guard acquisition to fail while first is held")
	}
	if !errors.Is(err, errAnotherInstanceRunning) {
		t.Fatalf("expected errAnotherInstanceRunning, got %v", err)
	}
	if state == nil || state.URL != "http://127.0.0.1:18110/" {
		t.Fatalf("expected running instance state to be returned, got %#v", state)
	}
}

func TestAppRuntimeIdleShutdown(t *testing.T) {
	minIdleCheckInterval = 5 * time.Millisecond
	maxIdleCheckInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		minIdleCheckInterval = 15 * time.Second
		maxIdleCheckInterval = time.Minute
	})

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	runtimeState := newAppRuntime(10 * time.Millisecond)
	server := &http.Server{
		Handler: runtimeState.wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	}
	runtimeState.attachServer(server)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runtimeState.monitorIdle(ctx)

	select {
	case serveErr := <-serveDone:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			t.Fatalf("Serve() error = %v", serveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected idle runtime to shut server down")
	}

	runtimeState.waitForShutdown()
}
