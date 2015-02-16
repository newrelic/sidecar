package main

import (
	"github.com/fsouza/go-dockerclient"
	"github.com/newrelic/bosun/service"
)

func containers() []service.Service {
	endpoint := "tcp://localhost:2375"
	client, _ := docker.NewClient(endpoint)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: false})
	if err != nil {
		return nil
	}

	var containerList []service.Service

	for _, container := range containers {
		containerList = append(containerList, service.ToService(container))
	}

	return containerList
}
