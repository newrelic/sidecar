package catalog

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/NinesStack/sidecar/service"
	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/jarcoal/httpmock.v1"
)

func Test_NewUrlListener(t *testing.T) {
	Convey("NewUrlListener() configures all the right things", t, func() {
		url := "http://beowulf.example.com"
		listener := NewUrlListener(url, false)

		So(listener.Client, ShouldNotBeNil)
		So(listener.Url, ShouldEqual, url)
		So(listener.looper, ShouldNotBeNil)
	})
}

func Test_prepareCookieJar(t *testing.T) {
	Convey("When preparing the cookie jar", t, func() {
		listenurl := "http://beowulf.example.com/"

		Convey("We get a properly generated cookie for our url", func() {
			jar := prepareCookieJar(listenurl)
			cookieUrl, _ := url.Parse(listenurl)
			cookies := jar.Cookies(cookieUrl)

			So(len(cookies), ShouldEqual, 1)
			So(cookies[0].Value, ShouldNotBeEmpty)
			So(cookies[0].Expires, ShouldNotBeEmpty)
		})

		Convey("We only get the right cookies", func() {
			jar := prepareCookieJar(listenurl)
			wrongUrl, _ := url.Parse("http://wrong.example.com")
			cookies := jar.Cookies(wrongUrl)

			So(len(cookies), ShouldEqual, 0)
		})
	})
}

func Test_Listen(t *testing.T) {
	Convey("Listen()", t, func() {
		url := "http://beowulf.example.com"

		httpmock.RegisterResponder(
			"POST", url,
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(500, "so bad!"), nil
			},
		)

		httpmock.Activate()
		listener := NewUrlListener(url, false)
		errors := make(chan error)
		listener.looper = director.NewFreeLooper(1, errors)

		hostname := "grendel"

		svcId1 := "deadbeef123"
		service1 := service.Service{ID: svcId1, Hostname: hostname}

		state := NewServicesState()
		state.Hostname = hostname
		state.AddServiceEntry(service1)
		state.Servers[hostname].Services[service1.ID].Tombstone()

		Reset(func() {
			httpmock.DeactivateAndReset()
		})

		Convey("handles a bad post", func() {
			listener.eventChannel <- ChangeEvent{}
			listener.Retries = 0
			listener.Watch(state)
			err := listener.looper.Wait()

			So(err, ShouldBeNil)
			So(len(errors), ShouldEqual, 0)
		})
	})
}
