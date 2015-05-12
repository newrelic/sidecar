package healthy

import (
	"errors"
	"testing"
	"time"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
)

func Test_NewCheck(t *testing.T) {
	Convey("Returns a properly configured Check", t, func() {
		check := NewCheck("testing")

		So(check.Count, ShouldEqual, 0)
		So(check.Type, ShouldEqual, "http")
		So(check.MaxCount, ShouldEqual, 1)
		So(check.ID, ShouldEqual, "testing")
		So(check.Command, ShouldResemble, &HttpGetCmd{})
	})
}

func Test_NewMonitor(t *testing.T) {
	Convey("Returns a properly configured Monitor", t, func() {
		monitor := NewMonitor(hostname)

		So(monitor.CheckInterval, ShouldEqual, HEALTH_INTERVAL)
		So(len(monitor.Checks), ShouldEqual, 0)
	})
}

func Test_Status(t *testing.T) {
	Convey("Testing Status", t, func() {
		monitor := NewMonitor(hostname)
		monitor.Checks = map[string]*Check{
			"12345a": &Check{Status: HEALTHY},
			"23456b": &Check{Status: HEALTHY},
			"34567c": &Check{Status: SICKLY},
			"45678d": &Check{Status: FAILED},
		}

		Convey("Healthy() returns a list of only healthy checks", func() {
			list := monitor.Healthy()
			So(len(list), ShouldEqual, 2)
		})

		Convey("Unhealthy() returns a list of anything but healthy checks", func() {
			list := monitor.Unhealthy()
			So(len(list), ShouldEqual, 2)
		})
	})
}

func Test_AddCheck(t *testing.T) {
	Convey("Adds a check to the list", t, func() {
		monitor := NewMonitor(hostname)
		So(len(monitor.Checks), ShouldEqual, 0)
		monitor.AddCheck(&Check{ID: "123"})
		So(len(monitor.Checks), ShouldEqual, 1)
		monitor.AddCheck(&Check{ID: "234"})
		So(len(monitor.Checks), ShouldEqual, 2)
	})
}

func Test_RemoveCheck(t *testing.T) {
	Convey("Removes a check from the list", t, func() {
		monitor := NewMonitor(hostname)
		monitor.AddCheck(&Check{ID: "123"})
		So(len(monitor.Checks), ShouldEqual, 1)
		monitor.RemoveCheck("123")
		So(len(monitor.Checks), ShouldEqual, 0)
	})
}

type mockCommand struct {
	CallCount     int
	LastArgs      string
	DesiredResult int
	Error         error
}

func (m *mockCommand) Run(args string) (int, error) {
	m.CallCount = m.CallCount + 1
	m.LastArgs = args
	return m.DesiredResult, m.Error
}

type slowCommand struct{}

func (s *slowCommand) Run(args string) (int, error) {
	time.Sleep(10 * time.Millisecond)
	return HEALTHY, nil
}

func Test_RunningChecks(t *testing.T) {
	Convey("Working with health checks", t, func() {
		monitor := NewMonitor(hostname)
		cmd := mockCommand{DesiredResult: HEALTHY}
		check := &Check{
			Type:    "mock",
			Args:    "testing",
			Command: &cmd,
		}
		monitor.AddCheck(check)
		state := catalog.NewServicesState()

		looper := director.NewFreeLooper(director.ONCE, nil)

		Convey("The Check Command gets evaluated", func() {
			monitor.Run(state, looper)
			So(cmd.CallCount, ShouldEqual, 1)
			So(cmd.LastArgs, ShouldEqual, "testing")
			So(cmd.DesiredResult, ShouldEqual, HEALTHY) // We know it's our cmd
		})

		Convey("Healthy Checks are marked healthy", func() {
			monitor.Run(state, looper)
			So(cmd.CallCount, ShouldEqual, 1)
			So(cmd.LastArgs, ShouldEqual, "testing")
			So(check.Status, ShouldEqual, HEALTHY)
		})

		Convey("Unhealthy Checks are marked unhealthy", func() {
			fail := mockCommand{DesiredResult: SICKLY}
			badCheck := &Check{
				Type:     "mock",
				Args:     "testing123",
				Command:  &fail,
				MaxCount: 3,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(state, looper)

			So(fail.CallCount, ShouldEqual, 1)
			So(badCheck.Status, ShouldEqual, SICKLY)
		})

		Convey("Erroring checks are marked UNKNOWN", func() {
			fail := mockCommand{Error: errors.New("Uh oh!"), DesiredResult: FAILED}
			badCheck := &Check{
				Type:     "mock",
				Args:     "testing123",
				Command:  &fail,
				MaxCount: 3,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(state, looper)

			So(fail.CallCount, ShouldEqual, 1)
			So(badCheck.Status, ShouldEqual, UNKNOWN)
		})

		Convey("Checks that fail too many times are marked FAILED", func() {
			fail := mockCommand{DesiredResult: SICKLY}
			maxCount := 2
			badCheck := &Check{
				Type:     "mock",
				Args:     "testing123",
				Command:  &fail,
				MaxCount: maxCount,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(state, director.NewFreeLooper(maxCount, nil))
			So(fail.CallCount, ShouldEqual, maxCount)
			So(badCheck.Count, ShouldEqual, maxCount)
			So(badCheck.Status, ShouldEqual, FAILED)
		})

		Convey("Checks that were failed return to health", func() {
			healthy := mockCommand{DesiredResult: HEALTHY}
			badCheck := &Check{
				Type:    "mock",
				Status:  FAILED,
				Args:    "testing123",
				Command: &healthy,
				Count:   2,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(state, looper)
			So(badCheck.Count, ShouldEqual, 0)
			So(badCheck.Status, ShouldEqual, HEALTHY)

		})

		Convey("Checks that take too long time out", func() {
			check := &Check{
				ID:       "test",
				Type:     "mock",
				Status:   FAILED,
				Args:     "testing123",
				Command:  &slowCommand{},
				MaxCount: 3,
			}
			monitor.AddCheck(check)
			monitor.CheckInterval = 1 * time.Millisecond
			monitor.Run(state, looper)

			So(check.Status, ShouldEqual, UNKNOWN)
			So(check.LastError.Error(), ShouldEqual, "Timed out!")
		})

		Convey("Checks that had an error become UNKNOWN on first pass", func() {
			check := NewCheck("test")
			check.Command = &slowCommand{}
			check.MaxCount = 3
			check.UpdateStatus(1, errors.New("Borked!"))

			So(check.Status, ShouldEqual, UNKNOWN)
		})
	})
}

func Test_MarkingServices(t *testing.T) {

	Convey("When marking services", t, func() {
		// Set up a bunch of services in various states and some checks.
		// Then we health check them and look at the results carefully.
		monitor := NewMonitor(hostname)
		services := []*service.Service{
			&service.Service{ID: "test", Status: service.ALIVE},
			&service.Service{ID: "bad", Status: service.ALIVE},
			&service.Service{ID: "unknown", Status: service.ALIVE},
			&service.Service{ID: "test2", Status: service.TOMBSTONE},
			&service.Service{ID: "unknown2", Status: service.UNKNOWN},
		}

		state := catalog.NewServicesState()
		state.Hostname = hostname
		for _, svc := range services {
			svc.Hostname = hostname
			state.AddServiceEntry(*svc)
		}

		looper := director.NewFreeLooper(director.ONCE, nil)

		monitor.AddCheck(
			&Check{
				ID:      "test",
				Type:    "mock",
				Status:  HEALTHY,
				Args:    "testing123",
				Command: &mockCommand{DesiredResult: HEALTHY},
			},
		)
		monitor.AddCheck(
			&Check{
				ID:      "bad",
				Type:    "mock",
				Status:  HEALTHY,
				Args:    "testing123",
				Command: &mockCommand{DesiredResult: SICKLY},
			},
		)
		monitor.AddCheck(
			&Check{
				ID:      "test2",
				Type:    "mock",
				Status:  HEALTHY,
				Args:    "foofoofoo",
				Command: &mockCommand{DesiredResult: HEALTHY},
			},
		)
		monitor.AddCheck(
			&Check{
				ID:      "unknown2",
				Type:    "mock",
				Status:  HEALTHY,
				Args:    "foofoofoo",
				Command: &mockCommand{DesiredResult: HEALTHY},
			},
		)

		monitor.Run(state, looper)

		Convey("When healthy, marks the service as ALIVE", func() {
			So(state.GetLocalService(services[0].ID).Status,
				ShouldEqual, service.ALIVE)
		})

		Convey("When not healthy, marks the service as UNHEALTHY", func() {
			So(state.GetLocalService(services[1].ID).Status,
				ShouldEqual, service.UNHEALTHY)
		})

		Convey("When there is no check, marks the service as UNKNOWN", func() {
			So(state.GetLocalService(services[2].ID).Status,
				ShouldEqual, service.UNKNOWN)
		})

		Convey("Removes a check when encountering a Tombstone", func() {
			So(state.GetLocalService(services[3].ID).Status,
				ShouldEqual, service.TOMBSTONE)
		})

		Convey("Transitions services to healthy when they are", func() {
			So(state.GetLocalService(services[4].ID).Status,
				ShouldEqual, service.ALIVE)
		})
	})
}
