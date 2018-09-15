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
	IncTxPerSec               uint64
	NewTxPerSec               uint64
	KnownTxPerSec             uint64
	ValidTxPerSec             uint64
	TipReqPerSec              uint64
	outTxPerSec               uint64
	TotalIncTx                uint64
	nbWorkers                 = runtime.NumCPU()
	reportTicker              *time.Ticker
	reportTickerQuit          = make(chan struct{})
	hostnameRefreshTicker     *time.Ticker
	hostnameRefreshTickerQuit = make(chan struct{})
	server                    *Server
	Neighbors                 map[string]*Neighbor
	NeighborsLock             = &sync.RWMutex{}
	connection                net.PacketConn
	ended                     = false
)

type Message struct {
	Neighbor *Neighbor
	Msg      []byte
}

type RawMsg struct {
	Data *[]byte
	Addr *net.Addr
}

type Server struct {
	Incoming          chan *RawMsg
	Outgoing          chan *Message
	IncomingWaitGroup *sync.WaitGroup
	OutgoingWaitGroup *sync.WaitGroup
	receiveWaitGroup  *sync.WaitGroup
	IncomingQueueQuit chan struct{}
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

// receive accepts incoming datagrams and adds them to the Incoming queue
func (server Server) receive() {
	server.receiveWaitGroup.Add(1)
	defer server.receiveWaitGroup.Done()

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
		server.Incoming <- &RawMsg{Data: &msg, Addr: &addr}
	}
}

func Start() {
	create()

	go server.receive()

	go reportIncomingMessages()
	go refreshHostnames()
	go writeMessages()
}

func End() {
	ended = true

	if reportTicker != nil {
		reportTicker.Stop()
		close(reportTickerQuit)
	}
	if hostnameRefreshTicker != nil {
		hostnameRefreshTicker.Stop()
		close(hostnameRefreshTickerQuit)
	}

	connection.Close()
	server.receiveWaitGroup.Wait()

	close(server.IncomingQueueQuit)
	server.IncomingWaitGroup.Wait()
	close(server.Incoming)

	atomic.AddUint64(&TotalIncTx, IncTxPerSec)
	logs.Log.Debugf("Total Incoming TXs %d\n", TotalIncTx)
	logs.Log.Debug("Neighbor server exited")
}

func create() *Server {
	server = &Server{
		Incoming:          make(chan *RawMsg, maxQueueSize),
		Outgoing:          make(chan *Message, maxQueueSize),
		IncomingWaitGroup: &sync.WaitGroup{},
		OutgoingWaitGroup: &sync.WaitGroup{},
		receiveWaitGroup:  &sync.WaitGroup{},
		IncomingQueueQuit: make(chan struct{}),
	}

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
	server.OutgoingWaitGroup.Add(1)
	defer server.OutgoingWaitGroup.Done()

	for msg := range server.Outgoing {
		if msg.Neighbor != nil {
			go msg.Neighbor.Write(msg)
		} else {
			go server.Write(msg)
		}
	}
}

func reportIncomingMessages() {
	reportTicker = time.NewTicker(reportInterval)
	for {
		select {
		case <-reportTickerQuit:
			return

		case <-reportTicker.C:
			if ended {
				break
			}
			report()
			atomic.AddUint64(&TotalIncTx, IncTxPerSec)
			atomic.StoreUint64(&IncTxPerSec, 0)
			atomic.StoreUint64(&NewTxPerSec, 0)
			atomic.StoreUint64(&KnownTxPerSec, 0)
			atomic.StoreUint64(&ValidTxPerSec, 0)
			atomic.StoreUint64(&TipReqPerSec, 0)
			atomic.StoreUint64(&outTxPerSec, 0)
		}
	}
}

func refreshHostnames() {
	hostnameRefreshTicker = time.NewTicker(hostnameRefreshInterval)
	for {
		select {
		case <-hostnameRefreshTickerQuit:
			return

		case <-hostnameRefreshTicker.C:
			if ended {
				break
			}
			UpdateHostnameAddresses()
		}
	}
}

func report() {
	logs.Log.Debugf("Incoming TX/s: (All: %3d, Known: %3d, New: %3d, Valid: %3d, TipReq: %3d) // Outgoing TX/s: %4d\n", IncTxPerSec, KnownTxPerSec, NewTxPerSec, ValidTxPerSec, TipReqPerSec, outTxPerSec)
}

func GetServer() *Server {
	return server
}
