package server

import (
	"errors"
	"net"
	"strings"

	"../logs"
)

func AddNeighbor(address string) error {
	connectionType, identifier, port, err := getConnectionTypeAndAddressAndPort(address)

	if err != nil {
		return err
	}

	if connectionType != "udp" {
		return errors.New("This protocol is not supported yet")
	}

	addr := net.ParseIP(identifier)
	hostname := ""
	if addr == nil {
		// Probably hostname. Check it
		addresses, _ := net.LookupHost(identifier)
		if len(addresses) > 0 {
			hostname = identifier
			address = addresses[0] + ":" + port
		} else {
			return errors.New("Couldn't lookup host: " + address)
		}
	}

	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	for _, neighbor := range Neighbors {
		if neighbor.Addr == address || (len(hostname) > 0 && neighbor.Hostname == hostname) {
			return errors.New("Neighbor already exists")
		}
	}

	if len(hostname) > 0 {
		identifier = hostname
	}
	neighbor := createNeighbor(connectionType, address, hostname)
	Neighbors[identifier] = neighbor

	addingLogMessage := "Adding neighbor '%v://%v' with address:port '%v'"
	if neighbor.Hostname != "" {
		addingLogMessage += " and hostname '%v'"
		logs.Log.Debugf(addingLogMessage, neighbor.ConnectionType, identifier, neighbor.Addr, neighbor.Hostname)
	} else {
		logs.Log.Debugf(addingLogMessage, neighbor.ConnectionType, identifier, neighbor.Addr)
	}

	return nil
}

func RemoveNeighbor(address string) error {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	identifier, neighbor := getNeighborByAddress(address)
	if neighbor != nil {
		delete(Neighbors, identifier)
		return nil
	}

	return errors.New("Neighbor not found")
}

func TrackNeighbor(msg *NeighborTrackingMessage) {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	_, neighbor := getNeighborByAddress(msg.Addr)

	if neighbor != nil {
		neighbor.Incoming += msg.Incoming
		neighbor.New += msg.New
		neighbor.Invalid += msg.Invalid
	}
}

func GetNeighborByAddress(address string) (string, *Neighbor) {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	return getNeighborByAddress(address)
}

func UpdateHostnameAddresses() {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()
	for identifier, neighbor := range Neighbors {
		if len(neighbor.Hostname) > 0 {
			logs.Log.Debugf("Checking %v with current address: %v", identifier, neighbor.Addr)
			_, port := getAddressAndPort(neighbor.Addr)
			addresses, _ := net.LookupHost(neighbor.Hostname)
			if len(addresses) > 0 {
				neighbor.Addr = addresses[0] + ":" + port
				logs.Log.Debugf("Refreshed Hostname address for %v: %v", neighbor.Hostname, neighbor.Addr)
				neighbor.UDPAddr, _ = net.ResolveUDPAddr("udp", neighbor.Addr)
			}
		}
	}
}

func getNeighborByAddress(address string) (string, *Neighbor) {
	_, identifier, _, err := getConnectionTypeAndAddressAndPort(address)

	if err != nil {
		return "", nil
	}

	for id, neighbor := range Neighbors {
		if neighbor.Addr == address || neighbor.Hostname == identifier {
			return id, neighbor
		}
	}
	return "", nil
}

func createNeighbor(connectionType string, address string, hostname string) *Neighbor {
	UDPAddr, _ := net.ResolveUDPAddr("udp", address)
	neighbor := Neighbor{
		Addr:           address,
		Hostname:       hostname,
		UDPAddr:        UDPAddr,
		ConnectionType: connectionType,
		Incoming:       0,
		New:            0,
		Invalid:        0,
	}
	return &neighbor
}

func listenNeighborTracker() {
	for msg := range NeighborTrackingQueue {
		TrackNeighbor(msg)
	}
}
func getConnectionTypeAndAddressAndPort(address string) (connectionType string, addr string, port string, e error) {
	addressWithoutConnectionType, connectionType := getConnectionType(address)
	addr, port = getAddressAndPort(addressWithoutConnectionType)

	if connectionType == "" || addr == "" || port == "" {
		return "", "", "", errors.New("Address could not be loaded")
	}

	return
}

func getConnectionType(address string) (addressWithoutConnectionType string, connectionType string) {
	tokens := strings.Split(address, "://")
	addressAndPortIndex := len(tokens) - 1
	if addressAndPortIndex > 0 {
		connectionType = tokens[0]
		addressWithoutConnectionType = tokens[addressAndPortIndex]
	} else {
		connectionType = "udp" // default if none is provided
		addressWithoutConnectionType = address
	}
	return
}

func getAddressAndPort(address string) (addr string, port string) {
	tokens := strings.Split(address, ":")
	portIndex := len(tokens) - 1
	if portIndex > 0 {
		port = tokens[portIndex]
		addr = strings.Join(tokens[:portIndex], ":")
	} else {
		addr = address
		port = config.GetString("node.port") // Tries to use same port as this node
	}

	return addr, port
}
