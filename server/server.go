package server

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"../logs"
	"github.com/spf13/viper"
)

const (
	flushInterval           = time.Duration(1) * time.Second
	hostnameRefreshInterval = time.Duration(300) * time.Second
	maxQueueSize            = 1000000
	UDPPacketSize           = 1650
	UDP                     = "udp"
	TCP                     = "tcp"
)

type Neighbor struct {
	Hostname       string // Formatted like: <domainname> (Empty if its IP address)
	Addr           string // Formatted like: <ip>:<port> OR <domainname>:<port>
	IP             string // Formatted like: XXX.XXX.XXX.XXX OR [x:x:x:...] (IPv6)
	Port           string // Also saved separately from Addr for performance reasons
	UDPAddr        *net.UDPAddr
	Incoming       int
	New            int
	Invalid        int
	ConnectionType string // Formatted like: udp
}

type Message struct {
	IPAddressWithPort string // Formatted like: XXX.XXX.XXX.XXX OR [x:x:x:...]:x (IPv6)
	Msg               []byte
}

type NeighborTrackingMessage struct {
	IPAddressWithPort string
	Incoming          int
	New               int
	Invalid           int
}

type messageQueue chan *Message
type neighborTrackingQueue chan *NeighborTrackingMessage

type Server struct {
	Incoming messageQueue
	Outgoing messageQueue
}

var ops uint64 = 0
var Speed uint64 = 1
var total uint64 = 0
var flushTicker *time.Ticker
var hostnameTicker *time.Ticker

//var nbWorkers = runtime.NumCPU()
var mq messageQueue
var NeighborTrackingQueue neighborTrackingQueue
var NeighborsLock sync.RWMutex
var server *Server
var config *viper.Viper
var Neighbors map[string]*Neighbor
var connection net.PacketConn
var ended = false

func Create(serverConfig *viper.Viper) *Server {
	config = serverConfig
	mq = make(messageQueue, maxQueueSize)
	NeighborTrackingQueue = make(neighborTrackingQueue, maxQueueSize)
	server = &Server{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}

	Neighbors = make(map[string]*Neighbor)
	logs.Log.Debug("Initial neighbors", config.GetStringSlice("node.neighbors"))
	for _, address := range config.GetStringSlice("node.neighbors") {
		err := AddNeighbor(address)
		if err != nil {
			logs.Log.Warningf("Could not add neighbor '%v' (%v)", address, err)
		}
	}

	c, err := net.ListenPacket("udp", ":"+config.GetString("node.port"))
	if err != nil {
		panic(err)
	}
	connection = c
	return server
}

func Start() {
	server.listenAndReceive(1)

	flushTicker = time.NewTicker(flushInterval)
	go func() {
		for range flushTicker.C {
			if ended {
				break
			}
			report()
			atomic.AddUint64(&total, ops)
			atomic.StoreUint64(&Speed, ops+1)
			atomic.StoreUint64(&ops, 0)
		}
	}()

	hostnameTicker = time.NewTicker(hostnameRefreshInterval)
	go func() {
		for range hostnameTicker.C {
			if ended {
				break
			}
			UpdateHostnameAddresses()
		}
	}()

	go listenNeighborTracker()
	go func() {
		for msg := range server.Outgoing {
			if ended {
				break
			}
			if len(msg.IPAddressWithPort) > 0 {
				NeighborsLock.RLock()
				neighborExists, neighbor := checkNeighbourExistsByIPAddress(msg.IPAddressWithPort)
				NeighborsLock.RUnlock()
				if neighborExists {
					neighbor.Write(msg)
				}
			} else {
				server.Write(msg)
			}
		}
	}()
}

func End() {
	ended = true
	time.Sleep(time.Duration(5) * time.Second)
	connection.Close()
	atomic.AddUint64(&total, ops)
	logs.Log.Debugf("Total iTXs %d\n", total)
}

func (neighbor Neighbor) Write(msg *Message) {
	_, err := connection.WriteTo(msg.Msg[0:], neighbor.UDPAddr)
	if err != nil {
		logs.Log.Errorf("Error sending message to neighbor '%v': %v", neighbor.Addr, err)
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

func (mq messageQueue) enqueue(m *Message) {
	mq <- m
}

func (mq messageQueue) dequeue() {
	for m := range mq {
		handleMessage(m)
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
		ipAddressWithPort := addr.String() // Format <ip>:<port>
		NeighborsLock.RLock()
		neighborExists, _ := checkNeighbourExistsByIPAddress(ipAddressWithPort)
		NeighborsLock.RUnlock()
		if neighborExists {
			mq.enqueue(&Message{IPAddressWithPort: ipAddressWithPort, Msg: msg})
		} else {
			//logs.Log.Warning("Received from an unknown neighbor", address)
		}
		time.Sleep(1)
	}
}

func handleMessage(msg *Message) {
	server.Incoming <- msg
	NeighborTrackingQueue <- &NeighborTrackingMessage{IPAddressWithPort: msg.IPAddressWithPort, Incoming: 1}
	atomic.AddUint64(&ops, 1)
}

func report() {
	logs.Log.Debugf("Incoming TX/s: %.2f\n", float64(ops))
}
