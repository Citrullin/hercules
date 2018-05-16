package server

import (
	"net"
	"runtime"
	"sync/atomic"
	"time"
	"sync"
	"logs"
)

const (
	flushInterval = time.Duration(1) * time.Second
	maxQueueSize  = 1000000
	UDPPacketSize = 1650
)

var ops uint64 = 0
var Speed uint64 = 1
var total uint64 = 0
var flushTicker *time.Ticker
var nbWorkers = runtime.NumCPU()

type Neighbor struct {
	Addr string
	UDPAddr *net.UDPAddr
	Incoming int
	New int
	Invalid int
}

type Message struct {
	Addr   string
	Msg    []byte
}

type NeighborTrackingMessage struct {
	Addr string
	Incoming int
	New int
	Invalid int
}

type ServerConfig struct {
	Neighbors []string
	Port string
}

type messageQueue chan *Message
type neighborTrackingQueue chan *NeighborTrackingMessage

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
var NeighborTrackingQueue neighborTrackingQueue
var NeighborsLock sync.RWMutex
var server *Server
var config *ServerConfig
var Neighbors map[string]*Neighbor
var connection net.PacketConn
var ended = false

func Create (serverConfig *ServerConfig) *Server {
	// TODO: add Hostname support
	// TODO: allow hostname Neighbors, periodically check for changed IP
	//ip, err := net.LookupIP("192.168.1.1")

	config = serverConfig
	mq = make(messageQueue, maxQueueSize)
	NeighborTrackingQueue = make(neighborTrackingQueue, maxQueueSize)
	server = &Server{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}

	Neighbors = make(map[string]*Neighbor)
	for _, v := range config.Neighbors {
		AddNeighbor(v)
	}

	c, err := net.ListenPacket("udp", ":" + config.Port)
	if err != nil {
		panic(err)
	}
	connection = c
	server.listenAndReceive(1)

	flushTicker = time.NewTicker(flushInterval)
	go func() {
		for range flushTicker.C {
			if ended { break }
			report()
			atomic.AddUint64(&total, ops)
			atomic.StoreUint64(&Speed, ops + 1)
			atomic.StoreUint64(&ops, 0)
		}
	}()

	go listenNeighborTracker()
	go func() {
		for msg := range server.Outgoing {
			if ended { break }
			if len(msg.Addr) > 0 {
				NeighborsLock.RLock()
				neighbor := Neighbors[msg.Addr]
				NeighborsLock.RUnlock()
				if neighbor != nil {
					neighbor.Write(msg)
				}
			} else {
				server.Write(msg)
			}
		}
	}()
	return server
}

func End () {
	ended = true
	time.Sleep(time.Duration(5) * time.Second)
	connection.Close()
	atomic.AddUint64(&total, ops)
	logs.Log.Debugf("Total iTXs %d\n", total)
}

func AddNeighbor (address string) int {
	if len(address) < 4 {
		return 0
	}
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	_, ok := Neighbors[address]
	if !ok {
		Neighbors[address] = createNeighbor(address)
		return 1
	}
	return 0
}

func RemoveNeighbor (address string) int {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	_, ok := Neighbors[address]
	if ok {
		delete(Neighbors, address)
		return 1
	}
	return 0
}

func createNeighbor (address string) *Neighbor {
	UDPAddr, _ := net.ResolveUDPAddr("udp", address)
 	neighbor := Neighbor{
 		Addr: address,
		UDPAddr: UDPAddr,
		Incoming: 0,
		New: 0,
		Invalid: 0,
	}
 	return &neighbor
}

func TrackNeighbor (msg *NeighborTrackingMessage) {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	neighbor, ok := Neighbors[msg.Addr]
	if ok && neighbor != nil {
		neighbor.Incoming += msg.Incoming
		neighbor.New += msg.New
		neighbor.Invalid += msg.Invalid
	}
}

func listenNeighborTracker () {
	for msg := range NeighborTrackingQueue {
		TrackNeighbor(msg)
	}
}

func (neighbor Neighbor) Write(msg *Message) {
	_, err := connection.WriteTo(msg.Msg[0:], neighbor.UDPAddr)
	if err != nil {
		logs.Log.Errorf("Error sending to neighbor %v: %v", neighbor.Addr, err)
	}
}

func (server Server) Write(msg *Message) {
	NeighborsLock.RLock()
	defer NeighborsLock.RUnlock()

	for _, neighbor := range Neighbors {
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
			logs.Log.Errorf("Error reading incoming packet: %v", err)
			continue
		}
		address := addr.String()
		NeighborsLock.RLock()
		neighbor := Neighbors[address]
		NeighborsLock.RUnlock()
		if neighbor != nil {
			mq.enqueue(&Message{address, msg})
		} else {
			//logs.Log.Warning("Received from an unknown neighbor", address)
		}
		time.Sleep(1)
	}
}

func handleMessage(msg *Message) {
	server.Incoming <- msg
	NeighborTrackingQueue <- &NeighborTrackingMessage{Addr: msg.Addr, Incoming: 1}
	atomic.AddUint64(&ops, 1)
}

func report () {
	logs.Log.Debugf("Incoming TX/s: %.2f\n", float64(ops))
}
