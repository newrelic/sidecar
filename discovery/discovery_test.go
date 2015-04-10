package discovery

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/service"
)

type mockDiscoverer struct {
	ServicesList    []service.Service
	RunInvoked      bool
	ServicesInvoked bool
	Quit            chan bool
}

func (m *mockDiscoverer) Services() []service.Service {
	m.ServicesInvoked = true
	return m.ServicesList
}

func (m *mockDiscoverer) Run(quit chan bool) {
	m.Quit = quit
	m.RunInvoked = true
}

func Test_MultiDiscovery(t *testing.T) {
	Convey("MultiDiscovery", t, func() {
		disco1 := &mockDiscoverer{
			[]service.Service{
				service.Service{Name: "svc1"},
			}, false, false, make(chan bool),
		}
		disco2 := &mockDiscoverer{
			[]service.Service{
				service.Service{Name: "svc2"},
			}, false, false, make(chan bool),
		}

		multi := &MultiDiscovery{[]Discoverer{disco1, disco2}}

		Convey("Run() invokes the Run() method for all the discoverers", func() {
			multi.Run(make(chan bool))

			So(disco1.RunInvoked, ShouldBeTrue)
			So(disco2.RunInvoked, ShouldBeTrue)
		})

		Convey("Run() propagates the quit signal", func() {
			quit := make(chan bool, 1)
			multi.Run(quit)
			quit <- true

			So(disco1.RunInvoked, ShouldBeTrue)
			So(disco2.RunInvoked, ShouldBeTrue)
			So(<-disco1.Quit, ShouldBeTrue)
			So(<-disco2.Quit, ShouldBeTrue)
		})

		Convey("Services() invokes the Services() method for all the discoverers", func() {
			multi.Services()

			So(disco1.ServicesInvoked, ShouldBeTrue)
			So(disco2.ServicesInvoked, ShouldBeTrue)
		})

		Convey("Services() aggregates all the service lists", func() {
			services := multi.Services()

			So(len(services), ShouldEqual, 2)
			So(services[0].Name, ShouldEqual, "svc1")
			So(services[1].Name, ShouldEqual, "svc2")
		})
	})
}
