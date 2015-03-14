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
		monitor := NewMonitor()
		So(len(monitor.Checks), ShouldEqual, 0)
		monitor.AddCheck(&Check{ID: "123"})
		So(len(monitor.Checks), ShouldEqual, 1)
		monitor.AddCheck(&Check{ID: "234"})
		So(len(monitor.Checks), ShouldEqual, 2)
	})
}

func Test_RemoveCheck(t *testing.T) {
	Convey("Removes a check from the list", t, func() {
		monitor := NewMonitor()
		monitor.AddCheck(&Check{ID: "123"})
		So(len(monitor.Checks), ShouldEqual, 1)
		monitor.RemoveCheck("123")
		So(len(monitor.Checks), ShouldEqual, 0)
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

		SkipConvey("The Check Command gets evaluated", func() {
			monitor.Run(1)
			So(cmd.CallCount, ShouldEqual, 1)
			So(cmd.LastArgs, ShouldEqual, "testing")
			So(cmd.DesiredResult, ShouldEqual, HEALTHY) // We know it's our cmd
		})

		SkipConvey("Healthy Checks are marked healthy", func() {
			monitor.Run(1)
			So(cmd.CallCount, ShouldEqual, 1)
			So(cmd.LastArgs, ShouldEqual, "testing")
			So(check.Status, ShouldEqual, HEALTHY)
		})

		SkipConvey("Unhealthy Checks are marked unhealthy", func() {
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

		SkipConvey("Erroring checks are marked UNKNOWN", func() {
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

		Convey("Checks that fail too many times are marked FAILED", func() {
			fail := mockCommand{DesiredResult: SICKLY}
			maxCount := 2
			badCheck := &Check{
				Type: "mock",
				Args: "testing123",
				Command: &fail,
				MaxCount: maxCount,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(maxCount)
			So(fail.CallCount, ShouldEqual, 2)
			So(badCheck.Count, ShouldEqual, 2)
			So(badCheck.Status, ShouldEqual, FAILED)
		})

		Convey("Checks that were failed return to health", func() {
			healthy := mockCommand{DesiredResult: HEALTHY}
			badCheck := &Check{
				Type: "mock",
				Status: FAILED,
				Args: "testing123",
				Command: &healthy,
				Count: 2,
			}
			monitor.AddCheck(badCheck)
			monitor.Run(1)
			So(badCheck.Count, ShouldEqual, 0)
			So(badCheck.Status, ShouldEqual, HEALTHY)

		})

		Convey("Checks that had an error become UNKNOWN on first pass", func() {
			check := NewCheck()
			check.MaxCount = 3
			check.UpdateStatus(1, errors.New("Borked!"))

			So(check.Status, ShouldEqual, UNKNOWN)
		})
	})
}
