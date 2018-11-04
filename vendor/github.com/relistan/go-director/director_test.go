package director

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_TimedLooper(t *testing.T) {
	Convey("TimedLooper", t, func() {
		looper := NewTimedLooper(1, 1*time.Nanosecond, make(chan error))

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
			So(looper.Wait(), ShouldBeNil)

			So(count, ShouldEqual, 5)
		})

		Convey("The loop returns an error on the DoneChan", func() {
			err := errors.New("Borked!")
			go looper.Loop(func() error { return err })
			So(looper.Wait(), ShouldEqual, err)
		})

		Convey("The loop exits when told to quit", func() {
			looper.Count = FOREVER
			count := 0

			go looper.Loop(func() error { count++; time.Sleep(2 * time.Nanosecond); return nil })
			looper.Quit()

			So(looper.Wait(), ShouldBeNil)

			previousCount := count
			time.Sleep(1 * time.Millisecond)
			So(count, ShouldEqual, previousCount)
		})
	})
}

func Test_NewImmediateTimedLooper(t *testing.T) {
	Convey("ImmediateTimedLooper", t, func() {
		looper := NewImmediateTimedLooper(10, 1*time.Nanosecond, make(chan error))

		Convey("Immediate looper must have immediate set to true", func() {
			So(looper.Immediate, ShouldBeTrue)
		})
	})
}

func Test_FreeLooper(t *testing.T) {
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
			So(looper.Wait(), ShouldBeNil)

			So(run, ShouldBeTrue)
		})

		Convey("The loop executes the correct number of times", func() {
			count := 0
			looper.Count = 5
			go looper.Loop(func() error { count++; return nil })
			So(looper.Wait(), ShouldBeNil)

			So(count, ShouldEqual, 5)
		})

		Convey("The loop returns an error on the DoneChan", func() {
			err := errors.New("Borked!")
			go looper.Loop(func() error { return err })
			So(looper.Wait(), ShouldEqual, err)
		})

		Convey("The loop exits when told to quit", func() {
			looper.Count = FOREVER
			count := 0

			go looper.Loop(func() error { count++; time.Sleep(2 * time.Nanosecond); return nil })
			looper.Quit()

			So(looper.Wait(), ShouldBeNil)

			previousCount := count
			time.Sleep(1 * time.Millisecond)
			So(count, ShouldEqual, previousCount)
		})
	})
}

// In this example, we run a really fast TimedLooper for a
// fixed number of runs.
func ExampleTimedLooper() {
	looper := NewTimedLooper(5, 1*time.Nanosecond, make(chan error))

	runner := func(looper Looper) {
		x := 0
		looper.Loop(func() error {
			fmt.Println(x)
			x++
			return nil
		})
	}

	go runner(looper)
	err := looper.Wait()
	if err != nil {
		fmt.Printf("I got an error: %s\n", err.Error())
	}

	// Output:
	// 0
	// 1
	// 2
	// 3
	// 4
}

// In this example we run a really fast TimedLooper for a fixed
// number of runs, but we interrupt it with a Quit() call so
// it only completes one run.
func ExampleTimedLooper_Quit() {
	looper := NewTimedLooper(5, 50*time.Millisecond, make(chan error))

	runner := func(looper Looper) {
		x := 0
		looper.Loop(func() error {
			fmt.Println(x)
			x++
			return nil
		})
	}

	go runner(looper)
	// Wait for one run to complete
	time.Sleep(90 * time.Millisecond)
	looper.Quit()
	err := looper.Wait()
	if err != nil {
		fmt.Printf("I got an error: %s\n", err.Error())
	}

	// Output:
	// 0
}

// In this example, we are going to run a FreeLooper with 5 iterations.
// In the course of running, an error is generated, which the parent
// function captures and outputs. As a result of the error only 3
// of the 5 iterations are completed and the output reflects this.
func Example() {
	looper := NewFreeLooper(5, make(chan error))

	runner := func(looper Looper) {
		x := 0
		looper.Loop(func() error {
			fmt.Println(x)
			x++
			if x == 3 {
				return errors.New("Uh oh")
			}
			return nil
		})
	}

	go runner(looper)
	err := looper.Wait()

	if err != nil {
		fmt.Printf("I got an error: %s\n", err.Error())
	}

	// Output:
	// 0
	// 1
	// 2
	// I got an error: Uh oh
}
