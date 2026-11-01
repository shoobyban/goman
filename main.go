package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"goman/internal/goman"
	"goman/internal/tui"
)

func main() {
	workspace := flag.String("workspace", ".", "workspace containing .goman collections (imports legacy .ghostman data)")
	address := flag.String("webhook-address", "127.0.0.1:9090", "webhook listen address")
	timeout := flag.Duration("timeout", 15*time.Second, "HTTP request timeout")
	insecure := flag.Bool("insecure", false, "disable TLS certificate verification")
	version := flag.Bool("version", false, "print version")
	flag.Parse()
	if *version {
		fmt.Println("goman 0.1.0")
		return
	}
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "timeout must be positive")
		os.Exit(1)
	}
	store, collections, err := goman.LoadWorkspace(*workspace)
	if err == nil {
		app := tui.New(store, collections, goman.NewClient(*timeout, *insecure), *address)
		err = app.Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "goman:", err)
		os.Exit(1)
	}
}
