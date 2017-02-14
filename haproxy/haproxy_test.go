package haproxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
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

		ports1 := []service.Port{{"tcp", 10450, 8080}, {"tcp", 10020, 9000}}
		ports2 := []service.Port{{"tcp", 9999, 8090}}
		ports3 := []service.Port{{"tcp", 32763, 8080}, {"tcp", 10020, 9000}}

		services := []service.Service{
			{
				ID:        svcId1,
				Name:      "awesome-svc",
				Image:     "awesome-svc",
				Hostname:  hostname1,
				Updated:   baseTime.Add(5 * time.Second),
				ProxyMode: "http",
				Ports:     ports1,
			},
			{
				ID:        svcId2,
				Name:      "awesome-svc",
				Image:     "awesome-svc",
				Hostname:  hostname2,
				Updated:   baseTime.Add(5 * time.Second),
				ProxyMode: "http",
				Ports:     ports3,
			},
			{
				ID:        svcId3,
				Name:      "some-svc",
				Image:     "some-svc",
				Hostname:  hostname2,
				Updated:   baseTime.Add(5 * time.Second),
				ProxyMode: "tcp",
				Ports:     ports2,
			},
			{
				ID:        svcId4,
				Name:      "some-svc",
				Image:     "some-svc",
				Hostname:  hostname2,
				Updated:   baseTime.Add(5 * time.Second),
				ProxyMode: "tcp",
				// No ports!
			},
		}

		for _, svc := range services {
			state.AddServiceEntry(svc)
		}

		proxy := New("tmpConfig", "tmpPid")
		proxy.BindIP = "192.168.168.168"
		proxy.Template = "../views/haproxy.cfg"

		Convey("New() returns a properly configured struct", func() {
			p := New("tmpConfig", "tmpPid")
			So([]byte(p.ReloadCmd), ShouldMatch, "^haproxy .*")
			So([]byte(p.VerifyCmd), ShouldMatch, "^haproxy .*")
			So([]byte(p.Template), ShouldMatch, "views/haproxy.cfg")
		})

		Convey("makePortmap() generates a properly formatted list", func() {
			result := proxy.makePortmap(state.ByService())

			So(len(result), ShouldEqual, 2)
			So(len(result[services[0].Image]), ShouldEqual, 2)
			So(len(result[services[2].Image]), ShouldEqual, 1)
		})

		Convey("getModes() generates a correct mode map", func() {
			result := getModes(state)
			fmt.Println(result)

			So(len(result), ShouldEqual, 2)
			So(result["awesome-svc"], ShouldEqual, "http")
			So(result["some-svc"], ShouldEqual, "tcp")
		})

		Convey("servicesWithPorts() groups services by name and port", func() {
			badSvc := service.Service{
				ID:       "0000bad00000",
				Name:     "some-svc",
				Image:    "some-svc",
				Hostname: "titanic",
				Updated:  baseTime.Add(5 * time.Second),
				Ports:    []service.Port{{"tcp", 666, 6666}},
			}

			// It had 1 before
			svcList := servicesWithPorts(state)
			So(len(svcList[badSvc.Name]), ShouldEqual, 1)

			// We add an entry with mismatching ports and should get no more added
			state.AddServiceEntry(badSvc)

			svcList = servicesWithPorts(state)
			So(len(svcList[badSvc.Name]), ShouldEqual, 1)
		})

		Convey("WriteConfig() writes a template from a file", func() {
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

		Convey("WriteConfig() bubbles up templater errors", func() {
			proxy.Template = "/"
			buf := bytes.NewBuffer(make([]byte, 0, 2048))
			err := proxy.WriteConfig(state, buf)

			So(err, ShouldNotBeNil)
		})

		Convey("WriteConfig() only writes out healthy services", func() {
			badSvc := service.Service{
				ID:       "0000bad00000",
				Name:     "some-svc-0155555789a",
				Image:    "some-svc",
				Hostname: "titanic",
				Status:   service.UNHEALTHY,
				Updated:  baseTime.Add(5 * time.Second),
				Ports:    []service.Port{{"tcp", 666, 6666}},
			}
			badSvc2 := service.Service{
				ID:       "0000bad00001",
				Name:     "some-svc-0155555789a",
				Image:    "some-svc",
				Hostname: "titanic",
				Status:   service.UNKNOWN,
				Updated:  baseTime.Add(5 * time.Second),
				Ports:    []service.Port{{"tcp", 666, 6666}},
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

		Convey("Reload() doesn't return an error when it works", func() {
			proxy.ReloadCmd = "sh -c 'exit 0'"
			err := proxy.Reload()
			So(err, ShouldBeNil)
		})

		Convey("Reload() returns an error when it fails", func() {
			proxy.ReloadCmd = "sh -c 'exit 1'"
			err := proxy.Reload()
			So(err.Error(), ShouldEqual, "exit status 1")

			proxy.ReloadCmd = "yomomma"
			err = proxy.Reload()
			So(err.Error(), ShouldEqual, "exit status 127")
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
			startTime := time.Now().Truncate(time.Second) // Localized to compare with os.Stat
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
				Ports:    []service.Port{{"tcp", 1337, 8090}},
			}
			go state.AddServiceEntry(svc)

			// We have to wait until the file has the right data in it to avoid
			// race conditions during testing. This is a little tedious and a bit
			// slow, but without substantially refactoring the HAproxy code, this
			// is seemingly the best solution.
			for {
				stat, _ := os.Stat(config)
				if stat != nil {
					if startTime.Before(stat.ModTime()) || startTime.Equal(stat.ModTime()) &&
						stat.Size() > 10000 {
						break
					}
				}
			}

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
