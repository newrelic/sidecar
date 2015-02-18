package services_state

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_NewServer(t *testing.T) {
	hostname := "shakespeare"

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

func ShouldBeTheSameTimeAs(actual interface{}, expected ...interface{}) string {
    wanted := expected[0].(time.Time)
    got    := actual.(time.Time)

    if !got.Equal(wanted) {
        return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\ngot:\n" + fmt.Sprintf("%#v", got)
    }

    return ""
}
