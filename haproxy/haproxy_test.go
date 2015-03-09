package haproxy

import (
	"bytes"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/services_state"
	. "github.com/smartystreets/goconvey/convey"
)

var hostname1 = "indomitable"
var hostname2 = "indefatigable"
var hostname3 = "invincible"


func Test_HAproxy(t * testing.T) {
	state    := services_state.NewServicesState()
    svcId1   := "deadbeef123"
    svcId2   := "deadbeef101"
    svcId3   := "deadbeef105"
    baseTime := time.Now().UTC().Round(time.Second)

	ports1   := []service.Port{service.Port{"tcp", 10450}, service.Port{"tcp", 10020}}
	ports2   := []service.Port{service.Port{"tcp", 9999}}

	services := []service.Service{
		service.Service{
			ID: svcId1,
			Name: "awesome-svc-adfffed1233",
			Image: "awesome-svc",
			Hostname: hostname1,
			Updated: baseTime.Add(5 * time.Second),
			Ports: ports1,
		},
		service.Service{
			ID: svcId2,
			Name: "awesome-svc-1234fed1233",
			Image: "awesome-svc",
			Hostname: hostname2,
			Updated: baseTime.Add(5 * time.Second),
			Ports: ports1,
		},
		service.Service{
			ID: svcId3,
			Name: "some-svc-0123456789a",
			Image: "some-svc",
			Hostname: hostname2,
			Updated: baseTime.Add(5 * time.Second),
			Ports: ports2,
		},
		service.Service{
			ID: svcId3,
			Name: "some-svc-befede6789a",
			Image: "some-svc",
			Hostname: hostname2,
			Updated: baseTime.Add(5 * time.Second),
		},
	}

    state.HostnameFn = func() (string, error) { return hostname1, nil }

	for _, svc := range services {
		state.AddServiceEntry(svc)
	}

	proxy := HAproxy{BindIP: "192.168.168.168"}

	Convey("makePortmap() generates a properly formatted list", t, func() {
		result := proxy.makePortmap(state.ByService())

		So(len(result), ShouldEqual, 2)
		So(len(result[services[0].Image]), ShouldEqual, 2)
		So(len(result[services[2].Image]), ShouldEqual, 1)
	})

	Convey("WriteConfig() writes a template from a file", t, func() {
		buf := bytes.NewBuffer(make([]byte, 0, 2048))
		proxy.WriteConfig(state, buf)

		output := buf.Bytes()
		// Look at a bunch of things we should see
		So(output, ShouldMatch, "frontend awesome-svc-10450")
		So(output, ShouldMatch, "backend awesome-svc-10450")
		So(output, ShouldMatch, "server.*indefatigable:10020")
		So(output, ShouldMatch, "bind 192.168.168.168:10020")
		So(output, ShouldMatch, "frontend some-svc-9999")
		So(output, ShouldMatch, "backend some-svc-9999")
		So(output, ShouldMatch, "server some-svc-0123456789a indefatigable:9999 cookie some-svc-0123456789a-9999")
	})
}

func ShouldMatch(actual interface{}, expected ...interface{}) string {
	wanted := expected[0].(string)
	got	   := actual.([]byte)

	wantedRegexp := regexp.MustCompile(wanted)

	if !wantedRegexp.Match(got) {
		return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\nto match:\n" + fmt.Sprintf("%v", string(got))
	}

	return ""
}
