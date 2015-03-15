package healthy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/services_state"
)

var hostname string = "indefatigable"

func Test_ServicesBridge(t *testing.T) {
	Convey("When working with the services bridge", t, func() {
		svcId1   := "deadbeef123"
		svcId2   := "deadbeef101"
		baseTime := time.Now().UTC().Round(time.Second)

		service1 := service.Service{ ID: svcId1, Hostname: hostname, Updated: baseTime }
		service2 := service.Service{ ID: svcId2, Hostname: hostname, Updated: baseTime }

		monitor := NewMonitor()
		state   := services_state.NewServicesState()
		state.HostnameFn = func() (string, error) { return hostname, nil }

		check1 := Check{ ID: svcId1, Status: HEALTHY }
		check2 := Check{ ID: svcId2, Status: UNKNOWN }
		monitor.AddCheck(&check1)
		monitor.AddCheck(&check2)

		state.AddServiceEntry(service1)
		state.AddServiceEntry(service2)

		Convey("Services() returns the services for each healthy check", func() {
			svcList := monitor.Services(state)

			So(len(svcList), ShouldEqual, 1)
			So(svcList[0].ID, ShouldEqual, svcId1)
		})
	})
}
