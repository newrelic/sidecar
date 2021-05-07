package healthy

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/NinesStack/sidecar/discovery"
	"github.com/NinesStack/sidecar/service"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

const (
	DEFAULT_STATUS_ENDPOINT = "/"
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
func (m *Monitor) defaultCheckForService(svc *service.Service) *Check {
	port := findFirstTCPPort(svc)
	if port == nil {
		return &Check{ID: svc.ID, Command: &AlwaysSuccessfulCmd{}}
	}

	// Use the const default unless we've been provided something else
	defaultCheckEndpoint := DEFAULT_STATUS_ENDPOINT
	if len(m.DefaultCheckEndpoint) != 0 {
		defaultCheckEndpoint = m.DefaultCheckEndpoint
	}

	url := fmt.Sprintf("http://%v:%v%v", m.DefaultCheckHost, port.Port, defaultCheckEndpoint)
	return &Check{
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
	case "AlwaysSuccessful":
		return &AlwaysSuccessfulCmd{}
	default:
		return &HttpGetCmd{}
	}
}

// Talks to a Discoverer and returns the configured check
func (m *Monitor) fetchCheckForService(svc *service.Service, disco discovery.Discoverer) *Check {

	check := &Check{}
	check.Type, check.Args = disco.HealthCheck(svc)
	if check.Type == "" {
		log.Warnf("Got empty check type for service %s (id: %s) with args: %s!", svc.Name, svc.ID, check.Args)
		return nil
	}

	// Setup some other parts of the check that don't come from discovery
	check.ID = svc.ID
	check.Command = m.GetCommandNamed(check.Type)
	check.Status = FAILED

	return check
}

// Use templating to substitute in some info about the service.  Important because
// we won't know the actual Port that the container will bind to, for example.
func (m *Monitor) templateCheckArgs(check *Check, svc *service.Service) string {
	funcMap := template.FuncMap{
		"tcp":       func(p int64) int64 { return svc.PortForServicePort(p, "tcp") },
		"udp":       func(p int64) int64 { return svc.PortForServicePort(p, "udp") },
		"host":      func() string { return m.DefaultCheckHost },
		"container": func() string { return svc.Hostname },
	}

	t, err := template.New("check").Funcs(funcMap).Parse(check.Args)
	if err != nil {
		log.Errorf("Unable to parse check Args: '%s'", check.Args)
		return check.Args
	}

	var output bytes.Buffer
	err = t.Execute(&output, svc)
	if err != nil {
		log.Errorf("Unable to execute template: '%s'", check.Args)
		return check.Args
	}

	return output.String()
}

// CheckForService returns a Check that has been properly configured for this
// particular service.
func (m *Monitor) CheckForService(svc *service.Service, disco discovery.Discoverer) *Check {
	check := m.fetchCheckForService(svc, disco)
	if check == nil { // We got nothing
		log.Warnf("Using default check for service %s (id: %s).", svc.Name, svc.ID)
		check = m.defaultCheckForService(svc)
	}

	check.Args = m.templateCheckArgs(check, svc)

	return check
}

// Watch loops over a list of services and adds checks for services we don't already
// know about. It then removes any checks for services which have gone away. All
// services are expected to be local to this node.
func (m *Monitor) Watch(disco discovery.Discoverer, looper director.Looper) {
	m.DiscoveryFn = disco.Services // Store this so we can use it from Services()

	looper.Loop(func() error {
		services := disco.Services()

		// Add checks when new services are found
		for _, svc := range services {
			if m.Checks[svc.ID] == nil {
				check := m.CheckForService(&svc, disco)
				if check.Command == nil {
					log.Errorf(
						"Attempted to add %s (id: %s) but no check configured!",
						svc.Name, svc.ID,
					)
				} else {
					m.AddCheck(check)
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
