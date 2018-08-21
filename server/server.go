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
}

type Message struct {
	IPAddressWithPort string // Formatted like: XXX.XXX.XXX.XXX:x (IPv4) OR [x:x:x:...]:x (IPv6)
	Msg               []byte
}

type NeighborTrackingMessage struct {
	IPAddressWithPort string // Formatted like: XXX.XXX.XXX.XXX:x (IPv4) OR [x:x:x:...]:x (IPv6)
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

func Create(serverConfig *viper.Viper) *Server {
	config = serverConfig
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
	workers := 1
	if !config.GetBool("light") {
		workers = nbWorkers
	}
	server.listenAndReceive(workers)

	reportTicker = time.NewTicker(reportInterval)
	go func() {
		for range reportTicker.C {
			if ended {
				break
			}
			report()
			atomic.AddUint64(&totalIncTx, incTxPerSec)
			atomic.StoreUint64(&incTxPerSec, 0)
			atomic.StoreUint64(&outTxPerSec, 0)
		}
	}()

	hostnameRefreshTicker = time.NewTicker(hostnameRefreshInterval)
	go func() {
		for range hostnameRefreshTicker.C {
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
				neighborExists, neighbor := checkNeighbourExistsByIPAddressWithPort(msg.IPAddressWithPort, false)
				NeighborsLock.RUnlock()
				if neighborExists {
					go neighbor.Write(msg)
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
	atomic.AddUint64(&totalIncTx, incTxPerSec)
	logs.Log.Debugf("Total Incoming TXs %d\n", totalIncTx)
}

func (neighbor Neighbor) Write(msg *Message) {
	_, err := connection.WriteTo(msg.Msg[0:], neighbor.UDPAddr)
	if err != nil {
		logs.Log.Errorf("Error sending message to neighbor '%v': %v", neighbor.Addr, err)
	} else {
		atomic.AddUint64(&outTxPerSec, 1)
	}
}

func (server Server) Write(msg *Message) {
	NeighborsLock.RLock()
	defer NeighborsLock.RUnlock()

	for _, neighbor := range Neighbors {
		if neighbor != nil {
			go neighbor.Write(msg)
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
			logs.Log.Errorf("Error reading incoming packet: %v", err)
			continue
		}
		ipAddressWithPort := addr.String() // Format <ip>:<port>

		NeighborsLock.RLock()

		neighborExists, _ := checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort, false)
		if !neighborExists {
			// Check all known addresses => slower
			neighborExists, neighbor := checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort, true)

			NeighborsLock.RUnlock()

			if neighborExists {
				// If the neighbor was found now, the preferred IP is wrong => Update it!
				neighbor.UpdateIPAddressWithPort(ipAddressWithPort)
			}
		} else {
			NeighborsLock.RUnlock()
		}

		if neighborExists {
			handleMessage(&Message{IPAddressWithPort: ipAddressWithPort, Msg: msg})
		} else {
			logs.Log.Warningf("Received from an unknown neighbor (%v)", ipAddressWithPort)
		}
		time.Sleep(1)
	}
}

func handleMessage(msg *Message) {
	server.Incoming <- msg
	NeighborTrackingQueue <- &NeighborTrackingMessage{IPAddressWithPort: msg.IPAddressWithPort, Incoming: 1}
	atomic.AddUint64(&incTxPerSec, 1)
}

func report() {
	logs.Log.Debugf("Incoming TX/s: %d, Outgoing TX/s: %d\n", incTxPerSec, outTxPerSec)
}
