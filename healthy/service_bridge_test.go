package healthy

import (
	"regexp"
	"testing"
	"time"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/catalog"
)

var hostname string = "indefatigable"

func Test_ServicesBridge(t *testing.T) {
	Convey("The services bridge", t, func() {
		svcId1   := "deadbeef123"
		svcId2   := "deadbeef101"
		baseTime := time.Now().UTC().Round(time.Second)

		service1 := service.Service{ ID: svcId1, Hostname: hostname, Updated: baseTime }
		service2 := service.Service{ ID: svcId2, Hostname: hostname, Updated: baseTime }

		monitor := NewMonitor()
		state   := catalog.NewServicesState()
		state.HostnameFn = func() (string, error) { return hostname, nil }
		state.ServiceNameMatch  = regexp.MustCompile("^(.+)(-[0-9a-z]{7,14})$")

		check1 := Check{
			ID: svcId1,
			Status: HEALTHY,
		}
		check2 := Check{
			ID: svcId2,
			Status: UNKNOWN,
		}
		monitor.AddCheck(&check1)
		monitor.AddCheck(&check2)

		state.AddServiceEntry(service1)
		state.AddServiceEntry(service2)

		Convey("Returns the services for each healthy check", func() {
			svcList := monitor.Services(state)

			So(len(svcList), ShouldEqual, 1)
			So(svcList[0].ID, ShouldEqual, svcId1)
		})

		Convey("Responds to changes in a list of services", func() {
			So(len(monitor.Checks), ShouldEqual, 2)
			svcList := []service.Service{}
			listFn  := func() []service.Service { return svcList }
			svc     := service.Service{ ID: "babbacabba", Name: "testing-12312312" }

			cmd := mockCommand{DesiredResult: HEALTHY}
			check := &Check{ID: svc.ID, Command: &cmd}
			monitor.ServiceChecks[state.ServiceName(&svc)] = check

			waitChan := make(chan error)
			looper := director.NewTimedLooper(5, 5 * time.Nanosecond, waitChan)
			go monitor.Watch(listFn, state.ServiceName, looper)

			svcList = append(svcList, svc)
			<-waitChan

			So(len(monitor.Checks), ShouldEqual, 1)
			So(monitor.Checks[svc.ID], ShouldResemble, check)
		})
	})
}
