package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/newrelic/bosun/output"
	"github.com/fsouza/go-dockerclient"
)

const (
	ALIVE = iota
	TOMBSTONE = iota
)

type Port struct {
	Type string
	Port int64
}

type Service struct {
	ID string
	Name string
	Image string
	Created time.Time
	Hostname string
	Ports []Port
	Updated time.Time
	Status int
}

func (svc Service) Encode() ([]byte, error) {
	return json.Marshal(svc)
}

func (svc *Service) AliveOrDead() string {
	if svc.Status == ALIVE {
		return "Alive"
	}

	return "Tombstone"
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
		ports = append(ports, strconv.FormatInt(port.Port, 10))
	}
	return fmt.Sprintf("      %s %-30s %-15s %-45s  %-15s %-9s\n",
				svc.ID,
				svc.Name,
				strings.Join(ports, ","),
				svc.Image,
				output.TimeAgo(svc.Updated, time.Now().UTC()),
				svc.AliveOrDead(),
	)
}

func (svc *Service) Tombstone() {
	svc.Status  = TOMBSTONE
	svc.Updated = time.Now().UTC()
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

	svc.ID       = container.ID[0:12]  // Use short IDs
	svc.Name     = container.Names[0] // Use the first name
	svc.Image    = container.Image
	svc.Created  = time.Unix(container.Created, 0).UTC()
	svc.Updated  = time.Now().UTC()
	svc.Hostname = hostname
	svc.Status   = ALIVE

	svc.Ports = make([]Port, 0)

	for _, port := range container.Ports {
		if port.PublicPort != 0 {
			svc.Ports = append(svc.Ports, Port{Port: port.PublicPort, Type: port.Type})
		}
	}

	return svc
}
