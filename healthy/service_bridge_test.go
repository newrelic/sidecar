package healthy

import (
	"testing"
	"time"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/sidecar/service"
)

var hostname string = "indefatigable"

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

		monitor := NewMonitor(hostname, "")
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

			ports := []service.Port{service.Port{"udp", 11234}, service.Port{"tcp", 1234}}
			svc := service.Service{ID: "babbacabba", Name: "testing-12312312", Ports: ports}
			svcList := []service.Service{svc}
			listFn := func() []service.Service { return svcList }

			cmd := HttpGetCmd{}
			check := &Check{
				ID:      svc.ID,
				Command: &cmd,
				Type:    "HttpGet",
				Args:    "http://" + hostname + ":1234/status/check",
				Status:  FAILED,
			}
			looper := director.NewTimedLooper(5, 5*time.Nanosecond, nil)

			monitor.Watch(listFn, looper)

			So(len(monitor.Checks), ShouldEqual, 1)
			So(monitor.Checks[svc.ID], ShouldResemble, check)
		})
	})
}

func Test_CheckForService(t *testing.T) {
	Convey("When building a default check", t, func() {
		svcId1 := "deadbeef123"
		ports := []service.Port{service.Port{"udp", 11234}, service.Port{"tcp", 1234}}
		service1 := service.Service{ID: svcId1, Hostname: hostname, Ports: ports}

		Convey("Find the first tcp port", func() {
			port := findFirstTCPPort(&service1)
			So(port, ShouldNotBeNil)
			So(port.Port, ShouldEqual, 1234)
			So(port.Type, ShouldEqual, "tcp")
		})

		Convey("Returns proper check", func() {
			monitor := NewMonitor(hostname, "")
			check := monitor.CheckForService(&service1)
			So(check.ID, ShouldEqual, service1.ID)
		})
	})
}

func Test_GetCommandNamed(t *testing.T) {
	Convey("Returns the correct command", t, func() {
		monitor := NewMonitor("localhost", "")

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

func Test_urlForService(t *testing.T) {
	Convey("Generating the check URL for a service", t, func() {
		monitor := NewMonitor("localhost", hostname)
		svcId1 := "deadbeef123"
		service1 := service.Service{ID: svcId1, Hostname: hostname}
		monitor.ServiceNameFn = func(s *service.Service) string { return "chaucer" }
		monitor.TomeAddr = "localhost:7776"

		Convey("Without a ServiceNameFn, returns and empty URL", func() {
			monitor.ServiceNameFn = nil
			So(monitor.urlForService(&service1), ShouldEqual, "")
		})

		Convey("Without a TomeAddr, returns an empty URL", func() {
			monitor.TomeAddr = ""
			So(monitor.urlForService(&service1), ShouldEqual, "")
		})

		Convey("Without any substitution returns a correct check", func() {
			So(monitor.urlForService(&service1), ShouldEqual, "http://localhost:7776/checks/chaucer")
		})

		Convey("Without any substitution returns a replaced check URL", func() {
			So(monitor.urlForService(&service1), ShouldEqual, "http://localhost:7776/checks/chaucer")
		})

		Convey("When the service name is blank, returns blank URL", func() {
			monitor.ServiceNameFn = func(s *service.Service) string { return "" }
			So(monitor.urlForService(&service1), ShouldEqual, "")
		})
	})
}
