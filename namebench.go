package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/google/namebench/ui"
)

var port = flag.Int("port", 0, "Port to listen on (0 picks a random local port)")
var openBrowserFlag = flag.Bool("open_browser", true, "Open the default browser automatically")

// openBrowser opens the system default browser at the given URL.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func main() {
	flag.Parse()
	ui.RegisterHandlers()

	listenAddr := fmt.Sprintf("127.0.0.1:%d", *port)
	if *port == 0 {
		listenAddr = "127.0.0.1:0"
	}

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
