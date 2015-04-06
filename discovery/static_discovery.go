package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
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
}

func (d *StaticDiscovery) Services() []service.Service {
	var services []service.Service
	for _, target := range d.Targets {
		services = append(services, target.Service)
	}
	return services
}

func (d *StaticDiscovery) Run(quit chan bool) {
	//targets, err := d.ParseConfig("./static.json")
}

// Parses a JSON config file containing an array of Targets. These are
// then augmented with a random hex ID and stamped with the current
// UTC time as the creation time. The same hex ID is applied to the Check
// and the Service to make sure that they are matched by the healthy
// package later on.
func (d *StaticDiscovery) ParseConfig(filename string) ([]Target, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("Unable to read announcements file: '%s!'", err.Error())
		return nil, err
	}

	var targets []Target
	json.Unmarshal(file, &targets)

	// Have to loop with traditional 'for' loop so we can modify entries
	for i := 0; i < len(targets); i++ {
		idBytes, err := RandomHex(14)
		if err != nil {
			log.Printf("ParseConfig(): Unable to get random bytes (%s)", err.Error())
			return nil, err
		}

		targets[i].Service.ID = string(idBytes)
		targets[i].Service.Created = time.Now().UTC()
		targets[i].Check.ID = string(idBytes)
		log.Printf("Discovered service: %s, ID: %s\n",
			targets[i].Service.Name,
			targets[i].Service.ID,
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
