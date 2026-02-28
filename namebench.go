package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"

	"github.com/google/namebench/ui"
)

var nw_path = flag.String("nw_path", "/Applications/node-webkit.app/Contents/MacOS/node-webkit",
	"Path to nodejs-webkit binary")
var nw_package = flag.String("nw_package", "./ui/app.nw", "Path to nodejs-webkit package")
var port = flag.Int("port", 0, "Port to listen on")

// openWindow opens a nodejs-webkit window, and points it at the given URL.
func openWindow(url string) (err error) {
	if err := os.Setenv("APP_URL", url); err != nil {
		return err
	}
	cmd := exec.Command(*nw_path, *nw_package)
	if err := cmd.Run(); err != nil {
		log.Printf("error running %s %s: %s", *nw_path, *nw_package, err)
		return err
	}
	return
}

func main() {
	flag.Parse()
	ui.RegisterHandlers()

	if *port != 0 {
		log.Printf("Listening at :%d", *port)
		err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
		if err != nil {
			log.Fatalf("Failed to listen on %d: %s", *port, err)
		}
	} else {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("Failed to listen: %s", err)
		}
		url := fmt.Sprintf("http://%s/", listener.Addr().String())
		log.Printf("URL: %s", url)
		go func() {
			if err := openWindow(url); err != nil {
				log.Printf("window launch failed: %v", err)
			}
		}()
		if err := http.Serve(listener, nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}
}
