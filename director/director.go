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
	Stop()
}

type TimedLooper struct {
	Count int
	Interval time.Duration
	WaitChan chan error
}

func (l *TimedLooper) Done(err error) {
	if l.WaitChan != nil {
		l.WaitChan <-err
	}
}

func (l *TimedLooper) Loop(fn func() error) {
	i := 0
	ticks := time.Tick(l.Interval)
	for range ticks {
		err := fn()
		if err != nil {
			l.Done(err)
			return
		}
		if l.Count != FOREVER {
			i = i + 1
			if i >= l.Count {
				l.Done(nil)
				return
			}
		}
	}
}

func (l *TimedLooper) Stop() {}
