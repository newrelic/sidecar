package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/newrelic/bosun/healthy"
	"github.com/newrelic/bosun/service"
)

type Target struct {
	Service service.Service
	Check healthy.Check
}

type StaticDiscovery struct {
	Targets []*Target
	ConfigFile string
	HostnameFn func() (string, error)
}

func NewStaticDiscovery(filename string) *StaticDiscovery {
	return &StaticDiscovery{
		ConfigFile: filename,
	    HostnameFn: os.Hostname,
	}
}

// Returns the list of services derived from the targets that were parsed
// out of the config file.
func (d *StaticDiscovery) Services() []service.Service {
	var services []service.Service
	for _, target := range d.Targets {
		target.Service.Updated = time.Now().UTC()
		services = append(services, target.Service)
	}
	return services
}

// Causes the configuration to be parsed and loaded. There is no background
// processing needed on an ongoing basis.
func (d *StaticDiscovery) Run(quit chan bool) {
	var err error

	d.Targets, err = d.ParseConfig(d.ConfigFile)
	if err != nil {
		log.Printf("ERROR StaticDiscovery cannot parse: %s\n", err.Error())
	}
}

// Parses a JSON config file containing an array of Targets. These are
// then augmented with a random hex ID and stamped with the current
// UTC time as the creation time. The same hex ID is applied to the Check
// and the Service to make sure that they are matched by the healthy
// package later on.
func (d *StaticDiscovery) ParseConfig(filename string) ([]*Target, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("Unable to read announcements file: '%s!'", err.Error())
		return nil, err
	}

	var targets []*Target
	json.Unmarshal(file, &targets)

	// Have to loop with traditional 'for' loop so we can modify entries
	for _, target := range targets {
		idBytes, err := RandomHex(6)
		if err != nil {
			log.Printf("ParseConfig(): Unable to get random bytes (%s)", err.Error())
			return nil, err
		}

		target.Service.ID = string(idBytes)
		target.Service.Created = time.Now().UTC()
		target.Service.Hostname, _ = d.HostnameFn()
		target.Check.ID = string(idBytes)
		log.Printf("Discovered service: %s, ID: %s\n",
			target.Service.Name,
			target.Service.ID,
		)
	}
	return targets, nil
}

// Return a defined number of random bytes as a slice
func RandomHex(count int) ([]byte, error) {
	raw := make([]byte, count)
	_, err := rand.Read(raw)
	if err != nil {
		log.Printf("RandomBytes(): Error ", err)
		return nil, err
	}

	encoded := make([]byte, count * 2)
	hex.Encode(encoded, raw)
	return encoded, nil
}
