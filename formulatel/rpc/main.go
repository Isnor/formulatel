package main

import (
	"net"

	formulatel "github.com/isnor/formulatel/server"
)

func main() {
	server := formulatel.FormulaTelServer()

	listener, err := net.Listen("tcp", "0.0.0.0:29292")

	if err != nil {
		panic(err)
	}

	println("formulatel-rpc listening on 29292")
	server.Serve(listener)
}
