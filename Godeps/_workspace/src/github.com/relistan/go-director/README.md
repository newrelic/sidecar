Director
========

This package is built to make it easy to write and to test background
goroutines. These are the kinds of goroutines that are meant to have a
reasonably long lifespan built around a central loop. This is often a `for {}`
loop with no conditions.

The interface allows routines to be dispatched and run for a set number of
iterations, or indefinitely. You can also signal them to quit, and block
waiting for them to complete.

The core interface for the package is the `Looper`. Two `Looper`
implementations are currently included, a `TimedLooper` whichs runs the loop on
a specified interval, and a `FreeLooper` which runs the loop as quickly as
possible.

This is really a convenience package to prevent writing boilerplate over and
over, and to make it easy to test your code.

The `Looper` interface looks like this:

```go
type Looper interface {
	// To be called when we want to run the inner loop. Used by the
	// dependant code.
	Loop(fn func() error)
	// Called by dependant routine. Block waiting for the loop to end
	Wait() error
	// Signal that the routine is done. Generally used internally
	Done(err error)
	// Externally signal the long-lived goroutine to complete work
	Quit()
}
```

Here's an example goroutine that could benefit from a `FreeLooper`:

```go
func RunForever() error {
	for {
		... do some work ...
		if err != nil {
			return err
		}
	}
}

go RunForever()
```

This works but it kind of stinks, because we can't easily test the code with
`go test` and we can't capture our error. If we start this up, it will never
exit, which is what we want it to do in our production code. But we want it to
stop after running in test code. To do that, we need to have a way to get the
code to quit after iterating. So we can do something like this:

```go
func RunForever(quit chan bool) error {
	for {
		... do some work ...
		if err != nil {
			return err
		}

		select {
		case <-quit:
			return nil
		}
	}
}

quit := make(chan bool, 1)
quit <-true
go RunForever(quit)
```

Now we can tell it to quit when we want to. We probably wanted that to begin
with, so that the main program can tell the goroutine to end. But it also now
means we can test it by using a buffered channel, putting a message into the
channel, then running the test.

But what about when we want to run it more than once in a pass? Or when we want
to have our code wait on its completion somewhere during execution? These are
all common patterns and require boilerplate code.  If you do that once in your
program, fine. But it's often the case that this proliferates all over the
code. Particularly for applications which are doing more than one thing in the
background. Instead we could use a `FreeLooper` like this:

```go
func RunForever(looper Looper) error {
	looper.Loop(func() error {
		... do some work ...
		if err != nil {
			return err
		}

		select {
		case <-quit:
			return nil
		}
	})
}

looper := NewFreeLooper(1, make(chan error))
go RunForever(looper)

err := looper.Wait()
if err != nil {
	... handle it ...
}
```

That will run the loop once, and wait for it to complete, handling the
resulting error.

Or we, can tell it to run forever, and then stop it when we want to:

```go
looper := NewFreeLooper(FOREVER, make(chan error))
go RunForever(looper)

... do more work ...

looper.Quit()
err := looper.Wait()
if err != nil {
	... handle it ...
}

```

And if on a later basis we want this to run as a timed loop, such as one
iteration per 5 seconds, we can just substitute a `TimedLooper`:

```go
looper := NewTimedLooper(1, 5 * time.Second, make(chan error))
go RunForever(looper)
```
