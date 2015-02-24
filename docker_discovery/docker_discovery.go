package docker_discovery

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/newrelic/bosun/service"
)

const (
	SLEEP_INTERVAL = 2 * time.Second
)

type DockerDiscovery struct {
	events chan *docker.APIEvents  // Where events are announced to us
	endpoint string                // The Docker endpoint to talk to
	containers []*service.Service  // The list of containers we know about
	containersLock sync.RWMutex    // Reader/Writer lock controlling .containers
}

func New(endpoint string) *DockerDiscovery {
	discovery := DockerDiscovery{endpoint: endpoint}
	discovery.events = make(chan *docker.APIEvents)
	return &discovery
}

func (d *DockerDiscovery) Run(quit chan bool) {
	getContainersQuit := make(chan bool)
	watchEventsQuit   := make(chan bool)
	processEventsQuit := make(chan bool)

	// Propagate quit channel message
	go func() {
		<-quit // Block on channel until we get a message
		go func() { getContainersQuit <-true }()
		go func() { watchEventsQuit <-true }()
		go func() { processEventsQuit <-true }()
	}()

	// Loop around fetching the whole container list
	go func() {
		for ;; {
			d.getContainers()
			select {
			case <- getContainersQuit:
				return
			default:
			}
			time.Sleep(SLEEP_INTERVAL)
		}
	}()

	go d.watchEvents(watchEventsQuit)
	go d.processEvents(processEventsQuit)
}

func (d *DockerDiscovery) Services() []service.Service {
	d.containersLock.RLock()
	defer d.containersLock.RUnlock()

	containerList := make([]service.Service, 0, len(d.containers))

	for _, container := range d.containers {
		containerList = append(containerList, *container)
	}

	return containerList
}

func (d *DockerDiscovery) getContainers() {
	// New connection every time
	client, _ := docker.NewClient(d.endpoint)
	containers, err := client.ListContainers(docker.ListContainersOptions{ All: false })
	if err != nil {
		return
	}

	d.containersLock.Lock()
	defer d.containersLock.Unlock()

	d.containers = make([]*service.Service, 0, len(containers))

	for _, container := range containers {
		svc := service.ToService(&container)
		d.containers = append(d.containers, &svc)
	}
}

func (d *DockerDiscovery) watchEvents(quit chan bool) {
	client, _ := docker.NewClient(d.endpoint)
	client.AddEventListener(d.events)

	// Health check the connection and set it back up when it goes away.
	for ;; {
		err := client.Ping()
		if err != nil {
			log.Println("Lost connection to Docker, re-connecting")
			client.RemoveEventListener(d.events)
			d.events  = make(chan *docker.APIEvents) // RemoveEventListener closes it
			client, _ = docker.NewClient(d.endpoint)
			client.AddEventListener(d.events)
		}
		select {
		case <- quit:
			return
		default:
		}
		time.Sleep(SLEEP_INTERVAL)
	}
}

func (d *DockerDiscovery) handleEvent(event *docker.APIEvents) {
	// We're only worried about stopping containers
	if event.Status == "die" || event.Status == "stop" {
		d.containersLock.Lock()
		defer d.containersLock.Unlock()

		for i, container := range d.containers {
			if event.ID[:12] == container.ID {
				log.Printf("Deleting %s based on event\n", container.ID)
				// Delete the entry in the slice
				d.containers[i] = nil
				d.containers = append(d.containers[:i], d.containers[i+1:]...)
				// Once we found a match, return
				return
			}
		}
	}
}

func (d *DockerDiscovery) processEvents(quit chan bool) {
	for ;; {
		select {
		case <- quit:
			return
		default:
		}

		event := <-d.events
		fmt.Printf("Event: %#v\n", event)
		if event == nil {
			continue
		}
		go d.handleEvent(event)
	}
}
