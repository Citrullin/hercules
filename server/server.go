package server

import (
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"../logs"
	"github.com/spf13/viper"
)

const (
	reportInterval          = time.Duration(1) * time.Second
	hostnameRefreshInterval = time.Duration(300) * time.Second
	maxQueueSize            = 1000000
	UDPPacketSize           = 1650
	UDP                     = "udp"
	TCP                     = "tcp"
)

type Neighbor struct {
	Hostname          string // Formatted like: <domainname> (Empty if its IP address)
	Addr              string // Formatted like: <ip>:<port> OR <domainname>:<port>
	IP                string // Formatted like: XXX.XXX.XXX.XXX (IPv4) OR [x:x:x:...] (IPv6)
	Port              string // Also saved separately from Addr for performance reasons
	IPAddressWithPort string // Formatted like: XXX.XXX.XXX.XXX:x (IPv4) OR [x:x:x:...]:x (IPv6)
	UDPAddr           *net.UDPAddr
	Incoming          int
	New               int
	Invalid           int
	ConnectionType    string // Formatted like: udp
	PreferIPv6        bool
	KnownIPs          []*IPAddress
	LastIncomingTime  time.Time
}

type Message struct {
	Neighbor *Neighbor
	Msg      []byte
}

type NeighborTrackingMessage struct {
	Neighbor *Neighbor
	Incoming int
	New      int
	Invalid  int
}

type messageQueue chan *Message
type neighborTrackingQueue chan *NeighborTrackingMessage

type Server struct {
	Incoming messageQueue
	Outgoing messageQueue
}

var incTxPerSec uint64
var outTxPerSec uint64
var totalIncTx uint64
var nbWorkers = runtime.NumCPU()
var reportTicker *time.Ticker
var hostnameRefreshTicker *time.Ticker
var NeighborTrackingQueue neighborTrackingQueue
var server *Server
var config *viper.Viper
var Neighbors map[string]*Neighbor
var NeighborsLock = &sync.RWMutex{}
var connection net.PacketConn
var ended = false

func create() *Server {
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

func Start(serverConfig *viper.Viper) {
	config = serverConfig

	create()

	workers := 1
	if !config.GetBool("light") {
		workers = nbWorkers
	}
	server.listenAndReceive(workers)

	go reportIncomingMessages()
	go refreshHostnames()
	go listenNeighborTracker()
	go writeMessages()
}

func reportIncomingMessages() {
	reportTicker = time.NewTicker(reportInterval)
	for range reportTicker.C {
		if ended {
			break
		}
		report()
		atomic.AddUint64(&totalIncTx, incTxPerSec)
		atomic.StoreUint64(&incTxPerSec, 0)
		atomic.StoreUint64(&outTxPerSec, 0)
	}
}

func refreshHostnames() {
	hostnameRefreshTicker = time.NewTicker(hostnameRefreshInterval)
	for range hostnameRefreshTicker.C {
		if ended {
			break
		}
		UpdateHostnameAddresses()
	}
}

func writeMessages() {
	for msg := range server.Outgoing {
		if ended {
			break
		}
		if msg.Neighbor != nil {
			go msg.Neighbor.Write(msg)
		} else {
			go server.Write(msg)
		}
	}
}

func (neighbor Neighbor) Write(msg *Message) {
	if ended {
		return
	}
	_, err := connection.WriteTo(msg.Msg[0:], neighbor.UDPAddr)
	if err != nil {
		if !ended { // Check again
			logs.Log.Errorf("Error sending message to neighbor '%v': %v", neighbor.Addr, err)
		}
	} else {
		atomic.AddUint64(&outTxPerSec, 1)
	}
}

func (server Server) Write(msg *Message) {
	if ended {
		return
	}

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
			// Check again (there might be messages received before ending)
			if !ended {
				logs.Log.Errorf("Error reading incoming packet: %v", err)
			}
			continue
		}
		ipAddressWithPort := addr.String() // Format <ip>:<port>

		NeighborsLock.RLock()

		var neighbor *Neighbor
		neighborExists, _ := checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort, false)
		if !neighborExists {
			// Check all known addresses => slower

			neighborExists, neighbor = checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort, true)

			NeighborsLock.RUnlock()

			if neighborExists {
				// If the neighbor was found now, the preferred IP is wrong => Update it!
				neighbor.UpdateIPAddressWithPort(ipAddressWithPort)
			}
		} else {
			NeighborsLock.RUnlock()
		}

		if neighborExists {
			handleMessage(&Message{Neighbor: neighbor, Msg: msg})
		} else {
			logs.Log.Warningf("Received from an unknown neighbor (%v)", ipAddressWithPort)
		}
		time.Sleep(1)
	}
}

func handleMessage(msg *Message) {
	server.Incoming <- msg
	NeighborTrackingQueue <- &NeighborTrackingMessage{Neighbor: msg.Neighbor, Incoming: 1}
	atomic.AddUint64(&incTxPerSec, 1)
}

func report() {
	logs.Log.Debugf("Incoming TX/s: %d, Outgoing TX/s: %d\n", incTxPerSec, outTxPerSec)
}

func GetServer() *Server {
	return server
}

func End() {
	ended = true
	connection.Close()
	atomic.AddUint64(&totalIncTx, incTxPerSec)
	logs.Log.Debugf("Total Incoming TXs %d\n", totalIncTx)
	logs.Log.Info("Neighbor server exited")
}
