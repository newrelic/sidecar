package haproxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/newrelic/sidecar/catalog"
	"github.com/newrelic/sidecar/service"
	. "github.com/smartystreets/goconvey/convey"
)

var hostname1 = "indomitable"
var hostname2 = "indefatigable"
var hostname3 = "invincible"

func Test_HAproxy(t *testing.T) {
	Convey("End-to-end testing HAproxy functionality", t, func() {
		state := catalog.NewServicesState()
		state.Hostname = hostname1
		svcId1 := "deadbeef123"
		svcId2 := "deadbeef101"
		svcId3 := "deadbeef105"
		svcId4 := "deadbeef999"
		baseTime := time.Now().UTC().Round(time.Second)

		ports1 := []service.Port{service.Port{"tcp", 10450, 8080}, service.Port{"tcp", 10020, 9000}}
		ports2 := []service.Port{service.Port{"tcp", 9999, 8090}}
		ports3 := []service.Port{service.Port{"tcp", 32763, 8080}, service.Port{"tcp", 10020, 9000}}

		services := []service.Service{
			service.Service{
				ID:       svcId1,
				Name:     "awesome-svc",
				Image:    "awesome-svc",
				Hostname: hostname1,
				Updated:  baseTime.Add(5 * time.Second),
				Profile:  "default",
				Ports:    ports1,
			},
			service.Service{
				ID:       svcId2,
				Name:     "awesome-svc",
				Image:    "awesome-svc",
				Hostname: hostname2,
				Updated:  baseTime.Add(5 * time.Second),
				Profile:  "default",
				Ports:    ports3,
			},
			service.Service{
				ID:       svcId3,
				Name:     "some-svc",
				Image:    "some-svc",
				Hostname: hostname2,
				Updated:  baseTime.Add(5 * time.Second),
				Profile:  "default",
				Ports:    ports2,
			},
			service.Service{
				ID:       svcId4,
				Name:     "some-svc",
				Image:    "some-svc",
				Hostname: hostname2,
				Updated:  baseTime.Add(5 * time.Second),
				Profile:  "default",
				// No ports!
			},
		}

		for _, svc := range services {
			state.AddServiceEntry(svc)
		}

		proxy := New("tmpConfig", "tmpPid")
		proxy.BindIP = "192.168.168.168"
		proxy.TemplateDir = "../views/haproxy"

		Convey("New() returns a properly configured struct", func() {
			p := New("tmpConfig", "tmpPid")
			So([]byte(p.ReloadCmd), ShouldMatch, "^haproxy .*")
			So([]byte(p.VerifyCmd), ShouldMatch, "^haproxy .*")
			So([]byte(p.TemplateDir), ShouldMatch, "views/haproxy")
		})

		Convey("makePortmap() generates a properly formatted list", func() {
			result := proxy.makePortmap(state.ByService())

			So(len(result), ShouldEqual, 2)
			So(len(result[services[0].Image]), ShouldEqual, 2)
			So(len(result[services[2].Image]), ShouldEqual, 1)
		})

		Convey("servicesWithPorts() groups services by name and port", func() {
			badSvc := service.Service{
				ID:       "0000bad00000",
				Name:     "some-svc",
				Image:    "some-svc",
				Hostname: "titanic",
				Updated:  baseTime.Add(5 * time.Second),
				Ports:    []service.Port{service.Port{"tcp", 666, 6666}},
			}

			// It had 1 before
			svcList := servicesWithPorts(state)
			So(len(svcList[badSvc.Name]), ShouldEqual, 1)

			// We add an entry with mismatching ports and should get no more added
			state.AddServiceEntry(badSvc)

			svcList = servicesWithPorts(state)
			So(len(svcList[badSvc.Name]), ShouldEqual, 1)
		})

		Convey("WriteConfig()", func() {

			Convey("writes a template from a file", func() {
				buf := bytes.NewBuffer(make([]byte, 0, 2048))
				err := proxy.WriteConfig(state, buf)

				output := buf.Bytes()
				// Look at a bunch of things we should see
				So(err, ShouldBeNil)
				So(output, ShouldMatch, "frontend awesome-svc-8080")
				So(output, ShouldMatch, "backend awesome-svc-8080")
				So(output, ShouldMatch, "server.*indefatigable:10020")
				So(output, ShouldMatch, "server.*indefatigable:32763")
				So(output, ShouldMatch, "bind 192.168.168.168:9000")
				So(output, ShouldMatch, "frontend some-svc-8090")
				So(output, ShouldMatch, "backend some-svc-8090")
				So(output, ShouldMatch, "server indefatigable-deadbeef105 indefatigable:9999 cookie indefatigable-9999")
			})

			Convey("bubbles up templater errors", func() {
				proxy.TemplateDir = "/somewhere-that-doesnt-exist-at-all"
				buf := bytes.NewBuffer(make([]byte, 0, 2048))
				err := proxy.WriteConfig(state, buf)

				So(err, ShouldNotBeNil)
			})

			Convey("only writes out healthy services", func() {
				badSvc := service.Service{
					ID:       "0000bad00000",
					Name:     "some-svc-0155555789a",
					Image:    "some-svc",
					Hostname: "titanic",
					Status:   service.UNHEALTHY,
					Updated:  baseTime.Add(5 * time.Second),
					Ports:    []service.Port{service.Port{"tcp", 666, 6666}},
				}
				badSvc2 := service.Service{
					ID:       "0000bad00001",
					Name:     "some-svc-0155555789a",
					Image:    "some-svc",
					Hostname: "titanic",
					Status:   service.UNKNOWN,
					Updated:  baseTime.Add(5 * time.Second),
					Ports:    []service.Port{service.Port{"tcp", 666, 6666}},
				}
				state.AddServiceEntry(badSvc)
				state.AddServiceEntry(badSvc2)

				buf := bytes.NewBuffer(make([]byte, 0, 2048))
				proxy.WriteConfig(state, buf)

				output := buf.Bytes()
				// Look for a few things we should NOT see
				So(output, ShouldNotMatch, "0000bad00000")
				So(output, ShouldNotMatch, "0000bad00001")
			})

			Convey("renders the correct partials", func() {
				svc := service.Service{
					ID:       "000deadbeef000",
					Name:     "some-svc-123123333",
					Image:    "some-svc",
					Hostname: "titanic",
					Status:   service.ALIVE,
					Profile:  "longtimeout",
					Updated:  baseTime.Add(5 * time.Second),
					Ports:    []service.Port{service.Port{"tcp", 666, 6666}},
				}
				state.AddServiceEntry(svc)

				buf := bytes.NewBuffer(make([]byte, 0, 2048))
				proxy.WriteConfig(state, buf)

				output := buf.Bytes()
				So(output, ShouldMatch, "timeout server 605s")
				So(output, ShouldMatch, "timeout client 605s")
			})
		})

		Convey("Reload()", func() {
			Convey("doesn't return an error when it works", func() {
				proxy.ReloadCmd = "/usr/bin/true"
				err := proxy.Reload()
				So(err, ShouldBeNil)
			})

			Convey("returns an error when it fails", func() {
				proxy.ReloadCmd = "/usr/bin/false"
				err := proxy.Reload()
				So(err.Error(), ShouldEqual, "exit status 1")

				proxy.ReloadCmd = "yomomma"
				err = proxy.Reload()
				So(err.Error(), ShouldEqual, "exit status 127")
			})
		})

		Convey("WriteAndReload() bubbles up errors on failure", func() {
			proxy.ReloadCmd = "/usr/bin/false"
			tmpfile, _ := ioutil.TempFile("", "WriteAndReload")
			proxy.ConfigFile = tmpfile.Name()

			err := proxy.WriteAndReload(state)
			os.Remove(tmpfile.Name())

			So(err, ShouldNotBeNil)

		})

		Convey("sanitizeName() fixes crazy image names", func() {
			image := "public/something-longish:latest"
			So(sanitizeName(image), ShouldEqual, "public-something-longish-latest")
		})

		Convey("Watch() writes out a config when the state changes", func() {
			tmpDir, _ := ioutil.TempDir("/tmp", "sidecar-test")
			config := fmt.Sprintf("%s/haproxy.cfg", tmpDir)
			proxy.ConfigFile = config

			go proxy.Watch(state)
			newTime := time.Now().UTC()

			svc := service.Service{
				ID:       "abcdef123123125",
				Name:     "some-svc-befede6789a",
				Image:    "some-svc",
				Hostname: hostname2,
				Updated:  newTime,
				Profile:  "default",
				Ports:    []service.Port{service.Port{"tcp", 1337, 8090}},
			}
			time.Sleep(5 * time.Millisecond)
			state.AddServiceEntry(svc)
			time.Sleep(5 * time.Millisecond)

			result, _ := ioutil.ReadFile(config)
			So(result, ShouldMatch, "port 8090")

			os.Remove(config)
			os.Remove(tmpDir)
		})
	})
}

func ShouldMatch(actual interface{}, expected ...interface{}) string {
	wanted := expected[0].(string)
	got := actual.([]byte)

	wantedRegexp := regexp.MustCompile(wanted)

	if !wantedRegexp.Match(got) {
		return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\nto match:\n" + fmt.Sprintf("%v", string(got))
	}

	return ""
}

func ShouldNotMatch(actual interface{}, expected ...interface{}) string {
	unwanted := expected[0].(string)
	got := actual.([]byte)

	unwantedRegexp := regexp.MustCompile(unwanted)

	if unwantedRegexp.Match(got) {
		return "expected not to match:\n" + fmt.Sprintf("%#v", unwanted) + "\n\nbut matched:\n" + fmt.Sprintf("%v", string(got))
	}

	return ""
}
