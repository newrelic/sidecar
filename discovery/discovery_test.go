package discovery

import (
	"testing"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/sidecar/service"
)

type mockDiscoverer struct {
	ServicesList    []service.Service
	RunInvoked      bool
	ServicesInvoked bool
	Done            chan error
}

func (m *mockDiscoverer) Services() []service.Service {
	m.ServicesInvoked = true
	return m.ServicesList
}

func (m *mockDiscoverer) Run(looper director.Looper) {
	m.RunInvoked = true
}

func (m *mockDiscoverer) HealthCheck(*service.Service) (string, string) {
	return "", ""
}

func Test_MultiDiscovery(t *testing.T) {
	Convey("MultiDiscovery", t, func() {
		looper := director.NewFreeLooper(director.ONCE, nil)

		done1 := make(chan error, 1)
		done2 := make(chan error, 1)

		disco1 := &mockDiscoverer{
			[]service.Service{
				service.Service{Name: "svc1"},
			}, false, false, done1,
		}
		disco2 := &mockDiscoverer{
			[]service.Service{
				service.Service{Name: "svc2"},
			}, false, false, done2,
		}

		multi := &MultiDiscovery{[]Discoverer{disco1, disco2}}

		Convey("Run() invokes the Run() method for all the discoverers", func() {
			multi.Run(looper)

			So(disco1.RunInvoked, ShouldBeTrue)
			So(disco2.RunInvoked, ShouldBeTrue)
		})

		SkipConvey("Run() propagates the quit signal", func() {
			multi.Run(looper)

			So(disco1.RunInvoked, ShouldBeTrue)
			So(disco2.RunInvoked, ShouldBeTrue)
			So(<-done1, ShouldBeNil)
			So(<-done2, ShouldBeNil)
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
