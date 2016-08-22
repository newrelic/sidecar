package discovery

import (
	"testing"

	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/newrelic/sidecar/service"
)

const (
	STATIC_JSON = "../fixtures/static.json"
	STATIC_HOSTNAMED_JSON = "../fixtures/static-hostnamed.json"
)

func Test_ParseConfig(t *testing.T) {
	Convey("ParseConfig()", t, func() {
		disco := NewStaticDiscovery(STATIC_JSON)
		disco.Hostname = hostname

		Convey("Errors when there is a problem with the file", func() {
			_, err := disco.ParseConfig("!!!!")
			So(err, ShouldNotBeNil)
		})

		Convey("Returns a properly parsed list of Targets", func() {
			parsed, err := disco.ParseConfig(STATIC_JSON)
			So(err, ShouldBeNil)
			So(len(parsed), ShouldEqual, 1)
			So(parsed[0].Service.Ports[0].Type, ShouldEqual, "tcp")
		})

		Convey("Applies hostnames to services", func() {
			parsed, _ := disco.ParseConfig(STATIC_JSON)
			So(parsed[0].Service.Hostname, ShouldEqual, hostname)
		})

		Convey("Uses the given hostname when specified", func() {
			parsed, _ := disco.ParseConfig(STATIC_HOSTNAMED_JSON)
			So(parsed[0].Service.Hostname, ShouldEqual, "chaucer")
		})
	})
}

func Test_Services(t *testing.T) {
	Convey("Services()", t, func() {
		disco := NewStaticDiscovery(STATIC_JSON)
		tgt1 := &Target{
			Service: service.Service{ID: "asdf"},
		}
		tgt2 := &Target{
			Service: service.Service{ID: "foofoo"},
		}
		disco.Targets = []*Target{tgt1, tgt2}

		Convey("Returns a list of services extracted from Targets", func() {
			services := disco.Services()

			So(len(services), ShouldEqual, 2)
			So(services[0], ShouldResemble, tgt1.Service)
			So(services[1], ShouldResemble, tgt2.Service)
		})

		Convey("Updates the current timestamp each time", func() {
			services := disco.Services()
			services2 := disco.Services()

			So(services[0].Updated.Before(services2[0].Updated), ShouldBeTrue)
		})
	})
}

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		disco := NewStaticDiscovery(STATIC_JSON)
		looper := director.NewFreeLooper(1, make(chan error))

		Convey("Parses the specified config file", func() {
			So(len(disco.Targets), ShouldEqual, 0)
			disco.Run(looper)
			So(len(disco.Targets), ShouldEqual, 1)
		})
	})
}
