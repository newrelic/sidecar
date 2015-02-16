package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/fsouza/go-dockerclient"
)

type ServiceContainer struct {
	ID string
	Name string
	Image string
	Created time.Time
	Hostname string
	Updated time.Time
}

func (container ServiceContainer) Encode() ([]byte, error) {
	return json.Marshal(container)
}

func Decode(data []byte) *ServiceContainer {
	var container ServiceContainer
	json.Unmarshal(data, &container)

	return &container
}

// Format an APIContainers struct into a more compact struct we
// can ship over the wire in a broadcast.
func toServiceContainer(container docker.APIContainers) ServiceContainer {
	var svcContainer ServiceContainer
	hostname, _ := os.Hostname()

	svcContainer.ID       = container.ID[0:7]  // Use short IDs
	svcContainer.Name     = container.Names[0] // Use the first name
	svcContainer.Image    = container.Image
	svcContainer.Created  = time.Unix(container.Created, 0).UTC()
	svcContainer.Updated  = time.Now().UTC()
	svcContainer.Hostname = hostname

	return svcContainer
}

func containers() []ServiceContainer {
	endpoint := "tcp://localhost:2375"
	client, _ := docker.NewClient(endpoint)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: false})
	if err != nil {
		return nil
	}

	var containerList []ServiceContainer

	for _, container := range containers {
		containerList = append(containerList, toServiceContainer(container))
	}

	return containerList
}
