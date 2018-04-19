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
	UDPPacketSize = 1500
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
var neighbors []Neighbor

func Create (serverConfig *ServerConfig) *Server {
	config = serverConfig
	mq = make(messageQueue, maxQueueSize)
	mqo = make(messageQueue, maxQueueSize)
	server = &Server{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}

	runtime.GOMAXPROCS(runtime.NumCPU())

	bufferPool = sync.Pool{
		New: func() interface{} { return make([]byte, UDPPacketSize) },
	}

	remote, _ := net.ResolveUDPAddr("udp", config.Neighbors[0])
	local, _ := net.ResolveUDPAddr("udp", ":" + config.Port)
	c, err := net.DialUDP("udp", local, remote)
	if err != nil {
		panic(err)
	}
	server.listenAndReceive(c, nbWorkers)

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
			server.Write(c, msg)
		}
	}()
	server.Write(c, Message{ Addr: config.Neighbors[0], Msg: []byte("PING"), Length: 4 })
	return server
}

func createNeighbor (address string) Neighbor {
 return Neighbor{}
}

func (server Server) Write(c *net.UDPConn, msg Message) {
	c.Write(msg.Msg[0:msg.Length])
}

func (server Server) listenAndReceive(c *net.UDPConn, maxWorkers int) error {
	for i := 0; i < maxWorkers; i++ {
		go mq.dequeue()
		go server.receive(c)
	}
	return nil
}

// receive accepts incoming datagrams on c and calls handleMessage() for each message
func (server Server) receive(c *net.UDPConn) {
	for {
		msg := bufferPool.Get().([]byte)
		nbytes, addr, err := c.ReadFrom(msg[0:])
		if err != nil {
			log.Printf("Error %s", err)
			continue
		}
		mq.enqueue(Message{addr.String(), msg, nbytes})
	}
}

func handleMessage(msg Message) {
	server.Incoming <- msg
	atomic.AddUint64(&ops, 1)
}