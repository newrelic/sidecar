package healthy

import (
	"regexp"
	"testing"
	"time"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
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

		monitor := NewMonitor(hostname)
		state := catalog.NewServicesState()
		state.Hostname = hostname
		state.ServiceNameMatch = regexp.MustCompile("^(.+)(-[0-9a-z]{7,14})$")

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

		state.AddServiceEntry(service1)
		state.AddServiceEntry(service2)
		state.AddServiceEntry(service3)
		state.AddServiceEntry(service4)

		Convey("Returns services that are healthy", func() {
			svcList := monitor.Services(state)

			var found bool

			for _, svc := range svcList {
				if svc.ID == svcId1 {
					found = true
					break
				}
			}

			So(found, ShouldBeTrue)
		})

		Convey("Does not return services that have failed or are unknown", func() {
			svcList := monitor.Services(state)

			So(len(svcList), ShouldEqual, 2) // healthy + sickly
		})

		Convey("Returns services that are sickly", func() {
			svcList := monitor.Services(state)

			var found bool

			for _, svc := range svcList {
				if svc.ID == svcId3 {
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
			check := &Check{ID: svc.ID, Command: &cmd, Type: "HttpGet", Args: "http://" + hostname + ":1234/status/check"}
			looper := director.NewTimedLooper(5, 5*time.Nanosecond, nil)

			monitor.Watch(listFn, looper)

			So(len(monitor.Checks), ShouldEqual, 1)
			So(monitor.Checks[svc.ID], ShouldResemble, check)
		})
	})
}

func Test_NewDefaultCheck(t *testing.T) {
	Convey("When building a default check", t, func() {
		svcId1 := "deadbeef123"
		baseTime := time.Now().UTC().Round(time.Second)
		ports := []service.Port{service.Port{"udp", 11234}, service.Port{"tcp", 1234}}
		service1 := service.Service{ID: svcId1, Hostname: hostname, Updated: baseTime, Ports: ports}

		Convey("Find the first tcp port", func() {
			port := findFirstTCPPort(&service1)
			So(port, ShouldNotBeNil)
			So(port.Port, ShouldEqual, 1234)
			So(port.Type, ShouldEqual, "tcp")
		})

		Convey("Returns proper check", func() {
			monitor := NewMonitor(hostname)
			check := monitor.CheckForService(&service1)
			So(check.ID, ShouldEqual, service1.ID)
		})
	})
}
