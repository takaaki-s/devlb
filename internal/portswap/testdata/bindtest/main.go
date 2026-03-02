package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bindtest <port>[,<port>...] [--serve]")
		os.Exit(2)
	}

	serve := false
	if len(os.Args) >= 3 && os.Args[2] == "--serve" {
		serve = true
	}

	var listeners []net.Listener
	portStrs := strings.Split(os.Args[1], ",")
	for _, ps := range portStrs {
		port, err := strconv.Atoi(ps)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port: %s\n", ps)
			os.Exit(2)
		}

		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			fmt.Fprintf(os.Stderr, "bind failed: %v\n", err)
			os.Exit(1)
		}
		listeners = append(listeners, ln)

		// Print actual bound address to stdout (one per line)
		fmt.Println(ln.Addr().String())
	}

	if serve {
		// Wait until killed
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
	}

	for _, ln := range listeners {
		ln.Close()
	}
}
