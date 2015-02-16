package main

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

func (container Service) Encode() ([]byte, error) {
	return json.Marshal(container)
}

func (container *Service) AliveOrDead() string {
	if container.Status == ALIVE {
		return "Alive"
	}

	return "Tombstone"
}

func (container *Service) Format() string {
	return fmt.Sprintf("      %s %-20s %-30s %20s %-9s\n",
				container.ID,
				container.Name,
				container.Image,
				container.Updated,
				container.AliveOrDead(),
	)
}

func (container *Service) Tombstone() {
	container.Status  = TOMBSTONE
	container.Updated = time.Now().UTC()
}

func Decode(data []byte) *Service {
	var container Service
	json.Unmarshal(data, &container)

	return &container
}

// Format an APIContainers struct into a more compact struct we
// can ship over the wire in a broadcast.
func toService(container docker.APIContainers) Service {
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

func containers() []Service {
	endpoint := "tcp://localhost:2375"
	client, _ := docker.NewClient(endpoint)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: false})
	if err != nil {
		return nil
	}

	var containerList []Service

	for _, container := range containers {
		containerList = append(containerList, toService(container))
	}

	return containerList
}
