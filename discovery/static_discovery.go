package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/relistan/go-director"

	"github.com/Nitro/sidecar/service"
)

type Target struct {
	Service service.Service
	Check   StaticCheck
}

type StaticDiscovery struct {
	Targets    []*Target
	ConfigFile string
	Hostname   string
	DefaultIP  string
}

type StaticCheck struct {
	Type string
	Args string
}

func NewStaticDiscovery(filename string, defaultIP string) *StaticDiscovery {
	hostname, err := os.Hostname()
	if err != nil {
		log.Errorf("Error getting hostname! %s", err.Error())
	}
	return &StaticDiscovery{
		ConfigFile: filename,
		Hostname:   hostname,
		DefaultIP:  defaultIP,
	}
}

func (d *StaticDiscovery) HealthCheck(svc *service.Service) (string, string) {
	for _, target := range d.Targets {
		if svc.ID == target.Service.ID {
			return target.Check.Type, target.Check.Args
		}
	}
	return "", ""
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
func (d *StaticDiscovery) Run(looper director.Looper) {
	var err error

	d.Targets, err = d.ParseConfig(d.ConfigFile)
	if err != nil {
		log.Errorf("StaticDiscovery cannot parse: %s", err.Error())
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
		log.Errorf("Unable to read announcements file: '%s!'", err.Error())
		return nil, err
	}

	var targets []*Target
	json.Unmarshal(file, &targets)

	// Have to loop with traditional 'for' loop so we can modify entries
	for _, target := range targets {
		idBytes, err := RandomHex(6)
		if err != nil {
			log.Errorf("ParseConfig(): Unable to get random bytes (%s)", err.Error())
			return nil, err
		}

		target.Service.ID = string(idBytes)
		target.Service.Created = time.Now().UTC()
		// We _can_ export services for a 3rd party. If we don't specify
		// the hostname, then it's for this host.
		if target.Service.Hostname == "" {
			target.Service.Hostname = d.Hostname
		}

		// Make sure we have an IP address on ports
		for i, port := range target.Service.Ports {
			if len(port.IP) == 0 {
				target.Service.Ports[i].IP = d.DefaultIP
			}
		}

		log.Printf("Discovered service: %s, ID: %s",
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
		log.Errorf("RandomBytes(): Error %s", err.Error())
		return nil, err
	}

	encoded := make([]byte, count*2)
	hex.Encode(encoded, raw)
	return encoded, nil
}
