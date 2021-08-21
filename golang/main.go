package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var (
	bind string
	port int
)

func init() {
	flag.StringVar(&bind, "bind", "0.0.0.0", "bind address")
	flag.IntVar(&port, "port", 9292, "bind port")

	flag.Parse()
}

func main() {
	fileName := "/tmp/go.sock"
	if err := os.Remove(fileName); err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	}
	l, err := net.Listen("unix", fileName)
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}
	if err := os.Chmod(fileName, 0666); err != nil {
		panic(err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func(c chan os.Signal) {
		// Wait for a SIGINT or SIGKILL:
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		// Stop listening (and unlink the socket if unix type):
		l.Close()
		// And we're done:
		os.Exit(0)
	}(sigc)

	err = http.Serve(l, serveMux())
	if err != nil {
		panic(err)
	}
}
