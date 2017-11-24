package discovery

import (
	"testing"

	"github.com/Nitro/sidecar/service"
	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	STATIC_JSON           = "../fixtures/static.json"
	STATIC_HOSTNAMED_JSON = "../fixtures/static-hostnamed.json"
)

func Test_ParseConfig(t *testing.T) {
	Convey("ParseConfig()", t, func() {
		ip := "127.0.0.1"
		disco := NewStaticDiscovery(STATIC_JSON, ip)
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

		Convey("Assigns the default IP address when a port doesn't have one", func() {
			parsed, _ := disco.ParseConfig(STATIC_JSON)
			So(parsed[0].Service.Ports[0].IP, ShouldEqual, ip)
		})
	})
}

func Test_Services(t *testing.T) {
	Convey("Services()", t, func() {
		ip := "127.0.0.1"
		disco := NewStaticDiscovery(STATIC_JSON, ip)
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

func Test_Listeners(t *testing.T) {
	Convey("Listeners()", t, func() {
		ip := "127.0.0.1"
		disco := NewStaticDiscovery(STATIC_JSON, ip)
		tgt1 := &Target{
			Service: service.Service{Name: "beowulf", ID: "asdf"},
			ListenPort: 10000,
		}
		tgt2 := &Target{
			Service: service.Service{Name: "hrothgar", ID: "abba"},
			ListenPort: 11000,
		}
		disco.Targets = []*Target{tgt1, tgt2}

		Convey("Returns all listeners extracted from Targets", func() {
			listeners := disco.Listeners()

			expected0 := ChangeListener{Name:"beowulf-asdf", Port:10000}
			expected1 := ChangeListener{Name:"hrothgar-abba", Port:11000}

			So(len(listeners), ShouldEqual, 2)
			So(listeners[0], ShouldResemble, expected0)
			So(listeners[1], ShouldResemble, expected1)
		})
	})
}

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		ip := "127.0.0.1"
		disco := NewStaticDiscovery(STATIC_JSON, ip)
		looper := director.NewFreeLooper(1, make(chan error))

		Convey("Parses the specified config file", func() {
			So(len(disco.Targets), ShouldEqual, 0)
			disco.Run(looper)
			So(len(disco.Targets), ShouldEqual, 1)
		})
	})
}
