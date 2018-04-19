package server

import (
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	flushInterval = time.Duration(1) * time.Second
	maxQueueSize  = 1000000
	UDPPacketSize = 1650
	maxNeighbors = 32
)

var address string
var remoteAddress string
var bufferPool sync.Pool
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
	Length int
}

type ServerConfig struct {
	Neighbors []string
	Port string
}

type messageQueue chan Message

type Server struct {
	Incoming messageQueue
	Outgoing messageQueue
}

func (mq messageQueue) enqueue(m Message) {
	mq <- m
}

func (mq messageQueue) dequeue() {
	for m := range mq {
		handleMessage(m)
		bufferPool.Put(m.Msg)
	}
}

var mq messageQueue
var mqo messageQueue
var server *Server
var config *ServerConfig
var neighbors []*Neighbor
var connection net.PacketConn

func Create (serverConfig *ServerConfig) *Server {
	config = serverConfig
	mq = make(messageQueue, maxQueueSize)
	mqo = make(messageQueue, maxQueueSize)
	server = &Server{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}

	runtime.GOMAXPROCS(runtime.NumCPU())
	neighbors = make([]*Neighbor, maxNeighbors)
	for i := range config.Neighbors {
		neighbors[i] = createNeighbor(config.Neighbors[i])
	}

	bufferPool = sync.Pool{
		New: func() interface{} { return make([]byte, UDPPacketSize) },
	}

	c, err := net.ListenPacket("udp", ":" + config.Port)
	if err != nil {
		panic(err)
	}
	connection = c
	server.listenAndReceive(nbWorkers)

	// Clean exit:
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		for range ch {
			atomic.AddUint64(&total, ops)
			log.Printf("Total ops %d", total)
			os.Exit(0)
		}
	}()

	flushTicker = time.NewTicker(flushInterval)
	go func() {
		for range flushTicker.C {
			log.Printf("Ops/s %f", float64(ops)/flushInterval.Seconds())
			atomic.AddUint64(&total, ops)
			atomic.StoreUint64(&ops, 0)
		}
	}()

	go func() {
		for msg := range server.Outgoing {
			if len(msg.Addr) > 0 {
				neighbor := FindNeighbor(msg.Addr)
				if neighbor != nil {
					neighbor.Write(msg)
				}
			}
			server.Write(msg)
		}
	}()
	server.Write(Message{ Msg: []byte("PING"), Length: 4 })
	return server
}

func FindNeighbor (address string) *Neighbor {
	for _, neighbor := range neighbors {
		if neighbor.Addr == address {
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

func (neighbor Neighbor) Write(msg Message) {
	connection.WriteTo(msg.Msg[0:msg.Length], neighbor.UDPAddr)
}

func (server Server) Write(msg Message) {
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
	for {
		msg := bufferPool.Get().([]byte)
		nbytes, addr, err := connection.ReadFrom(msg[0:])
		if err != nil {
			log.Printf("Error %s", err)
			continue
		}
		address := addr.String()
		neighbor := FindNeighbor(address)
		if neighbor != nil {
			mq.enqueue(Message{address, msg, nbytes})
		} else {
			bufferPool.Put(msg)
		}
	}
}

func handleMessage(msg Message) {
	server.Incoming <- msg
	atomic.AddUint64(&ops, 1)
}