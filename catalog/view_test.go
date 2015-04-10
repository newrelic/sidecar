package catalog

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
		svcId1 := "deadbeef123"
		svcId2 := "deadbeef101"
		svcId3 := "deadbeef105"
		baseTime := time.Now().UTC().Round(time.Second)

		service1 := service.Service{ID: svcId1, Hostname: hostname1, Updated: baseTime.Add(5 * time.Second)}
		service2 := service.Service{ID: svcId2, Hostname: hostname2, Updated: baseTime}
		service3 := service.Service{ID: svcId3, Hostname: hostname3, Updated: baseTime.Add(10 * time.Second)}

		state.HostnameFn = func() (string, error) { return hostname, nil }

		state.AddServiceEntry(service1)
		state.AddServiceEntry(service2)
		state.AddServiceEntry(service3)

		Convey("Returns a list of Servers sorted by Name", func() {
			sortedServers := state.SortedServers()
			names := make([]string, 0, len(sortedServers))

			for _, server := range sortedServers {
				names = append(names, server.Name)
			}

			should := []string{"bocaccio", "chaucer", "shakespeare"}
			for i, id := range should {
				So(names[i], ShouldEqual, id)
			}
		})

		Convey("Returns a list of Services sorted by Updates", func() {
			service1 := service.Service{ID: svcId1, Hostname: hostname3, Updated: baseTime.Add(5 * time.Second)}
			service2 := service.Service{ID: svcId2, Hostname: hostname3, Updated: baseTime}
			service3 := service.Service{ID: svcId3, Hostname: hostname3, Updated: baseTime.Add(10 * time.Second)}

			state.AddServiceEntry(service3)
			state.AddServiceEntry(service2)
			state.AddServiceEntry(service1)

			sortedServices := state.Servers[hostname3].SortedServices()
			ids := make([]string, 0, len(sortedServices))

			for _, service := range sortedServices {
				ids = append(ids, service.ID)
			}

			So(ids[0], ShouldEqual, svcId2)
			So(ids[1], ShouldEqual, svcId1)
			So(ids[2], ShouldEqual, svcId3)
		})

		Convey("Returs a list of Services sorted on sorted Servers", func() {
			service4 := service.Service{ID: svcId1, Hostname: hostname3, Updated: baseTime.Add(5 * time.Second)}
			service5 := service.Service{ID: svcId2, Hostname: hostname3, Updated: baseTime}
			service6 := service.Service{ID: svcId3, Hostname: hostname3, Updated: baseTime.Add(10 * time.Second)}

			state.AddServiceEntry(service4)
			state.AddServiceEntry(service5)
			state.AddServiceEntry(service6)

			services := make([]string, 0, 10)

			state.EachServiceSorted(func(hostname *string, serviceId *string, svc *service.Service) {
				services = append(services, svc.ID)
			})

			should := []string{"deadbeef101", "deadbeef101", "deadbeef123", "deadbeef123", "deadbeef105"}
			for i, id := range should {
				So(services[i], ShouldEqual, id)
			}
		})

	})

}
