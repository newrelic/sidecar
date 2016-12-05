package service

import (
	"os"
	"reflect"
	"testing"

	"github.com/fsouza/go-dockerclient"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_PortForServicePort(t *testing.T) {
	Convey("PortForServicePort()", t, func() {
		svc := &Service{
			ID: "deadbeef001",
			Ports: []Port{
				{"tcp", 8173, 8080},
				{"udp", 8172, 8080},
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
			PublicPort:  8723,
			Type:        "tcp",
		}

		container := &docker.APIContainers{
			Ports: []docker.APIPort{port},
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

func Test_ToService(t *testing.T) {
	sampleAPIContainer := &docker.APIContainers{
		ID:      "88862023487fa0ae043c47d7b441f684fc39145d1d9fa398450e4da2e53af5e8",
		Image:   "example.com/docker/fabulous-container:latest",
		Command: "/fabulous_app",
		Created: 1457144774,
		Status:  "Up 34 seconds",
		Ports: []docker.APIPort{
			docker.APIPort{
				PrivatePort: 9990,
				PublicPort:  0,
				Type:        "tcp",
				IP:          "",
			},
			docker.APIPort{
				PrivatePort: 8080,
				PublicPort:  31355,
				Type:        "tcp",
				IP:          "192.168.77.13",
			},
		},
		SizeRw:     0,
		SizeRootFs: 0,
		Names:      []string{"/sample-app-go-worker-eebb5aad1a17ee"},
		Labels: map[string]string{
			"ServicePort_8080": "17010",
			"ServiceProfile":   "special",
			"HealthCheck":      "HttpGet",
			"HealthCheckArgs":  "http://127.0.0.1:39519/status/check",
		},
	}

	samplePorts := []Port{
		Port{
			Type:        "tcp",
			Port:        31355,
			ServicePort: 17010,
		},
	}

	sampleHostname, _ := os.Hostname()

	Convey("ToService()", t, func() {

		Convey("Decodes everything correctly", func() {
			service := ToService(sampleAPIContainer)
			So(service.ID, ShouldEqual, sampleAPIContainer.ID[:12])
			So(service.Image, ShouldEqual, sampleAPIContainer.Image)
			So(service.Name, ShouldEqual, sampleAPIContainer.Names[0])
			So(service.Created.String(), ShouldEqual, "2016-03-05 02:26:14 +0000 UTC")
			So(service.Hostname, ShouldEqual, sampleHostname)
			So(reflect.DeepEqual(samplePorts, service.Ports), ShouldBeTrue)
			So(service.Updated, ShouldNotBeNil)
			So(service.Profile, ShouldEqual, "special")
			So(service.Status, ShouldEqual, 0)
		})

		Convey("Sets default Profile properly", func() {
			delete(sampleAPIContainer.Labels, "ServiceProfile")

			service := ToService(sampleAPIContainer)
			So(service.Profile, ShouldEqual, "default")
		})
	})
}
