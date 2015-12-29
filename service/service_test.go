package service

import (
	"testing"

	"github.com/fsouza/go-dockerclient"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_PortForServicePort(t *testing.T) {
	Convey("PortForServicePort()", t, func() {
		svc := &Service{
			ID:   "deadbeef001",
			Ports: []Port{
				{ "tcp", 8173, 8080 },
				{ "udp", 8172, 8080 },
			},
		}

		Convey("Returns the port when it matches", func() {
			So(svc.PortForServicePort(8080, "tcp"), ShouldEqual, 8173)
		})

		Convey("Returns -1 when there is no match", func() {
			So(svc.PortForServicePort(8090, "tcp"), ShouldEqual, -1)
		})
	})
}

func Test_buildPortFor(t *testing.T) {
	Convey("buildPortFor()", t, func() {
		port := docker.APIPort{
			PrivatePort: 80,
			PublicPort: 8723,
			Type: "tcp",
		}

		container := &docker.APIContainers{
			Ports: []docker.APIPort{ port },
			Labels: map[string]string{
				"ServicePort_80": "8080",
			},
		}

		Convey("Maps service ports to internal ports", func() {
			port := buildPortFor(&port, container)

			So(port.ServicePort, ShouldEqual, 8080)
			So(port.Port, ShouldEqual, 8723)
			So(port.Type, ShouldEqual, "tcp")
		})

		Convey("Skips the service port when there is none", func() {
			delete(container.Labels, "ServicePort_80")
			port := buildPortFor(&port, container)

			So(port.ServicePort, ShouldEqual, 0)
			So(port.Port, ShouldEqual, 8723)
			So(port.Type, ShouldEqual, "tcp")
		})

		Convey("Skips the service port when there is a conversion error", func() {
			container.Labels["ServicePort_80"] = "not a number"
			port := buildPortFor(&port, container)

			So(port.ServicePort, ShouldEqual, 0)
			So(port.Port, ShouldEqual, 8723)
			So(port.Type, ShouldEqual, "tcp")
		})
	})
}
