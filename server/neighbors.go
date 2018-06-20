package server

import (
	"errors"
	"net"
	"strings"

	"../logs"
)

func AddNeighbor(address string) error {
	neighbor, err := createNeighbor(address)

	if err != nil {
		return err
	}

	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	neighborExists, _ := checkNeighbourExists(neighbor)
	if neighborExists {
		return errors.New("Neighbor already exists")
	}

	Neighbors[neighbor.Addr] = neighbor

	logAddNeighbor(neighbor)

	return nil
}

func logAddNeighbor(neighbor *Neighbor) {
	addingLogMessage := "Adding neighbor '%v://%v:%v'"
	if neighbor.Hostname != "" {
		addingLogMessage += " - hostname '%v'"
		logs.Log.Debugf(addingLogMessage, neighbor.ConnectionType, neighbor.Addr, neighbor.Port, neighbor.Hostname)
	} else {
		logs.Log.Debugf(addingLogMessage, neighbor.ConnectionType, neighbor.Addr, neighbor.Port)
	}
}

func RemoveNeighbor(address string) error {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	neighborExists, neighbor := checkNeighbourExistsByAddress(address)
	if neighborExists {
		delete(Neighbors, neighbor.Addr)
		return nil
	}

	return errors.New("Neighbor not found")
}

func TrackNeighbor(msg *NeighborTrackingMessage) {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	neighborExists, neighbor := checkNeighbourExistsByAddress(msg.Addr)
	if neighborExists {
		neighbor.Incoming += msg.Incoming
		neighbor.New += msg.New
		neighbor.Invalid += msg.Invalid
	}
}

func GetNeighborByAddress(address string) (string, *Neighbor) {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	return "", getNeighborByAddress(address)
}

func UpdateHostnameAddresses() {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	for _, neighbor := range Neighbors {
		isRegisteredWithHostname := len(neighbor.Hostname) > 0
		if isRegisteredWithHostname {
			logs.Log.Debugf("Checking '%v' with current IP address: '%v'", neighbor.Hostname, neighbor.IP)
			ip, _, _ := getIpAndHostname(neighbor.Addr)

			if neighbor.IP != ip {
				logs.Log.Debugf("Updated '%v' IP address to '%v'", neighbor.Hostname, neighbor.IP)
				neighbor.UDPAddr, _ = net.ResolveUDPAddr("udp", getFormattedAddress(neighbor.IP, neighbor.Port))
			}
		}
	}
}

func createNeighbor(address string) (*Neighbor, error) {
	connectionType, identifier, port, err := getConnectionTypeAndIdentifierAndPort(address)
	if err != nil {
		return nil, err
	}

	if connectionType != UDP {
		return nil, errors.New("This protocol is not supported yet")
	}

	ip, hostname, err := getIpAndHostname(identifier)
	if err != nil {
		return nil, err
	}

	neighbor := Neighbor{
		Hostname:       hostname,
		IP:             ip,
		Addr:           getFormattedAddress(identifier, port),
		Port:           port,
		ConnectionType: connectionType,
		Incoming:       0,
		New:            0,
		Invalid:        0,
	}

	if connectionType == UDP {
		neighbor.UDPAddr, _ = net.ResolveUDPAddr(UDP, getFormattedAddress(neighbor.IP, port))
	}

	return &neighbor, nil
}

func listenNeighborTracker() {
	for msg := range NeighborTrackingQueue {
		TrackNeighbor(msg)
	}
}

func getConnectionTypeAndIdentifierAndPort(address string) (connectionType string, identifier string, port string, e error) {
	addressWithoutConnectionType, connectionType := getConnectionType(address)
	identifier, port = getIdentifierAndPort(addressWithoutConnectionType)

	if connectionType == "" || identifier == "" || port == "" {
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
		connectionType = UDP // default if none is provided
		addressWithoutConnectionType = address
	}
	return
}

func getIdentifierAndPort(address string) (identifier string, port string) {
	tokens := strings.Split(address, ":")
	portIndex := len(tokens) - 1
	if portIndex > 0 {
		identifier = strings.Join(tokens[:portIndex], ":")
		port = tokens[portIndex]
	} else {
		identifier = address
		port = config.GetString("node.port") // Tries to use same port as this node
	}

	return identifier, port
}

func getIpAndHostname(identifier string) (ip string, hostname string, err error) {

	addr := net.ParseIP(identifier)
	isIpFormat := addr != nil
	if isIpFormat {
		return addr.String(), "", nil // leave hostname empty when its in IP format
	} else {
		// Probably domain name. Check it
		addresses, err := net.LookupHost(identifier)
		if err != nil {
			return "", "", errors.New("Could not process look up for " + identifier)
		}
		addressFound := len(addresses) > 0
		if addressFound {
			return addresses[0], identifier, nil
		} else {
			return "", "", errors.New("Could not resolve a hostname for " + identifier)
		}
	}
}

func getNeighborByAddress(address string) *Neighbor {
	_, neighbor := checkNeighbourExistsByAddress(address)
	return neighbor
}

func checkNeighbourExistsByAddress(address string) (neighborExists bool, neighbor *Neighbor) {
	_, identifier, port, _ := getConnectionTypeAndIdentifierAndPort(address)
	formattedAddress := getFormattedAddress(identifier, port)
	neighbor, neighborExists = Neighbors[formattedAddress]
	return
}

func checkNeighbourExists(candidateNeighbor *Neighbor) (bool, *Neighbor) {
	neighbor, neighborExists := Neighbors[candidateNeighbor.Addr]
	return neighborExists, neighbor
}

func getFormattedAddress(identifier string, port string) string {
	return identifier + ":" + port
}
