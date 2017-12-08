package receiver

import (
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/jarcoal/httpmock.v1"
	"github.com/Nitro/sidecar/catalog"
)

func Test_FetchState(t *testing.T) {
	Convey("FetchState()", t, func() {
		stateUrl := "http://localhost:7777/api/state.json"
		httpmock.Activate()
		Reset(func() {
			httpmock.DeactivateAndReset()
		})

		Convey("returns an error on a bad status code", func() {
			httpmock.RegisterResponder(
				"GET", stateUrl,
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(500, "so bad!"), nil
				},
			)
			catalog, err := FetchState(stateUrl)

			So(catalog, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Bad status code")
		})

		Convey("returns an error on a bad json body", func() {
			httpmock.RegisterResponder(
				"GET", stateUrl,
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, "so bad!"), nil
				},
			)
			catalog, err := FetchState(stateUrl)

			So(catalog, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "ffjson error")
		})

		Convey("returns a valid ServicesState on success", func() {
			state := catalog.NewServicesState()

			httpmock.RegisterResponder(
				"GET", stateUrl,
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, string(state.Encode())), nil
				},
			)
			receivedState, err := FetchState(stateUrl)

			So(receivedState, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(receivedState.Servers, ShouldNotBeNil)
		})
	})
}

func Test_IsSubscribed(t *testing.T) {
	Convey("IsSubscribed()", t, func() {
		rcvr := &Receiver{}

		Convey("returns true when a service is subscribed", func() {
			rcvr.Subscriptions = []string{"some-svc"}

			So(rcvr.IsSubscribed("some-svc"), ShouldBeTrue)
		})

		Convey("returns false when a service is NOT subscribed", func() {
			rcvr.Subscriptions = []string{"some-svc"}

			So(rcvr.IsSubscribed("another-svc"), ShouldBeFalse)
		})
	})
}
