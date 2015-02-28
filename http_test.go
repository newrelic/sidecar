package main

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_TimeAgo(t *testing.T) {

	Convey("TimeAgo reports durations in friendly increments", t, func() {
		baseTime    := time.Now().UTC()
		secondsAgo  := baseTime.Add(0 - 5 * time.Second)
		minsAgo     := baseTime.Add(0 - 5 * time.Minute)
		hoursAgo    := baseTime.Add(0 - 5 * time.Hour)
		daysAgo     := baseTime.Add(0 - 36 * time.Hour)
		weeksAgo    := baseTime.Add(0 - 230 * time.Hour)

		tests := map[time.Time]string{
			baseTime:    "1.0 sec ago",
			secondsAgo:  "5.0 secs ago",
			minsAgo:     "5.0 mins ago",
			hoursAgo:    "5.0 hours ago",
			daysAgo:     "1.5 days ago",
			weeksAgo:    "1.4 weeks ago",
		}

		for time, result := range tests {
			So(TimeAgo(time, baseTime), ShouldEqual, result)
		}
	})
}
