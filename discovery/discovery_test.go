package discovery

import (
	"testing"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/Nitro/sidecar/service"
)

type mockDiscoverer struct {
	ServicesList    []service.Service
	RunInvoked      bool
	ServicesInvoked bool
	Done            chan error
	CheckName       string
}

func (m *mockDiscoverer) Services() []service.Service {
	m.ServicesInvoked = true
	return m.ServicesList
}

func (m *mockDiscoverer) Run(looper director.Looper) {
	m.RunInvoked = true
}

func (m *mockDiscoverer) HealthCheck(svc *service.Service) (string, string) {
	for _, aSvc := range m.ServicesList {
		if svc.Name == aSvc.Name {
			return m.CheckName, ""
		}
	}

	return "", ""
}

func Test_MultiDiscovery(t *testing.T) {
	Convey("MultiDiscovery", t, func() {
		looper := director.NewFreeLooper(director.ONCE, nil)

		done1 := make(chan error, 1)
		done2 := make(chan error, 1)

		svc1 := service.Service{Name: "svc1"}
		svc2 := service.Service{Name: "svc2"}

		disco1 := &mockDiscoverer{ []service.Service{ svc1 }, false, false, done1, "one" }
		disco2 := &mockDiscoverer{ []service.Service{ svc2 }, false, false, done2, "two" }

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

		Convey("HealthCheck() aggregates all the health checks", func() {
			check1, _ := multi.HealthCheck(&svc1)
			check2, _ := multi.HealthCheck(&svc2)

			So(check1, ShouldEqual, "one")
			So(check2, ShouldEqual, "two")
		})

		Convey("HealthCheck() returns empty string when the check is missing", func() {
			svc3 := service.Service{Name: "svc3"}
			check, args := multi.HealthCheck(&svc3)

			So(check, ShouldEqual, "")
			So(args, ShouldEqual, "")
		})
	})
}
