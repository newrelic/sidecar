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
	ServiceList []*Target
}

func (d *StaticDiscovery) Services() []service.Service {
	return nil
}

func (d *StaticDiscovery) Run(quit chan bool) {

}

func (d *StaticDiscovery) ParseConfig(filename string) ([]Target, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("Unable to read announcements file: '%s!'", err.Error())
		return nil, err
	}

	var targets []Target
	json.Unmarshal(file, &targets)

	// Have to loop with traditional for loop so we can modify entries
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
