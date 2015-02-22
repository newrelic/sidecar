package service

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/fsouza/go-dockerclient"
)

const (
	ALIVE = iota
	TOMBSTONE = iota
)

type Service struct {
	ID string
	Name string
	Image string
	Created time.Time
	Hostname string
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
	return fmt.Sprintf("      %s %-20s %-30s %20s %-9s\n",
				svc.ID,
				svc.Name,
				svc.Image,
				svc.Updated,
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
func ToService(container docker.APIContainers) Service {
	var svcContainer Service
	hostname, _ := os.Hostname()

	svcContainer.ID       = container.ID[0:7]  // Use short IDs
	svcContainer.Name     = container.Names[0] // Use the first name
	svcContainer.Image    = container.Image
	svcContainer.Created  = time.Unix(container.Created, 0).UTC()
	svcContainer.Updated  = time.Now().UTC()
	svcContainer.Hostname = hostname
	svcContainer.Status   = ALIVE

	return svcContainer
}
