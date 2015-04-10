package discovery

import (
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/service"
)

var hostname = "shakespeare"

func Test_DockerDiscovery(t *testing.T) {

	Convey("Working with Docker containers", t, func() {
		endpoint := "http://example.com:2375"
		disco := NewDockerDiscovery(endpoint)
		svcId1 := "deadbeef1231"
		svcId2 := "deadbeef1011"
		baseTime := time.Now().UTC().Round(time.Second)
		service1 := service.Service{ID: svcId1, Hostname: hostname, Updated: baseTime}
		service2 := service.Service{ID: svcId2, Hostname: hostname, Updated: baseTime}
		services := []*service.Service{&service1, &service2}

		Convey("New() configures an endpoint and events channel", func() {
			So(disco.endpoint, ShouldEqual, endpoint)
			So(disco.events, ShouldNotBeNil)
		})

		Convey("Services() returns the right list of services", func() {
			disco.containers = services

			processed := disco.Services()
			So(processed[0].Format(), ShouldEqual, service1.Format())
			So(processed[1].Format(), ShouldEqual, service2.Format())
		})

		Convey("handleEvents() prunes dead containers", func() {
			disco.containers = services
			disco.handleEvent(docker.APIEvents{ID: svcId1, Status: "die"})

			result := disco.Services()
			So(len(result), ShouldEqual, 1)
			So(result[0].Format(), ShouldEqual, service2.Format())
		})
	})
}
