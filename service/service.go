package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
	"github.com/kennygrant/sanitize"
	"github.com/newrelic/sidecar/output"
)

const (
	ALIVE     = iota
	TOMBSTONE = iota
	UNHEALTHY = iota
	UNKNOWN   = iota
)

type Port struct {
	Type        string
	Port        int64
	ServicePort int64
}

type Service struct {
	ID       string
	Name     string
	Image    string
	Created  time.Time
	Hostname string
	Ports    []Port
	Updated  time.Time
	Profile  string
	Status   int
}

func (svc Service) Encode() ([]byte, error) {
	return json.Marshal(svc)
}

func (svc *Service) StatusString() string {
	return StatusString(svc.Status)
}

func (svc *Service) IsAlive() bool {
	return svc.Status == ALIVE
}

func (svc *Service) IsTombstone() bool {
	return svc.Status == TOMBSTONE
}

func (svc *Service) Invalidates(otherSvc *Service) bool {
	return otherSvc != nil && svc.Updated.After(otherSvc.Updated)
}

func (svc *Service) Format() string {
	var ports []string
	for _, port := range svc.Ports {
		ports = append(ports,
			fmt.Sprintf("%d->%d", port.ServicePort, port.Port),
		)
	}
	return fmt.Sprintf("      %s %-30s %-15s %-45s  %-15s %-9s\n",
		svc.ID,
		svc.Name,
		strings.Join(ports, ","),
		svc.Image,
		output.TimeAgo(svc.Updated, time.Now().UTC()),
		svc.StatusString(),
	)
}

func (svc *Service) Tombstone() {
	svc.Status = TOMBSTONE
	svc.Updated = time.Now().UTC()
}

// Look up a (usually Docker) mapped Port for a service by ServicePort
func (svc *Service) PortForServicePort(findPort int64, pType string) int64 {
	for _, port := range svc.Ports {
		if port.ServicePort == findPort && port.Type == pType {
			return port.Port
		}
	}

	log.Warnf("Unable to find ServicePort %d for service %s", findPort, svc.ID)
	return -1
}

func Decode(data []byte) *Service {
	var svc Service
	json.Unmarshal(data, &svc)

	return &svc
}

// Format an APIContainers struct into a more compact struct we
// can ship over the wire in a broadcast.
func ToService(container *docker.APIContainers) Service {
	var svc Service
	hostname, _ := os.Hostname()

	svc.ID = container.ID[0:12]   // Use short IDs
	svc.Name = container.Names[0] // Use the first name
	svc.Image = container.Image
	svc.Created = time.Unix(container.Created, 0).UTC()
	svc.Updated = time.Now().UTC()
	svc.Hostname = hostname
	svc.Status = ALIVE

	if _, ok := container.Labels["ServiceProfile"]; ok {
		svc.Profile = sanitize.Path(container.Labels["ServiceProfile"])
		svc.Profile = strings.Replace(svc.Profile, "/", "", -1)
	} else {
		svc.Profile = "default"
	}

	svc.Ports = make([]Port, 0)

	for _, port := range container.Ports {
		if port.PublicPort != 0 {
			svc.Ports = append(svc.Ports, buildPortFor(&port, container))
		}
	}

	return svc
}

func StatusString(status int) string {
	switch status {
	case ALIVE:
		return "Alive"
	case UNHEALTHY:
		return "Unhealthy"
	case UNKNOWN:
		return "Unknown"
	default:
		return "Tombstone"
	}
}

// Figure out the correct port configuration for a service
func buildPortFor(port *docker.APIPort, container *docker.APIContainers) Port {
	// We look up service port labels by convention in the format "ServicePort_80=8080"
	svcPortLabel := fmt.Sprintf("ServicePort_%d", port.PrivatePort)

	returnPort := Port{Port: port.PublicPort, Type: port.Type}

	if svcPort, ok := container.Labels[svcPortLabel]; ok {
		svcPortInt, err := strconv.Atoi(svcPort)
		if err != nil {
			log.Errorf("Error converting label value for %s to integer: %s",
				svcPortLabel,
				err.Error(),
			)
			return returnPort
		}

		// Everything was good, set the service port
		returnPort.ServicePort = int64(svcPortInt)
	}

	return returnPort
}
