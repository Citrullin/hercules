package server

import (
	"strings"
	"testing"

	"../logs"

	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var expectedConnectionType = "udp"
var expectedIdentifier = "77.55.235.204"
var expectedHostname = "field.carriota.com"
var expectedPort = "443"
var invalidConnectionType = "tcp"

var addresses = []string{
	expectedIdentifier + ":" + expectedPort,
	expectedIdentifier,

	expectedConnectionType + "://" + expectedIdentifier,
	expectedConnectionType + "://" + expectedIdentifier + ":" + expectedPort,

	invalidConnectionType + "://" + expectedIdentifier + ":" + expectedPort,
}

func TestGetConnectionTypeAndIdentifierAndPort(t *testing.T) {
	restartConfig()

	for _, address := range addresses {
		logs.Log.Info("Running test with neighbor's address: " + address)
		connectionType, identifier, port, err := getConnectionTypeAndIdentifierAndPort(address)

		if connectionType != expectedConnectionType && connectionType != invalidConnectionType || err != nil || identifier != expectedIdentifier {
			t.Error("Not all URI parameters have been detected!")
		} else {

			if strings.Contains(address, expectedPort) {
				if port != expectedPort {
					t.Error("An invalid port was returned!")
				}
			} else {
				if port != config.GetString("node.port") {
					t.Error("An invalid port was returned!")
				}
			}
		}
	}
}

func TestAddNeighbor(t *testing.T) {

	restartConfig()

	Neighbors = make(map[string]*Neighbor)

	for _, address := range addresses {
		logs.Log.Info("Running test with neighbor's address: " + address)
		err := AddNeighbor(address)

		if strings.HasPrefix(address, invalidConnectionType) {
			if err == nil {
				t.Error("Added invalid neighbor!")
			}
		} else {
			if err != nil {
				t.Error("Could not add neighbor!")
			}

			for _, neighbor := range Neighbors {
				addr, _ := getConnectionType(address)
				if strings.Contains(address, expectedPort) {
					if neighbor.Addr != addr {
						t.Errorf("Add neighbor %v does not match with loaded %v", neighbor.Addr, addr)
					}
				} else {
					configPort := config.GetString("node.port")
					addressWithConfigPort := addr + ":" + configPort
					if neighbor.Addr != addressWithConfigPort {
						t.Errorf("Add neighbor %v does not match with loaded %v", neighbor.Addr, addressWithConfigPort)
					}
				}

			}

			err = RemoveNeighbor(address)

			if err != nil {
				t.Error("Error during test clean up")
			}
		}

		if len(Neighbors) > 0 {
			logs.Log.Fatal("Test clean up did not work as intended")
		}
	}

}

func TestCheckNeighbourExistsByIPAddress(t *testing.T) {
	restartConfig()

	Neighbors = make(map[string]*Neighbor)

	testAddresses := []string{
		expectedIdentifier + ":" + expectedPort,
		expectedHostname + ":" + expectedPort,
	}

	for _, address := range testAddresses {
		logs.Log.Info("Running test with neighbor's address: " + address)

		err := AddNeighbor(address)
		if err != nil {
			t.Error("Error during test set up")
		}

		_, identifier, port, err := getConnectionTypeAndIdentifierAndPort(address)
		if err != nil {
			t.Error("Error during test set up")
		}

		ip, _, err := getIPAndHostname(identifier)
		if err != nil {
			t.Error("Error during test set up")
		}

		ipWithPort := getFormattedAddress(ip, port)

		neighborsExists, neighbor := checkNeighbourExistsByIPAddress(ipWithPort)
		if !neighborsExists {
			t.Error("Neighbor does NOT exist!")
		} else {
			formattedAddress := getFormattedAddress(identifier, port)
			if neighbor.Addr != formattedAddress || neighbor.IP != ip || neighbor.Port != port {
				t.Error("Neighbor was found but it is NOT the same as the one which was searched for!")
			}
		}

		// Clean-up
		err = RemoveNeighbor(address)
		if err != nil {
			t.Error("Error during test clean up")
		}

		if len(Neighbors) > 0 {
			logs.Log.Fatal("Test clean up did not work as intended")
		}
	}
}

func TestRemoveNeighbor(t *testing.T) {

	restartConfig()

	Neighbors = make(map[string]*Neighbor)

	for _, address := range addresses {
		logs.Log.Info("Running test with neighbor's address: " + address)
		err := AddNeighbor(address)

		if !strings.HasPrefix(address, invalidConnectionType) && err != nil {
			t.Error("Error during test set up")
		}

		err = RemoveNeighbor(address)

		if strings.HasPrefix(address, invalidConnectionType) {
			if err == nil {
				t.Error("Removed invalid neighbor!")
			}
		} else {
			if err != nil {
				t.Error("Could not remove neighbor!")
			}
		}

		if len(Neighbors) > 0 {
			logs.Log.Fatal("Test did not work as intended")
		}
	}

}

func TestGetIpAndHostname(t *testing.T) {

	ip, hostname, err := getIPAndHostname(expectedHostname)

	if err != nil || ip == "" || hostname == "" {
		t.Error("Could not get IP and Hostname for " + expectedHostname)
	}
}

func restartConfig() {
	config = viper.New()
	flag.IntP("node.port", "u", 14600, "UDP Node port")
	flag.Parse()
	config.BindPFlags(flag.CommandLine)
}
