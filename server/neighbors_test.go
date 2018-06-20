package server

import (
	"strings"
	"testing"

	"gitlab.com/semkodev/hercules/logs"

	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var expectedConnectionType = "udp"
var expectedIdentifier = "iota.love"
var expectedPort = "15000"
var invalidConnectionType = "tcp"

var addresses = []string{
	invalidConnectionType + "://" + expectedIdentifier + ":" + expectedPort,

	expectedIdentifier,
	expectedIdentifier + ":" + expectedPort,
	expectedConnectionType + "://" + expectedIdentifier + ":" + expectedPort,
}

func TestGetConnectionTypeAndAddressAndPort(t *testing.T) {

	for _, address := range addresses {

		connectionType, identifier, port, err := getConnectionTypeAndAddressAndPort(address)

		if err != nil || connectionType != expectedConnectionType || identifier != expectedIdentifier || port != expectedPort {
			t.Error("Not all URI parameters have been detected!")
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
