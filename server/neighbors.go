package server

import (
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"../config"
	"../logs"
)

type IPAddress struct {
	ip string
}

func (i *IPAddress) IsIPv6() bool {
	return len(strings.Split(i.ip, ":")) > 1
}

func (i *IPAddress) String() string {
	if i.IsIPv6() {
		// IPv6
		return "[" + i.ip + "]"
	} else {
		return i.ip
	}
}

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

func (nb *Neighbor) Write(msg *Message) {
	if ended {
		return
	}
	_, err := connection.WriteTo(msg.Msg[0:], nb.UDPAddr)
	if err != nil {
		if !ended { // Check again
			logs.Log.Errorf("Error sending message to neighbor '%v': %v", nb.Addr, err)
		}
	} else {
		atomic.AddUint64(&outTxPerSec, 1)
	}
}

func (nb *Neighbor) GetPreferredIP() string {
	return GetPreferredIP(nb.KnownIPs, nb.PreferIPv6)
}

func (nb *Neighbor) UpdateIPAddressWithPort(ipAddressWithPort string) (changed bool) {

	if nb.IPAddressWithPort != ipAddressWithPort {
		for _, knownIP := range nb.KnownIPs {
			knownIPWithPort := GetFormattedAddress(knownIP.String(), nb.Port)
			if knownIPWithPort == ipAddressWithPort {
				logs.Log.Debugf("Updated IP address for '%v' from '%v' to '%v'", nb.Hostname, nb.IPAddressWithPort, ipAddressWithPort)
				NeighborsLock.Lock()
				delete(Neighbors, nb.IPAddressWithPort)
				nb.IPAddressWithPort = ipAddressWithPort
				Neighbors[nb.IPAddressWithPort] = nb
				NeighborsLock.Unlock()
				nb.PreferIPv6 = knownIP.IsIPv6()
				nb.IP = knownIP.String()
				nb.UDPAddr, _ = net.ResolveUDPAddr("udp", ipAddressWithPort)
				return true
			}
		}
	}
	return false
}

func AddNeighbor(address string) error {
	neighbor, err := createNeighbor(address)

	if err != nil {
		return err
	}

	NeighborsLock.RLock()
	neighborExists, _ := checkNeighbourExists(neighbor)
	NeighborsLock.RUnlock()
	if neighborExists {
		return errors.New("Neighbor already exists")
	}

	NeighborsLock.Lock()
	Neighbors[neighbor.IPAddressWithPort] = neighbor
	NeighborsLock.Unlock()

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
	NeighborsLock.RLock()
	neighborExists, neighbor := checkNeighbourExistsByAddress(address)
	NeighborsLock.RUnlock()
	if neighborExists {
		NeighborsLock.Lock()
		delete(Neighbors, neighbor.IPAddressWithPort)
		NeighborsLock.Unlock()
		return nil
	}

	return errors.New("Neighbor not found")
}

func TrackNeighbor(msg *NeighborTrackingMessage) {
	if msg.Neighbor != nil {
		msg.Neighbor.Incoming += msg.Incoming
		msg.Neighbor.New += msg.New
		msg.Neighbor.Invalid += msg.Invalid
	}
}

func GetNeighborByAddress(address string) *Neighbor {
	NeighborsLock.RLock()
	defer NeighborsLock.RUnlock()

	_, neighbor := checkNeighbourExistsByAddress(address)
	return neighbor
}

func GetNeighborByIPAddressWithPort(ipAddressWithPort string) *Neighbor {
	NeighborsLock.RLock()
	defer NeighborsLock.RUnlock()

	_, neighbor := checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort, false)
	return neighbor
}

func GetPreferredIP(knownIPs []*IPAddress, preferIPv6 bool) string {
	for _, ip := range knownIPs {
		if ip.IsIPv6() == preferIPv6 {
			return ip.String() // Returns IPv4 if IPv6 is not preferred, or IPv6 if otherwise
		}
	}

	if len(knownIPs) > 0 {
		return knownIPs[0].String() // Returns first IP if nothing preferred was found
	}

	return ""
}

func UpdateHostnameAddresses() {
	var neighborsToRemove []string
	var neighborsToAdd []*Neighbor

	NeighborsLock.RLock()
	for _, neighbor := range Neighbors {
		isRegisteredWithHostname := len(neighbor.Hostname) > 0
		if isRegisteredWithHostname {
			identifier, _ := getIdentifierAndPort(neighbor.Addr)
			neighbor.KnownIPs, _, _ = getIPsAndHostname(identifier)
			ip := neighbor.GetPreferredIP()
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
	NeighborsLock.RUnlock()

	NeighborsLock.Lock()
	defer NeighborsLock.Unlock()
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

	ips, hostname, err := getIPsAndHostname(identifier)
	if err != nil {
		return nil, err
	}

	ip := GetPreferredIP(ips, false)

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
		PreferIPv6:        false,
		KnownIPs:          ips,
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
		port = config.AppConfig.GetString("node.port") // Tries to use same port as this node
	}

	return identifier, port
}

func getIPsAndHostname(identifier string) (ips []*IPAddress, hostname string, err error) {
	addr := net.ParseIP(identifier)
	isIPFormat := addr != nil
	if isIPFormat {
		return []*IPAddress{&IPAddress{addr.String()}}, "", nil // leave hostname empty when its in IP format
	}

	// Probably domain name. Check it
	addresses, err := net.LookupHost(identifier)
	if err != nil {
		return nil, "", errors.New("Could not process look up for " + identifier)
	}

	addressFound := len(addresses) > 0
	if addressFound {
		for _, addr := range addresses {
			ips = append(ips, &IPAddress{addr})
		}

		return ips, identifier, nil
	}

	return nil, "", errors.New("Could not resolve a hostname for " + identifier)
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

func checkNeighbourExistsByIPAddressWithPort(ipAddressWithPort string, checkAllKnownAddresses bool) (neighborExists bool, neighbor *Neighbor) {
	neighbor, neighborExists = Neighbors[ipAddressWithPort]

	if !neighborExists && checkAllKnownAddresses {
		// Neighbor not found in Neighbors map, maybe another known address fits
		for _, candidateNeighbor := range Neighbors {
			for _, knownIP := range candidateNeighbor.KnownIPs {
				knownIPWithPort := GetFormattedAddress(knownIP.String(), candidateNeighbor.Port)
				if knownIPWithPort == ipAddressWithPort {
					return true, candidateNeighbor
				}
			}
		}
	}
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
