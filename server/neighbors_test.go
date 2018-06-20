package server

import (
	"strings"
	"testing"

	"../logs"

	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var expectedConnectionType = "udp"
var expectedIdentifier = "field.carriota.com"
var expectedPort = "443"
var invalidConnectionType = "tcp"

var addresses = []string{
	invalidConnectionType + "://" + expectedIdentifier + ":" + expectedPort,

	expectedIdentifier,
	expectedIdentifier + ":" + expectedPort,
	expectedConnectionType + "://" + expectedIdentifier + ":" + expectedPort,
}

func TestGetConnectionTypeAndAddressAndPort(t *testing.T) {
	restartConfig()

	for _, address := range addresses {
		logs.Log.Info("Running test with neighbor's address: " + address)
		connectionType, identifier, port, err := getConnectionTypeAndAddressAndPort(address)

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

func restartConfig() {
	config = viper.New()
	flag.IntP("node.port", "u", 14600, "UDP Node port")
	flag.Parse()
	config.BindPFlags(flag.CommandLine)
}
