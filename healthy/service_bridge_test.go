package healthy

import (
	"testing"
	"time"

	"github.com/Nitro/sidecar/service"
	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/Nitro/sidecar/discovery"
)

var hostname string = "indefatigable"

type mockDiscoverer struct {
	listFn func() []service.Service
}

func (m *mockDiscoverer) Services() []service.Service {
	return m.listFn()
}

func (m *mockDiscoverer) Listeners() []discovery.ChangeListener {
	return nil
}

func (m *mockDiscoverer) HealthCheck(svc *service.Service) (string, string) {
	if svc.Name == "hasCheck" {
		return "HttpGet", "http://{{ host }}:{{ tcp 8081 }}/status/check"
	}

	if svc.Name == "containerCheck" {
		return "HttpGet", "http://{{ container }}:{{ tcp 8081 }}/status/check"
	}

	return "", ""
}

func (m *mockDiscoverer) Run(director.Looper) {}

func Test_ServicesBridge(t *testing.T) {
	Convey("The services bridge", t, func() {
		svcId1 := "deadbeef123"
		svcId2 := "deadbeef101"
		svcId3 := "deadbeef102"
		svcId4 := "deadbeef103"
		baseTime := time.Now().UTC().Round(time.Second)

		service1 := service.Service{ID: svcId1, Hostname: hostname, Updated: baseTime}
		service2 := service.Service{ID: svcId2, Hostname: hostname, Updated: baseTime}
		service3 := service.Service{ID: svcId3, Hostname: hostname, Updated: baseTime}
		service4 := service.Service{ID: svcId4, Hostname: hostname, Updated: baseTime}
		empty := service.Service{}

		services := []service.Service{service1, service2, service3, service4, empty}

		monitor := NewMonitor(hostname, "/")
		monitor.DiscoveryFn = func() []service.Service { return services }

		check1 := Check{
			ID:     svcId1,
			Status: HEALTHY,
		}
		check2 := Check{
			ID:     svcId2,
			Status: UNKNOWN,
		}
		check3 := Check{
			ID:     svcId3,
			Status: SICKLY,
		}
		check4 := Check{
			ID:     svcId4,
			Status: FAILED,
		}
		monitor.AddCheck(&check1)
		monitor.AddCheck(&check2)
		monitor.AddCheck(&check3)
		monitor.AddCheck(&check4)

		Convey("Returns all the services, marked appropriately", func() {
			svcList := monitor.Services()
			So(len(svcList), ShouldEqual, 4)
		})

		Convey("Returns an empty list when DiscoveryFn is not defined", func() {
			monitor.DiscoveryFn = nil
			svcList := monitor.Services()
			So(len(svcList), ShouldEqual, 0)
		})

		Convey("Returns services that are healthy", func() {
			svcList := monitor.Services()

			var found bool

			for _, svc := range svcList {
				if svc.ID == svcId1 {
					found = true
					break
				}
			}

			So(found, ShouldBeTrue)
		})

		Convey("Returns services that are sickly", func() {
			svcList := monitor.Services()

			var found bool

			for _, svc := range svcList {
				if svc.ID == svcId3 {
					found = true
					break
				}
			}

			So(found, ShouldBeTrue)
		})

		Convey("Returns services that are unknown", func() {
			svcList := monitor.Services()

			var found bool

			for _, svc := range svcList {
				if svc.ID == svcId2 {
					found = true
					break
				}
			}

			So(found, ShouldBeTrue)
		})

		Convey("Responds to changes in a list of services", func() {
			So(len(monitor.Checks), ShouldEqual, 4)

			ports := []service.Port{{"udp", 11234, 8080, "127.0.0.1"}, {"tcp", 1234, 8081, "127.0.0.1"}}
			svc := service.Service{ID: "babbacabba", Name: "testing-12312312", Ports: ports}
			svcList := []service.Service{svc}

			disco := &mockDiscoverer{listFn: func() []service.Service { return svcList }}

			cmd := HttpGetCmd{}
			check := &Check{
				ID:      svc.ID,
				Command: &cmd,
				Type:    "HttpGet",
				Args:    "http://" + hostname + ":1234/",
				Status:  FAILED,
			}
			looper := director.NewTimedLooper(5, 5*time.Nanosecond, nil)

			monitor.Watch(disco, looper)

			So(len(monitor.Checks), ShouldEqual, 1)
			So(monitor.Checks[svc.ID], ShouldResemble, check)
		})
	})
}

func Test_CheckForService(t *testing.T) {
	Convey("When building a default check", t, func() {
		svcId1 := "deadbeef123"
		ports := []service.Port{
			{"udp", 11234, 8080, "127.0.0.1"},
			{"tcp", 1234, 8081, "127.0.0.1"},
		}
		service1 := service.Service{ID: svcId1, Hostname: hostname, Ports: ports}

		Convey("Find the first tcp port", func() {
			port := findFirstTCPPort(&service1)
			So(port, ShouldNotBeNil)
			So(port.Port, ShouldEqual, 1234)
			So(port.Type, ShouldEqual, "tcp")
		})

		Convey("Returns proper check", func() {
			monitor := NewMonitor(hostname, "/")
			check := monitor.CheckForService(&service1, &mockDiscoverer{})
			So(check.ID, ShouldEqual, service1.ID)
		})

		Convey("Templates in the check arguments", func() {
			monitor := NewMonitor(hostname, "/")
			service1.Name = "hasCheck"
			check := monitor.CheckForService(&service1, &mockDiscoverer{})
			So(check.Args, ShouldEqual, "http://indefatigable:1234/status/check")
		})

		Convey("Supports container hostname", func() {
			monitor := NewMonitor(hostname, "/")
			service1.Name = "containerCheck"
			check := monitor.CheckForService(&service1, &mockDiscoverer{})
			So(check.Args, ShouldEqual, "http://indefatigable:1234/status/check")
		})

		Convey("Uses the right default endpoint when it's configured", func() {
			monitor := NewMonitor(hostname, "/something/else")
			check := monitor.CheckForService(&service1, &mockDiscoverer{})
			So(check.Args, ShouldEqual, "http://indefatigable:1234/something/else")
		})
	})
}

func Test_GetCommandNamed(t *testing.T) {
	Convey("Returns the correct command", t, func() {
		monitor := NewMonitor("localhost", "/")

		Convey("When asked for an HttpGet", func() {
			So(monitor.GetCommandNamed("HttpGet"), ShouldResemble,
				&HttpGetCmd{},
			)
		})

		Convey("When asked for an ExternalCmd", func() {
			So(monitor.GetCommandNamed("External"), ShouldResemble,
				&ExternalCmd{},
			)
		})

		Convey("When asked for an invalid type", func() {
			So(monitor.GetCommandNamed("Awesome-sauce"), ShouldResemble,
				&HttpGetCmd{},
			)
		})
	})
}
