package services_state

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/service"
)

var hostname = "shakespeare"
var anotherHostname = "chaucer"

func Test_NewServer(t *testing.T) {

	Convey("Invoking NewServer()", t, func() {
		Convey("Returns a server with the correct name", func() {
			server := NewServer(hostname)
			So(server.Name, ShouldEqual, hostname)
		})

		Convey("Initializes the map", func() {
			server := NewServer(hostname)
			So(server.Services, ShouldNotBeNil)
		})

		Convey("Initializes the time", func() {
			server := NewServer(hostname)
			So(server.LastUpdated, ShouldBeTheSameTimeAs, time.Unix(0, 0))
		})
	})
}

func Test_NewServicesState(t *testing.T) {
	Convey("Invoking NewServicesState()", t, func() {

		Convey("Initializes the Servers map", func() {
			state := NewServicesState()
			So(state.Servers, ShouldNotBeNil)
		})

	})
}

func Test_ServicesState(t *testing.T) {
	state := NewServicesState()
	state.Servers[hostname] = NewServer(hostname)

	baseTime := time.Now().UTC()

	svc := service.Service{
		ID: "deadbeef123",
		Name: "radical_service",
		Image: "101deadbeef",
		Created: baseTime,
		Hostname: anotherHostname,
		Updated: baseTime,
		Status: service.ALIVE,
	}

	Convey("Encode() generates JSON that we can Decode()", t, func() {
		decoded, err := Decode(state.Encode())

		So(err, ShouldBeNil)
		So(decoded.Servers[hostname].Name, ShouldEqual, hostname)
		So(len(decoded.Servers), ShouldEqual, 1)
	})

	Convey("HasServer() is true when a server exists", t, func() {
		So(state.HasServer(hostname), ShouldBeTrue)
		So(state.HasServer("junk"), ShouldBeFalse)
	})

	Convey("AddServiceEntry() merges in a new service", t, func() {
		So(state.HasServer(anotherHostname), ShouldBeFalse)

		state.AddServiceEntry(svc)

		So(state.HasServer(anotherHostname), ShouldBeTrue)
		So(state.Servers[anotherHostname].Services[svc.ID], ShouldNotBeNil)
	})

	Convey("AddServiceEntry() doesn't merge a stale service", t, func() {
		state.Servers[anotherHostname].Services[svc.ID] = &svc

		staleService := service.Service{
			ID: "deadbeef123",
			Name: "radical_service",
			Image: "stale",
			Created: baseTime,
			Hostname: anotherHostname,
			Updated: baseTime.Add(0 - 1 * time.Minute),
			Status: service.ALIVE,
		}

		state.AddServiceEntry(staleService)

		So(state.HasServer(anotherHostname), ShouldBeTrue)
		So(state.Servers[anotherHostname].Services[svc.ID].Updated,
			ShouldBeTheSameTimeAs, baseTime)
		So(state.Servers[anotherHostname].Services[svc.ID].Image,
			ShouldEqual, "101deadbeef")
	})
}


func ShouldBeTheSameTimeAs(actual interface{}, expected ...interface{}) string {
    wanted := expected[0].(time.Time)
    got    := actual.(time.Time)

    if !got.Equal(wanted) {
        return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\ngot:\n" + fmt.Sprintf("%#v", got)
    }

    return ""
}
