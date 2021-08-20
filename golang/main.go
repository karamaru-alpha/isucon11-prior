package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
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
		panic(err)
	}
	l, err := net.Listen("unix", fileName)
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}
	if err := os.Chmod(fileName, 0666); err != nil {
		panic(err)
	}

	err = http.Serve(l, serveMux())
	if err != nil {
		panic(err)
	}
}
