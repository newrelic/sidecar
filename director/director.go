package director

import (
	"time"
)

const (
	FOREVER = -1
)

type Looper interface {
	Loop(fn func() error)
	Done(error)
	Quit()
}

type TimedLooper struct {
	Count int
	Interval time.Duration
	DoneChan chan error
	QuitChan chan bool
}

func (l *TimedLooper) Done(err error) {
	if l.DoneChan != nil {
		l.DoneChan <-err
	}
}

func (l *TimedLooper) Loop(fn func() error) {
	if l.QuitChan == nil {
		l.QuitChan = make(chan bool)
	}

	i := 0
	ticks := time.Tick(l.Interval)
	for range ticks {

		err := fn()
		if err != nil {
			l.Done(err)
			return
		}

		// We have to make sure not to increment if we started
		// at -1 otherwise we quit on maxint rollover.
		if l.Count != FOREVER {
			i = i + 1
			if i >= l.Count {
				l.Done(nil)
				return
			}
		}

		select {
		case <-l.QuitChan:
			l.Done(nil)
			return
		default:
		}
	}
}

func (l *TimedLooper) Quit() {
	go func() {
		l.QuitChan <-true
	}()
}
