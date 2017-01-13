package discovery

import (
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/fsouza/go-dockerclient"
	"github.com/Nitro/sidecar/service"
	"github.com/relistan/go-director"
)

const (
	CACHE_DRAIN_INTERVAL = 10 * time.Minute // Drain the cache every 10 mins
)

type DockerClient interface {
	InspectContainer(id string) (*docker.Container, error)
	ListContainers(opts docker.ListContainersOptions) ([]docker.APIContainers, error)
	AddEventListener(listener chan<- *docker.APIEvents) error
	RemoveEventListener(listener chan *docker.APIEvents) error
	Ping() error
}

type DockerDiscovery struct {
	events         chan *docker.APIEvents       // Where events are announced to us
	endpoint       string                       // The Docker endpoint to talk to
	services       []*service.Service           // The list of services we know about
	ClientProvider func() (DockerClient, error) // Return the client we'll use to connect
	containerCache map[string]*docker.Container // Cache of inspected containers
	serviceNamer   ServiceNamer                 // The service namer implementation
	sync.RWMutex                                // Reader/Writer lock
}

func NewDockerDiscovery(endpoint string, svcNamer ServiceNamer) *DockerDiscovery {
	discovery := DockerDiscovery{
		endpoint:       endpoint,
		events:         make(chan *docker.APIEvents),
		containerCache: make(map[string]*docker.Container),
		serviceNamer:   svcNamer,
	}

	// Default to our own method for returning this
	discovery.ClientProvider = discovery.getDockerClient

	return &discovery
}

func (d *DockerDiscovery) getDockerClient() (DockerClient, error) {
	if d.endpoint != "" {
		client, err := docker.NewClient(d.endpoint)
		if err != nil {
			return nil, err
		}

		return client, nil
	}

	client, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	return client, nil
}

// HealthCheck looks up a health check using Docker container labels to
// pass the type of check and the arguments to pass to it.
func (d *DockerDiscovery) HealthCheck(svc *service.Service) (string, string) {
	container, err := d.inspectContainer(svc)
	if err != nil {
		return "", ""
	}

	return container.Config.Labels["HealthCheck"], container.Config.Labels["HealthCheckArgs"]
}

func (d *DockerDiscovery) inspectContainer(svc *service.Service) (*docker.Container, error) {
	// If we have it cached, return it!
	if container, ok := d.containerCache[svc.ID]; ok {
		return container, nil
	}

	// New connection every time
	client, err := d.ClientProvider()
	if err != nil {
		log.Errorf("Error when creating Docker client: %s\n", err.Error())
		return nil, err
	}

	container, err := client.InspectContainer(svc.ID)
	if err != nil {
		log.Errorf("Error inspecting container : %v\n", svc.ID)
		return nil, err
	}

	// Cache it for next time
	d.containerCache[svc.ID] = container

	return container, nil
}

// The main loop, poll for containers continuously.
func (d *DockerDiscovery) Run(looper director.Looper) {
	watchEventsQuit := make(chan bool)
	processEventsQuit := make(chan bool)
	drainCacheQuit := make(chan bool)

	go d.watchEvents(watchEventsQuit)
	go d.processEvents(processEventsQuit)
	go d.drainCache(drainCacheQuit)

	go func() {
		// Loop around fetching the whole container list
		looper.Loop(func() error {
			d.getContainers()
			return nil
		})

		// Propagate quit channel message
		go func() { watchEventsQuit <- true }()
		go func() { processEventsQuit <- true }()
		go func() { drainCacheQuit <- true }()
	}()
}

func (d *DockerDiscovery) Services() []service.Service {
	d.RLock()
	defer d.RUnlock()

	svcList := make([]service.Service, len(d.services))

	for i, svc := range d.services {
		svcList[i] = *svc
	}

	return svcList
}

func (d *DockerDiscovery) getContainers() {
	// New connection every time
	client, err := d.ClientProvider()
	if err != nil {
		log.Errorf("Error when creating Docker client: %s\n", err.Error())
		return
	}

	containers, err := client.ListContainers(docker.ListContainersOptions{All: false})
	if err != nil {
		return
	}

	d.Lock()
	defer d.Unlock()

	// Temporary set to track if we have seen a container (for cache pruning)
	containerMap := make(map[string]interface{})

	// Build up the service list, and prepare to prune the containerCache
	d.services = make([]*service.Service, 0, len(containers))
	for _, container := range containers {
		// Skip services that are purposely excluded from discovery.
		if container.Labels["SidecarDiscover"] == "false" {
			continue
		}

		svc := service.ToService(&container)
		svc.Name = d.serviceNamer.ServiceName(&container)
		d.services = append(d.services, &svc)
		containerMap[svc.ID] = true
	}

	d.pruneContainerCache(containerMap)
}

// Loop through the current cache and remove anything that has disappeared
func (d *DockerDiscovery) pruneContainerCache(liveContainers map[string]interface{}) {
	for id, _ := range d.containerCache {
		if _, ok := liveContainers[id]; !ok {
			delete(d.containerCache, id)
		}
	}
}

func (d *DockerDiscovery) watchEvents(quit chan bool) {
	client, err := d.ClientProvider()
	if err != nil {
		log.Errorf("Error when creating Docker client: %s\n", err.Error())
		return
	}
	client.AddEventListener(d.events)

	// Health check the connection and set it back up when it goes away.
	for {

		err := client.Ping()
		if err != nil {
			log.Warn("Lost connection to Docker, re-connecting")
			client.RemoveEventListener(d.events)
			d.events = make(chan *docker.APIEvents) // RemoveEventListener closes it

			client, err = docker.NewClient(d.endpoint)
			if err == nil {
				client.AddEventListener(d.events)
			} else {
				log.Error("Can't reconnect to Docker!")
			}
		}

		select {
		case <-quit:
			return
		default:
		}

		time.Sleep(SLEEP_INTERVAL)
	}
}

func (d *DockerDiscovery) handleEvent(event docker.APIEvents) {
	// We're only worried about stopping containers
	if event.Status == "die" || event.Status == "stop" {
		d.Lock()
		defer d.Unlock()

		for i, service := range d.services {
			if len(event.ID) < 12 {
				continue
			}
			if event.ID[:12] == service.ID {
				log.Printf("Deleting %s based on Docker '%s' event\n", service.ID, event.Status)
				// Delete the entry in the slice
				d.services[i] = nil
				d.services = append(d.services[:i], d.services[i+1:]...)
				// Once we found a match, return
				return
			}
		}
	}
}

func (d *DockerDiscovery) processEvents(quit chan bool) {
	for {
		select {
		case <-quit:
			return
		default:
		}

		event := <-d.events
		if event == nil {
			// This usually happens because of a Docker restart.
			// Sleep, let us reconnect in the background, then loop.
			time.Sleep(SLEEP_INTERVAL)
			continue
		}
		log.Debugf("Event: %#v\n", event)
		d.handleEvent(*event)
	}
}

// On a timed basis, drain the containerCache
func (d *DockerDiscovery) drainCache(quit chan bool) {
	for {
		select {
		case <-quit:
			return
		case <-time.After(CACHE_DRAIN_INTERVAL):
			log.Debug("Draining containerCache")
			d.Lock()
			// Make a new one, leave the old one for GC
			d.containerCache = make(
				map[string]*docker.Container,
				len(d.services),
			)
			d.Unlock()
		}
	}
}
