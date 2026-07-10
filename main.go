// webterminal: a browser-based terminal server for Raspberry Pi class devices.
//
// Single static binary, embedded UI, basic auth. Designed to run on anything
// from a Pi 1 / RevPi Core 1 (ARMv6) upwards.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "listen address, e.g. :8080 or 192.168.1.10:8080")
	shell := flag.String("shell", defaultShell(), "shell to run (started as a login shell)")
	user := flag.String("user", os.Getenv("WT_USER"), "basic auth username (env WT_USER)")
	pass := flag.String("pass", os.Getenv("WT_PASS"), "basic auth password (env WT_PASS)")
	flag.Parse()

	if *user == "" || *pass == "" {
		log.Fatal("refusing to start without credentials: set WT_USER/WT_PASS env vars (or -user/-pass flags)")
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "pass" {
			log.Print("warning: -pass on the command line is visible to all local users via ps; prefer the WT_PASS environment variable")
		}
	})

	staticFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/ws", terminalHandler(*shell))

	handler := basicAuth(*user, *pass, mux)

	host, _ := os.Hostname()
	fmt.Printf("webterminal on %s — http://%s%s (shell: %s)\n", host, listenHost(*addr), *addr, *shell)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

func defaultShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/bash"
}

func listenHost(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		if h, err := os.Hostname(); err == nil {
			return h
		}
		return "localhost"
	}
	return ""
}
