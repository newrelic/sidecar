package services_state

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/service"
)

var (
	hostname1 string = "shakespeare"
	hostname2 string = "chaucer"
	hostname3 string = "bocaccio"
)

func Test_ServerSorting(t *testing.T) {

	Convey("Sorting", t, func() {
		state := NewServicesState()
		svcId1     := "deadbeef123"
		svcId2     := "deadbeef101"
		svcId3     := "deadbeef105"
		baseTime   := time.Now().UTC().Round(time.Second)

		service1 := service.Service{ ID: svcId1, Hostname: hostname1, Updated: baseTime.Add(5 * time.Second) }
		service2 := service.Service{ ID: svcId2, Hostname: hostname2, Updated: baseTime }
		service3 := service.Service{ ID: svcId3, Hostname: hostname3, Updated: baseTime.Add(10 * time.Second) }

		state.HostnameFn = func() (string, error) { return hostname, nil }

		state.AddServiceEntry(service1)
		state.AddServiceEntry(service2)
		state.AddServiceEntry(service3)

		Convey("Returns a list of Servers sorted by Name", func() {
			sortedServers := state.SortedServers()
			names := make([]string, 0, len(sortedServers))

			for _, server := range(sortedServers) {
				names = append(names, server.Name)
			}

			So(names[0], ShouldEqual, "bocaccio")
			So(names[1], ShouldEqual, "chaucer")
			So(names[2], ShouldEqual, "shakespeare")
		})

		Convey("Returns a list of Services sorted by Updates", func() {
			service1 := service.Service{ ID: svcId1, Hostname: hostname3, Updated: baseTime.Add(5 * time.Second) }
			service2 := service.Service{ ID: svcId2, Hostname: hostname3, Updated: baseTime }
			service3 := service.Service{ ID: svcId3, Hostname: hostname3, Updated: baseTime.Add(10 * time.Second) }

			state.AddServiceEntry(service3)
			state.AddServiceEntry(service2)
			state.AddServiceEntry(service1)

			sortedServices := state.Servers[hostname3].SortedServices()
			ids := make([]string, 0, len(sortedServices))

			for _, service := range(sortedServices) {
				ids = append(ids, service.ID)
			}

			So(ids[0], ShouldEqual, svcId2)
			So(ids[1], ShouldEqual, svcId1)
			So(ids[2], ShouldEqual, svcId3)
		})

	})

}
