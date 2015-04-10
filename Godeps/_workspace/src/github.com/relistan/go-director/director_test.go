package director

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_TimedLooper(t * testing.T) {
	Convey("TimedLooper", t, func() {
		looper := NewTimedLooper(1, 1 * time.Nanosecond, make(chan error))

		Convey("Sends a nil on the DoneChan when everything was kosher", func() {
			go looper.Done(nil)

			result := looper.Wait()
			So(result, ShouldBeNil)
		})

		Convey("Sends the error on the DoneChan when everything exploded", func() {
			err := errors.New("Borked!")
			go looper.Done(err)

			result := looper.Wait()
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
			looper.Wait()

			So(count, ShouldEqual, 5)
		})

		Convey("The loop returns an error on the DoneChan", func() {
			err := errors.New("Borked!")
			go looper.Loop(func() error { return err })
			So(looper.Wait(), ShouldEqual, err)
		})

		Convey("The loop exits when told to quit", func() {
			looper.Count = FOREVER
			go looper.Loop(func() error { time.Sleep(5 * time.Nanosecond); return nil })
			looper.Quit()

			So(looper.Wait(), ShouldBeNil)
		})
	})
}

func Test_FreeLooper(t * testing.T) {
	Convey("FreeLooper", t, func() {
		looper := NewFreeLooper(1, make(chan error))

		Convey("Sends a nil on the DoneChan when everything was kosher", func() {
			go looper.Done(nil)

			result := looper.Wait()
			So(result, ShouldBeNil)
		})

		Convey("Sends the error on the DoneChan when everything exploded", func() {
			err := errors.New("Borked!")
			go looper.Done(err)

			result := looper.Wait()
			So(result, ShouldEqual, err)
		})

		Convey("The loop executes the function", func() {
			run := false
			go looper.Loop(func() error { run = true; return nil })
			looper.Wait()

			So(run, ShouldBeTrue)
		})

		Convey("The loop executes the correct number of times", func() {
			count := 0
			looper.Count = 5
			go looper.Loop(func() error { count++; return nil })
			looper.Wait()

			So(count, ShouldEqual, 5)
		})

		Convey("The loop returns an error on the DoneChan", func() {
			err := errors.New("Borked!")
			go looper.Loop(func() error { return err })
			So(looper.Wait(), ShouldEqual, err)
		})

		Convey("The loop exits when told to quit", func() {
			looper.Count = FOREVER
			go looper.Loop(func() error { time.Sleep(1 * time.Nanosecond); return nil })
			looper.Quit()

			So(looper.Wait(), ShouldBeNil)
		})
	})
}

func Example_TimedLooperWithoutQuit() {
	looper := NewTimedLooper(5, 1 * time.Nanosecond, make(chan error))

	runner := func(looper Looper) {
		x := 0
		looper.Loop(func() error {
			println(x)
			x++
			return nil
		})
	}

	go runner(looper)
	<-looper.DoneChan

	// Output:
}
