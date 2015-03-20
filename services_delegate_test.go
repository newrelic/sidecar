package main

import (
	"testing"

	"github.com/newrelic/bosun/services_state"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_GetBroadcasts(t *testing.T) {
	Convey("When handing back broadcast messages", t, func() {
		state    := services_state.NewServicesState()
		delegate := NewServicesDelegate(state)
		bCast    := [][]byte{
			[]byte(`{"ID":"d419fa7ad1a7","Name":"/dockercon-6adfe629eebc91","Image":"nginx:latest","Created":"2015-02-25T19:04:46Z","Hostname":"docker2","Ports":[{"Type":"tcp","Port":10234}],"Updated":"2015-03-04T01:12:46.669648453Z","Status":0}`),
			[]byte(`{"ID":"deadbeefabba","Name":"/dockercon-6c01869525db08","Image":"nginx:latest","Created":"2015-02-25T19:04:46Z","Hostname":"docker2","Ports":[{"Type":"tcp","Port":10234}],"Updated":"2015-03-04T01:12:46.669648453Z","Status":0}`),
		}
		bCast2   := [][]byte{
			[]byte(`{"ID":"1b3295bf300f","Name":"/romantic_brown","Image":"0415448f2cc2","Created":"2014-10-02T23:58:48Z","Hostname":"docker1","Ports":[{"Type":"tcp","Port":9494}],"Updated":"2015-03-04T01:12:32.630357657Z","Status":0}`),
			[]byte(`{"ID":"deadbeefabba","Name":"/dockercon-6c01869525db08","Image":"nginx:latest","Created":"2015-02-25T19:04:46Z","Hostname":"docker2","Ports":[{"Type":"tcp","Port":10234}],"Updated":"2015-03-04T01:12:46.669648453Z","Status":0}`),
		}

		Convey("GetBroadcasts()", func() {
			Convey("Returns nil when there is nothing to send", func() {
				So(delegate.GetBroadcasts(3, 1398), ShouldBeNil)
			})

			Convey("Returns from the pending list when nothing in the channel", func() {
				data := []byte("data")
				delegate.pendingBroadcasts = [][]byte{data}

				result := delegate.GetBroadcasts(3, 1398)
				So(string(result[0]), ShouldEqual, string(data)) 
				So(len(result), ShouldEqual, 1)
			})

			Convey("Returns what's in the channel", func() {
				state.Broadcasts = make(chan [][]byte, 1)
				state.Broadcasts <-bCast
				result := delegate.GetBroadcasts(3, 1398)

				So(len(result), ShouldEqual, 2)
				So(string(result[0]), ShouldEqual, string(bCast[0]))
				So(string(result[1]), ShouldEqual, string(bCast[1]))
				So(len(delegate.pendingBroadcasts), ShouldEqual, 0)
			})

			Convey("Returns what's left when nothing is new", func() {
				delegate.pendingBroadcasts = bCast

				result := delegate.GetBroadcasts(3, 1398)
				So(len(result), ShouldEqual, 2)
				So(string(result[0]), ShouldEqual, string(bCast[0]))
				So(string(result[1]), ShouldEqual, string(bCast[1]))
				So(len(delegate.pendingBroadcasts), ShouldEqual, 0)
			})

			Convey("Returns what's left and what's new when it fits", func() {
				state.Broadcasts = make(chan [][]byte, 1)
				delegate.pendingBroadcasts = bCast
				state.Broadcasts <-bCast2

				result := delegate.GetBroadcasts(3, 1398)
				So(len(result), ShouldEqual, 4)
				for i, entry := range(append(bCast2, bCast...)) {
					So(string(result[i]), ShouldEqual, string(entry))
				}
				So(len(delegate.pendingBroadcasts), ShouldEqual, 0)
			})

			Convey("Many runs with leftovers don't leave junk or bad buffers", func() {
				state.Broadcasts = make(chan [][]byte, 1)
				delegate.pendingBroadcasts = bCast
				state.Broadcasts <-append(bCast2, bCast...)

				delegate.GetBroadcasts(3, 100)
				delegate.GetBroadcasts(3, 300) // 1 message fits here
				delegate.GetBroadcasts(3, 100)

				result := delegate.GetBroadcasts(3, 1398)
				So(len(result), ShouldEqual, 5)
				for i, entry := range(append(bCast2[1:], bCast...)) {
					So(string(result[i]), ShouldEqual, string(entry))
				}
				So(len(delegate.pendingBroadcasts), ShouldEqual, 0)
			})
		})
	})
}
