package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
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
	l, err := net.Listen("unix", "/tmp/go.sock")
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}

	err = http.Serve(l, serveMux())
	if err != nil {
		panic(err)
	}
}
