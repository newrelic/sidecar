package director

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_TimedLooper(t * testing.T) {
	Convey("TimedLooper", t, func() {
		looper := TimedLooper{1, 1 * time.Nanosecond, make(chan error), nil}

		Convey("Sends a nil on the DoneChan when everything was kosher", func() {
			go looper.Done(nil)

			result := <-looper.DoneChan
			So(result, ShouldBeNil)
		})

		Convey("Sends the error on the DoneChan when everything exploded", func() {
			err := errors.New("Borked!")
			go looper.Done(err)

			result := <-looper.DoneChan
			So(result, ShouldEqual, err)
		})

		Convey("The loop executes the function", func() {
			run := false
			go looper.Loop(func() error { run = true; return nil })
			<-looper.DoneChan

			So(run, ShouldBeTrue)
		})

		Convey("The loop executes the correct number of times", func() {
			count := 0
			looper.Count = 5
			go looper.Loop(func() error { count++; return nil })
			<-looper.DoneChan

			So(count, ShouldEqual, 5)
		})

		Convey("The loop returns an error on the DoneChan", func() {
			err := errors.New("Borked!")
			go looper.Loop(func() error { return err })
			So(<-looper.DoneChan, ShouldEqual, err)
		})

		Convey("The loop exits when told to quit", func() {
			looper.Count = FOREVER
			go looper.Loop(func() error { time.Sleep(5 * time.Nanosecond); return nil })
			looper.Quit()

			So(<-looper.DoneChan, ShouldBeNil)
		})
	})
}
