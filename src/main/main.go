package main

import (
	"server"
	"flag"
	"strings"
)

var serverConfig *server.ServerConfig
var srv *server.Server

func init() {
	var ns string
	var port string
	flag.StringVar(&port, "p", "14600", "Node Port to listen to")
	flag.StringVar(&ns, "n", "", "Initial Node neighbors")

	flag.Parse()

	neighbors := strings.Split(ns, ",")
	for i := range neighbors {
		neighbors[i] = strings.TrimSpace(neighbors[i])
	}
	serverConfig = &server.ServerConfig{
		Neighbors: neighbors,
		Port: port}
}

func main () {
	srv = server.Create(serverConfig)
	listenForIncomingMessage()
}

func listenForIncomingMessage () {
	for inc := range srv.Incoming {
		srv.Outgoing <- server.Message{
			Addr: inc.Addr,
			Msg: []byte("Pong"),
			Length: 4}
	}
}