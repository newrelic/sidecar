package catalog

import (
	"errors"
	"testing"

	"github.com/newrelic/sidecar/mockhttp"
	"github.com/newrelic/sidecar/service"
	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_NewUrlListener(t *testing.T) {
	Convey("NewUrlListener() configures all the right things", t, func() {
		url := "http://beowulf.example.com"
		listener := NewUrlListener(url)

		So(listener.Client, ShouldNotBeNil)
		So(listener.Url, ShouldEqual, url)
		So(listener.looper, ShouldNotBeNil)
	})
}

func Test_Listen(t *testing.T) {
	Convey("Listen()", t, func() {
		client := mockhttp.ClientWithExpectations([]mockhttp.HttpExpectation{
			{
				Expect:  "beowulf.example.com",
				Send:    "",
				Content: "application/json",
				Err:     errors.New("OMG!"),
			},
		})

		url := "http://beowulf.example.com"
		listener := NewUrlListener(url)
		listener.Client = client
		errors := make(chan error)
		listener.looper = director.NewFreeLooper(1, errors)

		hostname := "grendel"

		svcId1 := "deadbeef123"
		service1 := service.Service{ID: svcId1, Hostname: hostname}

		state := NewServicesState()
		state.Hostname = hostname
		state.AddServiceEntry(service1)
		state.Servers[hostname].Services[service1.ID].Tombstone()

		Convey("handles a bad post", func() {
			listener.eventChannel <-ChangeEvent{}
			listener.Retries = 0
			listener.Watch(state)
			listener.looper.Wait()

			So(len(errors), ShouldEqual, 0)
		})
	})
}
