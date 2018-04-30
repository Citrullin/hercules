package server

import (
	"log"
	"net"
	"runtime"
	"sync/atomic"
	"time"
)

const (
	flushInterval = time.Duration(1) * time.Second
	maxQueueSize  = 1000000
	UDPPacketSize = 1650
	maxNeighbors = 32
)

var ops uint64 = 0
var total uint64 = 0
var flushTicker *time.Ticker
var nbWorkers = runtime.NumCPU()

type Neighbor struct {
	Addr string
	UDPAddr *net.UDPAddr
}

type Message struct {
	Addr   string
	Msg    []byte
}

type ServerConfig struct {
	Neighbors []string
	Port string
}

type messageQueue chan *Message

type Server struct {
	Incoming messageQueue
	Outgoing messageQueue
}

func (mq messageQueue) enqueue(m *Message) {
	mq <- m
}

func (mq messageQueue) dequeue() {
	for m := range mq {
		handleMessage(m)
	}
}

var mq messageQueue
var server *Server
var config *ServerConfig
var neighbors []*Neighbor
var connection net.PacketConn
var ended = false

func Create (serverConfig *ServerConfig) *Server {
	// TODO: allow hostname neighbors, periodically check for changed IP
	//ip, err := net.LookupIP("192.168.1.1")

	config = serverConfig
	mq = make(messageQueue, maxQueueSize)
	server = &Server{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}

	neighbors = make([]*Neighbor, maxNeighbors)
	for i := range config.Neighbors {
		neighbors[i] = createNeighbor(config.Neighbors[i])
	}

	c, err := net.ListenPacket("udp", ":" + config.Port)
	if err != nil {
		panic(err)
	}
	connection = c
	server.listenAndReceive(nbWorkers / 2)

	flushTicker = time.NewTicker(flushInterval)
	go func() {
		for range flushTicker.C {
			if ended { break }
			log.Printf("iTXs/s %f", float64(ops)/flushInterval.Seconds())
			atomic.AddUint64(&total, ops)
			atomic.StoreUint64(&ops, 0)
		}
	}()

	go func() {
		for msg := range server.Outgoing {
			if ended { break }
			if len(msg.Addr) > 0 {
				neighbor := FindNeighbor(msg.Addr)
				if neighbor != nil {
					neighbor.Write(msg)
				}
			}
			server.Write(msg)
		}
	}()
	return server
}

func End () {
	ended = true
	time.Sleep(time.Duration(5) * time.Second)
	connection.Close()
	atomic.AddUint64(&total, ops)
	log.Printf("Total iTXs %d", total)
}

func FindNeighbor (address string) *Neighbor {
	for _, neighbor := range neighbors {
		if neighbor != nil && neighbor.Addr == address {
			return neighbor
		}
	}
	return nil
}

func createNeighbor (address string) *Neighbor {
	UDPAddr, _ := net.ResolveUDPAddr("udp", address)
 	neighbor := Neighbor{
 		Addr: address,
		UDPAddr: UDPAddr}
 	return &neighbor
}

func (neighbor Neighbor) Write(msg *Message) {
	_, err := connection.WriteTo(msg.Msg[0:], neighbor.UDPAddr)
	if err != nil {
		log.Fatalln("Error!", err)
	}
}

func (server Server) Write(msg *Message) {
	for _, neighbor := range neighbors {
		if neighbor != nil {
			neighbor.Write(msg)
		}
	}
}

func (server Server) listenAndReceive(maxWorkers int) error {
	for i := 0; i < maxWorkers; i++ {
		go mq.dequeue()
		go server.receive()
	}
	return nil
}

// receive accepts incoming datagrams on c and calls handleMessage() for each message
func (server Server) receive() {
	for !ended {
		msg := make([]byte, UDPPacketSize)
		_, addr, err := connection.ReadFrom(msg[0:])
		if err != nil {
			log.Printf("Error %s", err)
			continue
		}
		address := addr.String()
		neighbor := FindNeighbor(address)
		if neighbor != nil {
			mq.enqueue(&Message{address, msg})
		}
	}
}

func handleMessage(msg *Message) {
	server.Incoming <- msg
	atomic.AddUint64(&ops, 1)
}