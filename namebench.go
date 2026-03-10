package main

import (
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

	"github.com/google/namebench/ui"
)

var port = flag.Int("port", 8100, "Port to listen on")
var openBrowserFlag = flag.Bool("open_browser", true, "Open the default browser automatically")
var debugFlag = flag.Bool("debug", false, "Enable verbose logging to stdout")
var logFileFlag = flag.String("log_file", "namebench.log", "Path to the application log file")

// openBrowser opens the system default browser at the given URL.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command(systemExecutablePath("rundll32.exe"), "url.dll,FileProtocolHandler", url).Start()
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
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.SetOutput(io.Discard)
		return
	}
	log.SetOutput(file)
}

func main() {
	flag.Parse()
	configureLogging()
	ui.RegisterHandlers()

	listenAddr := fmt.Sprintf("127.0.0.1:%d", *port)

	listener, err := net.Listen("tcp4", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}

	url := fmt.Sprintf("http://%s/", listener.Addr().String())
	log.Printf("namebench is listening at %s", url)

	if *openBrowserFlag {
		go func() {
			if err := openBrowser(url); err != nil {
				log.Printf("auto-open browser failed: %v", err)
				log.Printf("open this URL manually: %s", url)
			}
		}()
	}

	if err := http.Serve(listener, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
