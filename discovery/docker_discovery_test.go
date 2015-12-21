package discovery

import (
	"errors"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/newrelic/sidecar/service"
	. "github.com/smartystreets/goconvey/convey"
)

var hostname = "shakespeare"

// Define a stubDockerClient that we can use to test the discovery
type stubDockerClient struct {
	ErrorOnInspectContainer bool
}

func (s *stubDockerClient) InspectContainer(id string) (*docker.Container, error) {
	if s.ErrorOnInspectContainer {
		return nil, errors.New("Oh no!")
	}

	// If we match this ID, return a real setup
	if id == "deadbeef1231" { // svcId1
		return &docker.Container{
			Config: &docker.Config{
				Labels: map[string]string{
					"HealthCheck":     "HttpGet",
					"HealthCheckArgs": "service1 check arguments",
				},
			},
		}, nil
	}

	// Otherwise return an empty one
	return &docker.Container{
		Config: &docker.Config{
			Labels: map[string]string{},
		},
	}, nil
}

func (s *stubDockerClient) ListContainers(opts docker.ListContainersOptions) ([]docker.APIContainers, error) {
	return nil, nil
}

func (s *stubDockerClient) AddEventListener(listener chan<- *docker.APIEvents) error {
	return nil
}

func (s *stubDockerClient) RemoveEventListener(listener chan *docker.APIEvents) error {
	return nil
}

func (s *stubDockerClient) Ping() error { return nil }

func Test_DockerDiscovery(t *testing.T) {

	Convey("Working with Docker containers", t, func() {
		endpoint := "http://example.com:2375"
		svcId1 := "deadbeef1231"
		svcId2 := "deadbeef1011"
		baseTime := time.Now().UTC().Round(time.Second)
		service1 := service.Service{ID: svcId1, Hostname: hostname, Updated: baseTime}
		service2 := service.Service{ID: svcId2, Hostname: hostname, Updated: baseTime}
		services := []*service.Service{&service1, &service2}

		stubClientProvider := func() (DockerClient, error) {
			return &stubDockerClient{
				ErrorOnInspectContainer: false,
			}, nil
		}

		disco := NewDockerDiscovery(endpoint)
		disco.ClientProvider = stubClientProvider

		Convey("New() configures an endpoint and events channel", func() {
			So(disco.endpoint, ShouldEqual, endpoint)
			So(disco.events, ShouldNotBeNil)
		})

		Convey("Services() returns the right list of services", func() {
			disco.services = services

			processed := disco.Services()
			So(processed[0].Format(), ShouldEqual, service1.Format())
			So(processed[1].Format(), ShouldEqual, service2.Format())
		})

		Convey("handleEvents() prunes dead containers", func() {
			disco.services = services
			disco.handleEvent(docker.APIEvents{ID: svcId1, Status: "die"})

			result := disco.Services()
			So(len(result), ShouldEqual, 1)
			So(result[0].Format(), ShouldEqual, service2.Format())
		})

		Convey("HealthCheck()", func() {
			Convey("returns a valid health check when it's defined", func() {
				check, args := disco.HealthCheck(&service1)
				So(check, ShouldEqual, "HttpGet")
				So(args, ShouldEqual, "service1 check arguments")
			})

			Convey("returns and empty health check when undefined", func() {
				check, args := disco.HealthCheck(&service2)
				So(check, ShouldEqual, "")
				So(args, ShouldEqual, "")
			})

			Convey("handles errors from the Docker client", func() {
				disco.ClientProvider = func() (DockerClient, error) {
					return &stubDockerClient{
						ErrorOnInspectContainer: true,
					}, nil
				}

				check, args := disco.HealthCheck(&service2)
				So(check, ShouldEqual, "")
				So(args, ShouldEqual, "")
			})
		})

		Convey("inspectContainer()", func() {
			Convey("looks in the cache first", func() {
				disco.containerCache[svcId1] = &docker.Container{Path: "cached"}
				container, err := disco.inspectContainer(&service1)

				So(err, ShouldBeNil)
				So(container.Path, ShouldEqual, "cached")
			})

			Convey("queries Docker if the service isn't cached", func() {
				container, err := disco.inspectContainer(&service1)

				So(err, ShouldBeNil)
				So(container.Config.Labels["HealthCheck"], ShouldEqual, "HttpGet")
			})

			Convey("bubbles up errors from the Docker client", func() {
				disco.ClientProvider = func() (DockerClient, error) {
					return &stubDockerClient{
						ErrorOnInspectContainer: true,
					}, nil
				}

				container, err := disco.inspectContainer(&service1)
				So(err, ShouldNotBeNil)
				So(container, ShouldBeNil)
			})
		})

		Convey("pruneContainerCache()", func() {
			Convey("prunes the containers we no longer see", func() {
				liveContainers := make(map[string]interface{}, 1)
				liveContainers[svcId1] = true

				// Cache some things
				disco.containerCache[svcId1] = &docker.Container{Path: "cached"}
				disco.containerCache[svcId2] = &docker.Container{Path: "cached"}

				So(len(disco.containerCache), ShouldEqual, 2)

				disco.pruneContainerCache(liveContainers)

				_, ok := disco.containerCache[svcId2] // Should be missing
				So(ok, ShouldBeFalse)
			})
		})
	})
}
