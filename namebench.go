package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/google/namebench/ui"
)

var port = flag.Int("port", 8100, "Port to listen on")
var openBrowserFlag = flag.Bool("open_browser", true, "Open the default browser automatically")
var debugFlag = flag.Bool("debug", false, "Enable verbose logging to stdout")
var logFileFlag = flag.String("log_file", "namebench.log", "Path to the application log file")
var idleTimeoutFlag = flag.Duration("idle_timeout", 15*time.Minute, "How long the local UI can stay idle before it shuts down automatically (0 disables auto-shutdown)")

// openBrowser opens the system default browser at the given URL.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return startHiddenCommand(systemExecutablePath("rundll32.exe"), "url.dll,FileProtocolHandler", url)
	case "darwin":
		return exec.Command("/usr/bin/open", url).Start()
	default:
		return exec.Command(linuxExecutablePath("xdg-open"), url).Start()
	}
}

func systemExecutablePath(name string) string {
	root := strings.TrimSpace(os.Getenv("SystemRoot"))
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", name)
}

func linuxExecutablePath(name string) string {
	candidate := filepath.Join("/usr/bin", name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return name
}

func configureLogging() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if *debugFlag {
		return
	}

	target := *logFileFlag
	if !filepath.IsAbs(target) {
		if exePath, err := os.Executable(); err == nil {
			target = filepath.Join(filepath.Dir(exePath), target)
		}
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.SetOutput(io.Discard)
		return
	}
	log.SetOutput(file)
}

func main() {
	flag.Parse()
	configureLogging()

	instanceGuard, existingState, err := acquireInstanceGuard()
	if err != nil {
		if existingState != nil && existingState.URL != "" {
			log.Printf("another namebench instance is already running at %s", existingState.URL)
			if *openBrowserFlag {
				if openErr := openBrowser(existingState.URL); openErr != nil {
					log.Printf("failed to open running instance URL: %v", openErr)
				}
			}
			return
		}
		log.Fatalf("failed to acquire single-instance lock: %v", err)
	}
	defer instanceGuard.Release()

	listenAddr := fmt.Sprintf("127.0.0.1:%d", *port)

	listener, err := net.Listen("tcp4", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}

	basePath := "/" + strings.ToLower(uuid.NewString()) + "/"
	url := fmt.Sprintf("http://%s%s", listener.Addr().String(), basePath)
	if err := instanceGuard.WriteState(instanceState{
		PID:       os.Getpid(),
		URL:       url,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		log.Printf("failed to persist running instance metadata: %v", err)
	}
	log.Printf("namebench is listening at %s", url)

	runtimeState := newAppRuntime(*idleTimeoutFlag)
	uiHandler := ui.RegisterHandlers(ui.HandlerOptions{
		BasePath: basePath,
		OnShutdown: func() {
			runtimeState.requestShutdown("requested by UI")
		},
		OnSessionOpen: func(sessionID string) {
			runtimeState.sessionOpened(sessionID)
		},
		OnSessionClose: func(sessionID string) {
			runtimeState.sessionClosed(sessionID)
		},
		OnSessionPing: func(sessionID string) {
			runtimeState.sessionPing(sessionID)
		},
	})
	prefix := strings.TrimSuffix(basePath, "/")
	baseMux := http.NewServeMux()
	baseMux.Handle(basePath, http.StripPrefix(prefix, uiHandler))
	handler := runtimeState.wrapHandler(baseMux)
	server := &http.Server{Handler: handler}
	runtimeState.attachServer(server)

	monitorCtx, cancelMonitor := context.WithCancel(context.Background())
	defer cancelMonitor()
	go runtimeState.monitorIdle(monitorCtx)

	if *openBrowserFlag {
		go func() {
			if err := openBrowser(url); err != nil {
				log.Printf("auto-open browser failed: %v", err)
				log.Printf("open this URL manually: %s", url)
			}
		}()
	}

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
	runtimeState.waitForShutdown()
}
