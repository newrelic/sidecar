package discovery

import (
	"reflect"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/bosun/healthy"
	"github.com/newrelic/bosun/service"
)

const (
	STATIC_JSON = "../fixtures/static.json"
)

func Test_ParseConfig(t *testing.T) {
	Convey("ParseConfig()", t, func() {
		disco := NewStaticDiscovery()
		disco.HostnameFn = func() (string, error) { return hostname, nil }

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

		Convey("Applies hostnames to services", func() {
			parsed, _ := disco.ParseConfig(STATIC_JSON)
			So(parsed[0].Service.Hostname, ShouldEqual, hostname)
		})
	})
}

func Test_Services(t *testing.T) {
	Convey("Services()", t, func() {
		disco := NewStaticDiscovery()
		tgt1 := &Target{
			service.Service{ID: "asdf"},
			healthy.Check{ID: "asdf"},
		}
		tgt2 := &Target{
			service.Service{ID: "foofoo"},
			healthy.Check{ID: "foofoo"},
		}
		disco.Targets = []*Target{ tgt1, tgt2 }

		Convey("Returns a list of services extracted from Targets", func() {
			services := disco.Services()

			So(len(services), ShouldEqual, 2)
			So(reflect.DeepEqual(services[0], tgt1.Service), ShouldBeTrue)
			So(reflect.DeepEqual(services[1], tgt2.Service), ShouldBeTrue)
		})

		Convey("Updates the current timestamp each time", func() {
			services  := disco.Services()
			services2 := disco.Services()

			So(services[0].Updated.Before(services2[0].Updated), ShouldBeTrue)
		})
	})
}

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		disco := NewStaticDiscovery()

		Convey("Parses the specified config file", func() {
			So(len(disco.Targets), ShouldEqual, 0)
			disco.ConfigFile = STATIC_JSON
			disco.Run(make(chan bool))
			So(len(disco.Targets), ShouldEqual, 1)
		})
	})
}
