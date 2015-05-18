package healthy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/service"
)

const (
	DEFAULT_STATUS_ENDPOINT = "/status/check"
)

func (m *Monitor) Services() []service.Service {
	var svcList []service.Service

	if m.DiscoveryFn == nil {
		log.Errorf("Error: DiscoveryFn not defined!")
		return []service.Service{}
	}

	for _, svc := range m.DiscoveryFn() {
		if svc.ID == "" {
			log.Errorf("Error: monitor found empty service ID")
			continue
		}

		m.MarkService(&svc)
		svcList = append(svcList, svc)
	}

	return svcList
}

func findFirstTCPPort(svc *service.Service) *service.Port {
	for _, port := range svc.Ports {
		if port.Type == "tcp" {
			return &port
		}
	}
	return nil
}

// Configure a default check for a service. The default is to return an HTTP
// check on the first TCP port on the endpoint set in DEFAULT_STATUS_ENDPOINT.
func (m *Monitor) defaultCheckForService(svc *service.Service) Check {
	port := findFirstTCPPort(svc)
	if port == nil {
		return Check{ID: svc.ID}
	}

	url := fmt.Sprintf("http://%v:%v%v", m.DefaultCheckHost, port.Port, DEFAULT_STATUS_ENDPOINT)
	return Check{
		ID:      svc.ID,
		Type:    "HttpGet",
		Args:    url,
		Status:  FAILED,
		Command: &HttpGetCmd{},
	}
}

func (m *Monitor) GetCommandNamed(name string) Checker {
	switch name {
	case "HttpGet":
		return &HttpGetCmd{}
	case "External":
		return &ExternalCmd{}
	default:
		return &HttpGetCmd{}
	}
}

// Figure out the Tome URL for a service if we can
func (m *Monitor) urlForService(svc *service.Service) string {
	if m.TomeAddr == "" {
		return ""
	}

	if m.ServiceNameFn == nil {
		log.Errorf("No naming function defined!")
		return ""
	}

	svcName := m.ServiceNameFn(svc)
	if svcName == "" {
		return ""
	}

	return fmt.Sprintf("http://%s/checks/%s", m.TomeAddr, svcName)
}

// Talks to a Tome service and returns the configured check
func (m *Monitor) fetchCheckForService(svc *service.Service) Check {
	url := m.urlForService(svc)
	if url == "" {
		return Check{}
	}

	resp, err := http.Get(url)

	if err != nil {
		log.Errorf("Error fetching '%s'! (%s)", url, err.Error())
		return Check{}
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	var check Check
	err = json.Unmarshal(data, &check)

	if err != nil {
		log.Errorf("Error decoding check response! (%s)", err.Error())
		return Check{}
	}

	// Setup some other parts of the check that don't come from the JSON
	check.ID = svc.ID
	check.Command = m.GetCommandNamed(check.Type)
	check.Args = strings.Replace(
		check.Args,
		"%CHECK_ADDR%",
		m.DefaultCheckHost,
		1,
	)
	check.Status = FAILED

	return check
}

// CheckForService returns a Check that has been properly configured for this
// particular service.
func (m *Monitor) CheckForService(svc *service.Service) Check {
	check := m.fetchCheckForService(svc)
	if check.ID == "" { // We got nothing
		log.Warnf("Using default check for %s\n", svc.ID)
		return m.defaultCheckForService(svc)
	}

	return check
}

// Watch loops over a list of services and adds checks for services we don't already
// know about. It then removes any checks for services which have gone away. All
// services are expected to be local to this node.
func (m *Monitor) Watch(svcFun func() []service.Service, looper director.Looper) {
	m.DiscoveryFn = svcFun // Store this so we can use it from Services()

	looper.Loop(func() error {
		services := svcFun()

		// Add checks when new services are found
		for _, svc := range services {
			if m.Checks[svc.ID] == nil {
				check := m.CheckForService(&svc)
				if check.Command == nil {
					log.Errorf(
						"Attempted to add %s (id: %s) but no check configured!",
						svc.Name, svc.ID,
					)
				} else {
					m.AddCheck(&check)
				}
			}
		}

		m.Lock()
		defer m.Unlock()
	OUTER:
		// We remove checks when encountering a missing service. This
		// prevents us from storing up checks forever. This is the only
		// way we'll find out about a service going away.
		for _, check := range m.Checks {
			for _, svc := range services {
				// Continue if we have a matching service/check pair
				if svc.ID == check.ID {
					continue OUTER
				}
			}

			// Remove checks for services that are no longer running
			delete(m.Checks, check.ID)
		}

		return nil
	})
}
