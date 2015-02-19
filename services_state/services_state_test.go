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

func Test_ServicesStateWithData(t *testing.T) {

	Convey("When working with data", t, func() {
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

		Convey("Encode() generates JSON that we can Decode()", func() {
			decoded, err := Decode(state.Encode())

			So(err, ShouldBeNil)
			So(decoded.Servers[hostname].Name, ShouldEqual, hostname)
			So(len(decoded.Servers), ShouldEqual, 1)
		})

		Convey("Decode() returns an error when handed junk", func() {
			result, err := Decode([]byte("asdf"))

			So(result.Servers, ShouldBeEmpty)
			So(err, ShouldNotBeNil)
		})

		Convey("HasServer() is true when a server exists", func() {
			So(state.HasServer(hostname), ShouldBeTrue)
		})

		Convey("HasServer() is false when a server is missing", func() {
			So(state.HasServer("junk"), ShouldBeFalse)
		})

		Convey("AddServiceEntry()", func() {
			Convey("Merges in a new service", func() {
				So(state.HasServer(anotherHostname), ShouldBeFalse)

				state.AddServiceEntry(svc)

				So(state.HasServer(anotherHostname), ShouldBeTrue)
				So(state.Servers[anotherHostname].Services[svc.ID], ShouldNotBeNil)
			})

			Convey("Doesn't merge a stale service", func() {
				state.AddServiceEntry(svc)

				staleService := service.Service{
					ID: "deadbeef123",
					Name: "stale_service",
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

			Convey("Updates the LastUpdated time for the server", func() {
				newDate := svc.Updated.AddDate(0, 0, 5)
				svc.Updated = newDate
				state.AddServiceEntry(svc)

				So(state.Servers[anotherHostname].LastUpdated, ShouldBeTheSameTimeAs, newDate)
			})
		})

		Reset(func() {
			state = NewServicesState()
			state.Servers[hostname] = NewServer(hostname)
		})
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
