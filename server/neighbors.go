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

	Neighbors[neighbor.IPAddressWithPort] = neighbor

	logAddNeighbor(neighbor)

	return nil
}

func logAddNeighbor(neighbor *Neighbor) {
	addingLogMessage := "Adding neighbor '%v://%v'"
	if neighbor.Hostname != "" {
		addingLogMessage += " - IP Address: '%v'"
		logs.Log.Debugf(addingLogMessage, neighbor.ConnectionType, neighbor.Addr, neighbor.IP)
	} else {
		logs.Log.Debugf(addingLogMessage, neighbor.ConnectionType, neighbor.Addr)
	}
}

func RemoveNeighbor(address string) error {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	neighborExists, neighbor := checkNeighbourExistsByAddress(address)
	if neighborExists {
		delete(Neighbors, neighbor.IPAddressWithPort)
		return nil
	}

	return errors.New("Neighbor not found")
}

func TrackNeighbor(msg *NeighborTrackingMessage) {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	neighborExists, neighbor := checkNeighbourExistsByIPAddressWithPort(msg.IPAddressWithPort)
	if neighborExists {
		neighbor.Incoming += msg.Incoming
		neighbor.New += msg.New
		neighbor.Invalid += msg.Invalid
	}
}

func GetNeighborByAddress(address string) *Neighbor {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	_, neighbor := checkNeighbourExistsByAddress(address)
	return neighbor
}

func GetNeighborByIPAddressWithPort(ipAddressWithPort string) *Neighbor {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	_, neighbor := checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort)
	return neighbor
}

func UpdateHostnameAddresses() {
	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()

	var neighborsToRemove []string
	var neighborsToAdd []*Neighbor

	for _, neighbor := range Neighbors {
		isRegisteredWithHostname := len(neighbor.Hostname) > 0
		if isRegisteredWithHostname {
			identifier, _ := getIdentifierAndPort(neighbor.Addr)
			ip, _, _ := getIPAndHostname(identifier)
			ipAddressWithPort := GetFormattedAddress(ip, neighbor.Port)

			if neighbor.IPAddressWithPort == ipAddressWithPort {
				logs.Log.Debugf("IP address for '%v' is up-to-date ('%v')", neighbor.Hostname, neighbor.IPAddressWithPort)
			} else {
				neighbor.UDPAddr, _ = net.ResolveUDPAddr("udp", ipAddressWithPort)
				neighborsToRemove = append(neighborsToRemove, neighbor.IPAddressWithPort)

				logs.Log.Debugf("Updated IP address for '%v' from '%v' to '%v'", neighbor.Hostname, neighbor.IPAddressWithPort, ipAddressWithPort)

				neighbor.IP = ip
				neighbor.IPAddressWithPort = ipAddressWithPort
				neighborsToAdd = append(neighborsToAdd, neighbor)
			}
		}
	}

	for _, neighbor := range neighborsToRemove {
		delete(Neighbors, neighbor)
	}

	for _, neighbor := range neighborsToAdd {
		Neighbors[neighbor.IPAddressWithPort] = neighbor
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

	ip, hostname, err := getIPAndHostname(identifier)
	if err != nil {
		return nil, err
	}

	neighbor := Neighbor{
		Hostname:          hostname,
		IP:                ip,
		Addr:              GetFormattedAddress(identifier, port),
		Port:              port,
		IPAddressWithPort: GetFormattedAddress(ip, port),
		ConnectionType:    connectionType,
		Incoming:          0,
		New:               0,
		Invalid:           0,
	}

	if connectionType == UDP {
		conType := connectionType
		neighbor.UDPAddr, err = net.ResolveUDPAddr(conType, neighbor.IPAddressWithPort)
		if err != nil {
			return nil, err
		}
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

func getIPAndHostname(identifier string) (ip string, hostname string, err error) {

	addr := net.ParseIP(identifier)
	isIPFormat := addr != nil
	if isIPFormat {
		return addr.String(), "", nil // leave hostname empty when its in IP format
	}

	// Probably domain name. Check it
	addresses, err := net.LookupHost(identifier)
	if err != nil {
		return "", "", errors.New("Could not process look up for " + identifier)
	}
	addressFound := len(addresses) > 0
	if addressFound {
		ip = addresses[0]
		if len(strings.Split(ip, ":")) > 1 {
			ip = "[" + ip + "]"
		}

		return ip, identifier, nil
	}

	return "", "", errors.New("Could not resolve a hostname for " + identifier)
}

func checkNeighbourExistsByAddress(address string) (neighborExists bool, neighbor *Neighbor) {
	_, identifier, port, _ := getConnectionTypeAndIdentifierAndPort(address)
	formattedAddress := GetFormattedAddress(identifier, port)

	for _, candidateNeighbor := range Neighbors {
		if candidateNeighbor.Addr == formattedAddress {
			return true, candidateNeighbor
		}
	}
	return false, nil
}

func checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort string) (neighborExists bool, neighbor *Neighbor) {
	neighbor, neighborExists = Neighbors[ipAddressWithPort]
	return
}

func checkNeighbourExists(neighbor *Neighbor) (bool, *Neighbor) {
	for _, candidateNeighbor := range Neighbors {
		if candidateNeighbor.Addr == neighbor.Addr {
			return true, candidateNeighbor
		}
	}
	return false, nil
}

func GetFormattedAddress(identifier string, port string) string {
	return identifier + ":" + port
}
