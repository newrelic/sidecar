package services_state

import (
	"encoding/json"
	"fmt"
	"testing"
	"regexp"
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

		Convey("Format() pretty-prints the state even without a Memberlist", func() {
			formatted := state.Format(nil)

			So(formatted, ShouldNotBeNil)
		})

		Reset(func() {
			state = NewServicesState()
			state.Servers[hostname] = NewServer(hostname)
		})
	})
}

func Test_Broadcasts(t *testing.T) {

	Convey("When Broadcasting services", t, func() {
		state := NewServicesState()
		state.Servers[hostname] = NewServer(hostname)
		broadcasts := make(chan [][]byte)
		quit       := make(chan bool)
		svcId1     := "deadbeef123"
		svcId2     := "deadbeef101"
		baseTime   := time.Now().UTC().Round(time.Second)

		service1 := service.Service{ ID: svcId1, Hostname: hostname, Updated: baseTime }
		service2 := service.Service{ ID: svcId2, Hostname: hostname, Updated: baseTime }
		services := []service.Service{ service1, service2 }

		containerFn := func() []service.Service {
			return services
		}

		state.HostnameFn = func() (string, error) { return hostname, nil }

		Convey("New services are serialized into the channel", func() {
			go func() { quit <- true }()
			go state.BroadcastServices(broadcasts, containerFn, quit)

			json1, _ := json.Marshal(service1)
			json2, _ := json.Marshal(service2)

			readBroadcasts := <-broadcasts
			So(len(readBroadcasts), ShouldEqual, 2)
			So(string(readBroadcasts[0]), ShouldEqual, string(json1))
			So(string(readBroadcasts[1]), ShouldEqual, string(json2))
		})

		Convey("All of the services are added to state", func() {
			go func() { quit <- true }()
			go state.BroadcastServices(broadcasts, containerFn, quit)
			<-broadcasts // Block until we get a result

			So(state.Servers[hostname].Services[svcId1], ShouldNotBeNil)
			So(state.Servers[hostname].Services[svcId2], ShouldNotBeNil)
			So(state.Servers[hostname].Services[svcId1].ID, ShouldEqual, svcId1)
			So(state.Servers[hostname].Services[svcId2].ID, ShouldEqual, svcId2)
		})

		Convey("All of the tombstones are serialized into the channel", func() {
			go func() { quit <- true }()
			junk := service.Service{ ID: "runs", Hostname: hostname, Updated: baseTime }
			state.AddServiceEntry(junk)
			state.AddServiceEntry(service1)
			state.AddServiceEntry(service2)
			go state.BroadcastTombstones(broadcasts, containerFn, quit)

			readBroadcasts := <-broadcasts
			So(len(readBroadcasts), ShouldEqual, 2) // 2 per service
			// Match with regexes since the timestamp changes during tombstoning
			So(readBroadcasts[0], ShouldMatch, "^{\"ID\":\"runs\".*\"Status\":1}$")
			So(readBroadcasts[1], ShouldMatch, "^{\"ID\":\"runs\".*\"Status\":1}$")
		})

		Convey("Services that are still alive are not tombstoned", func() {
			go func() { quit <- true }()
			state.AddServiceEntry(service1)
			state.AddServiceEntry(service2)
			go state.BroadcastTombstones(broadcasts, containerFn, quit)

			readBroadcasts := <-broadcasts
			So(len(readBroadcasts), ShouldEqual, 0)
		})

		Reset(func() {
			broadcasts = make(chan [][]byte)
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

func ShouldMatch(actual interface{}, expected ...interface{}) string {
	wanted := expected[0].(string)
	got    := actual.([]byte)

	wantedRegexp := regexp.MustCompile(wanted)

	if !wantedRegexp.Match(got) {
		return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\nto match:\n" + fmt.Sprintf("%v", string(got))
	}

	return ""
}
