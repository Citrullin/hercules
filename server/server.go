package server

import (
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"../config"
	"../logs"
)

const (
	reportInterval          = time.Duration(1) * time.Second
	hostnameRefreshInterval = time.Duration(300) * time.Second
	maxQueueSize            = 1000000
	UDPPacketSize           = 1650
	UDP                     = "udp"
	TCP                     = "tcp"
)

var (
	incTxPerSec           uint64
	outTxPerSec           uint64
	totalIncTx            uint64
	nbWorkers             = runtime.NumCPU()
	reportTicker          *time.Ticker
	hostnameRefreshTicker *time.Ticker
	server                *Server
	Neighbors             map[string]*Neighbor
	NeighborsLock         = &sync.RWMutex{}
	connection            net.PacketConn
	ended                 = false
)

type Message struct {
	Neighbor *Neighbor
	Msg      []byte
}


type messageQueue chan *Message

type Server struct {
	Incoming messageQueue
	Outgoing messageQueue
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

func Start() {
	create()

	workers := 1
	if !config.AppConfig.GetBool("light") {
		workers = nbWorkers
	}
	server.listenAndReceive(workers)

	go reportIncomingMessages()
	go refreshHostnames()
	go writeMessages()
}

func create() *Server {
	server = &Server{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}

	Neighbors = make(map[string]*Neighbor)
	logs.Log.Debug("Initial neighbors", config.AppConfig.GetStringSlice("node.neighbors"))
	for _, address := range config.AppConfig.GetStringSlice("node.neighbors") {
		err := AddNeighbor(address)
		if err != nil {
			logs.Log.Warningf("Could not add neighbor '%v' (%v)", address, err)
		}
	}

	c, err := net.ListenPacket("udp", ":"+config.AppConfig.GetString("node.port"))
	if err != nil {
		panic(err)
	}
	connection = c
	return server
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

func handleMessage(msg *Message) {
	server.Incoming <- msg
	msg.Neighbor.TrackIncoming(1)
	atomic.AddUint64(&incTxPerSec, 1)
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
	logs.Log.Debug("Neighbor server exited")
}
