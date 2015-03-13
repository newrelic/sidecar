package healthy

import (
	"errors"
	"reflect"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_NewCheck(t *testing.T) {
	Convey("Returns a properly configured Check", t, func() {
		check := NewCheck()

		So(check.Count, ShouldEqual, 0)
		So(check.Type, ShouldEqual, "http")
		So(check.MaxCount, ShouldEqual, 1)
		So(reflect.DeepEqual(check.Command, &HttpCheck{}), ShouldBeTrue)
	})
}

func Test_NewMonitor(t *testing.T) {
	Convey("Returns a properly configured Monitor", t, func() {
		monitor := NewMonitor()

		So(monitor.CheckInterval, ShouldEqual, 3 * time.Second)
		So(len(monitor.Checks), ShouldEqual, 0)
	})
}

func Test_Status(t *testing.T) {
	Convey("Testing Status", t, func() {
		monitor := NewMonitor()
		monitor.Checks = []*Check{
			&Check{Status: HEALTHY},
			&Check{Status: HEALTHY},
			&Check{Status: SICKLY},
			&Check{Status: FAILED},
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
		monitor := NewMonitor()
		So(len(monitor.Checks), ShouldEqual, 0)
		monitor.AddCheck(&Check{})
		So(len(monitor.Checks), ShouldEqual, 1)
		monitor.AddCheck(&Check{})
		So(len(monitor.Checks), ShouldEqual, 2)
	})
}

type mockCommand struct {
	CallCount int
	LastArgs string
	DesiredResult int
	Error error
}

func (m *mockCommand) Run(args string) (int, error) {
	m.CallCount = m.CallCount + 1
	m.LastArgs = args
	return m.DesiredResult, m.Error
}

func Test_RunningChecks(t *testing.T) {
	Convey("Working with health checks", t, func() {
		monitor := NewMonitor()
		monitor.CheckInterval = 1 * time.Nanosecond
		cmd := mockCommand{DesiredResult: HEALTHY}
		check := &Check{
			Type: "mock",
			Args: "testing",
			Command: &cmd,
		}
		monitor.AddCheck(check)

		Convey("The Check Command gets evaluated", func() {
			monitor.Run(1)
			So(cmd.CallCount, ShouldEqual, 1)
			So(cmd.LastArgs, ShouldEqual, "testing")
			So(cmd.DesiredResult, ShouldEqual, HEALTHY) // We know it's our cmd
		})

		Convey("Healthy Checks are marked healthy", func() {
			monitor.Run(1)
			So(cmd.CallCount, ShouldEqual, 1)
			So(cmd.LastArgs, ShouldEqual, "testing")
			So(check.Status, ShouldEqual, HEALTHY)
		})

		Convey("Unhealthy Checks are marked unhealthy", func() {
			fail := mockCommand{DesiredResult: SICKLY}
			badCheck := &Check{
				Type: "mock",
				Args: "testing123",
				Command: &fail,
				MaxCount: 1,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(1)

			So(fail.CallCount, ShouldEqual, 1)
			So(badCheck.Status, ShouldEqual, SICKLY)
		})

		Convey("Erroring checks are marked UNKNOWN", func() {
			fail := mockCommand{Error: errors.New("Uh oh!"), DesiredResult: FAILED}
			badCheck := &Check{
				Type: "mock",
				Args: "testing123",
				Command: &fail,
				MaxCount: 1,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(1)

			So(fail.CallCount, ShouldEqual, 1)
			So(badCheck.Status, ShouldEqual, UNKNOWN)
		})
	})
}
