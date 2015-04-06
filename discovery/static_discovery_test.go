package discovery

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	STATIC_JSON = "../fixtures/static.json"
)

func Test_ParseConfig(t *testing.T) {
	Convey("ParseConfig()", t, func() {
		disco := new(StaticDiscovery)

		Convey("Errors when there is a problem with the file", func() {
			_, err := disco.ParseConfig("!!!!")
			So(err, ShouldNotBeNil)
		})

		Convey("Returns a properly parsed list of Targets", func() {
			parsed, err := disco.ParseConfig(STATIC_JSON)
			So(err, ShouldBeNil)
			So(len(parsed), ShouldEqual, 1)
			So(parsed[0].Service.Ports[0].Type, ShouldEqual, "tcp")
			So(parsed[0].Check.ID, ShouldEqual, parsed[0].Service.ID)
		})
	})
}
