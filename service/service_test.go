package service

import (
	"testing"

	"github.com/fsouza/go-dockerclient"
	. "github.com/smartystreets/goconvey/convey"
)

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
