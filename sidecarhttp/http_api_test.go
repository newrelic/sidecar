package sidecarhttp

import (
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_oneServiceHandler(t *testing.T) {
	Convey("oneServiceHandler", t, func() {
		hostname := "chaucer"
		state := catalog.NewServicesState()
		state.Servers[hostname] = catalog.NewServer(hostname)

		baseTime := time.Now().UTC()

		svcId := "deadbeef123"
		svcId2 := "deadbeef456"

		svc := service.Service{
			ID:       svcId,
			Name:     "bocaccio",
			Image:    "101deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.ALIVE,
		}

		svc2 := service.Service{
			ID:       svcId2,
			Name:     "shakespeare",
			Image:    "202deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.UNHEALTHY,
		}

		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)

		req := httptest.NewRequest("GET", "/services/boccacio.json", nil)
		recorder := httptest.NewRecorder()

		api := &SidecarApi{state: state}

		params := map[string]string{
			"name":      "bocaccio",
			"extension": "json",
		}

		Convey("only returns JSON", func() {
			params["extension"] = "asdf"
			req := httptest.NewRequest("GET", "/services/bocaccio.asdf", nil)
			api.oneServiceHandler(recorder, req, params)

			status, headers, body := getResult(recorder)

			So(body, ShouldContainSubstring, "Invalid content")
			So(status, ShouldEqual, 404)
			So(headers.Get("Content-Type"), ShouldEqual, "application/json")
		})

		Convey("has CORS headers", func() {
			delete(params, "name")
			api.oneServiceHandler(recorder, req, params)

			status, headers, _ := getResult(recorder)

			So(status, ShouldEqual, 404)
			So(headers.Get("Access-Control-Allow-Origin"), ShouldEqual, "*")
			So(headers.Get("Access-Control-Allow-Methods"), ShouldEqual, "GET")
		})

		Convey("protects against a nil state", func() {
			api.state = nil
			api.oneServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 500)
			So(body, ShouldContainSubstring, "terribly wrong")
		})

		Convey("returns the contents for the service queried", func() {
			api.oneServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldContainSubstring, `"bocaccio": [`)
			So(body, ShouldNotContainSubstring, `"shakespeare"`)
		})

		Convey("sends a 404 for unknown services", func() {
			params["name"] = "garbage"
			api.oneServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 404)
			So(body, ShouldContainSubstring, `no instances of garbage`)
			So(body, ShouldNotContainSubstring, `"shakespeare"`)
			So(body, ShouldNotContainSubstring, `"bocaccio"`)
		})
	})
}

func Test_stateHandler(t *testing.T) {
	Convey("stateHandler", t, func() {
		hostname := "chaucer"
		state := catalog.NewServicesState()
		state.Servers[hostname] = catalog.NewServer(hostname)

		baseTime := time.Now().UTC()

		svcId := "deadbeef123"
		svcId2 := "deadbeef456"

		svc := service.Service{
			ID:       svcId,
			Name:     "bocaccio",
			Image:    "101deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.ALIVE,
		}

		svc2 := service.Service{
			ID:       svcId2,
			Name:     "shakespeare",
			Image:    "202deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.ALIVE,
		}

		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)

		req := httptest.NewRequest("GET", "/state.json", nil)
		recorder := httptest.NewRecorder()

		api := &SidecarApi{state: state}

		params := map[string]string{
			"extension": "json",
		}

		Convey("returns an error for unknown content types", func() {
			params["extension"] = ""
			api.stateHandler(recorder, req, params)

			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 404)
			So(body, ShouldContainSubstring, `Invalid content type`)
			So(body, ShouldNotContainSubstring, `"shakespeare"`)
			So(body, ShouldNotContainSubstring, `"bocaccio"`)
		})

		Convey("returns the encoded state", func() {
			api.stateHandler(recorder, req, params)
			resp := recorder.Result()
			bodyBytes, _ := ioutil.ReadAll(resp.Body)

			So(resp.StatusCode, ShouldEqual, 200)

			decoded, err := catalog.Decode(bodyBytes)
			So(err, ShouldBeNil)
			So(decoded, ShouldNotBeNil)
			So(decoded.Servers, ShouldResemble, state.Servers)
		})

	})
}

func Test_servicesHandler(t *testing.T) {
	Convey("servicesHandler", t, func() {
		hostname := "chaucer"
		state := catalog.NewServicesState()
		state.Servers[hostname] = catalog.NewServer(hostname)

		baseTime := time.Now().UTC()

		svcId := "deadbeef123"
		svcId2 := "deadbeef456"

		svc := service.Service{
			ID:       svcId,
			Name:     "bocaccio",
			Image:    "101deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.ALIVE,
		}

		svc2 := service.Service{
			ID:       svcId2,
			Name:     "shakespeare",
			Image:    "202deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.ALIVE,
		}

		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)

		req := httptest.NewRequest("GET", "/state.json", nil)
		recorder := httptest.NewRecorder()

		api := &SidecarApi{state: state}

		params := map[string]string{
			"extension": "json",
		}

		Convey("returns an error for unknown content types", func() {
			params["extension"] = ""
			api.servicesHandler(recorder, req, params)

			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 404)
			So(body, ShouldContainSubstring, `Invalid content type`)
			So(body, ShouldNotContainSubstring, `"shakespeare"`)
			So(body, ShouldNotContainSubstring, `"bocaccio"`)
		})

		Convey("returns the encoded state", func() {
			api.servicesHandler(recorder, req, params)
			resp := recorder.Result()
			bodyBytes, _ := ioutil.ReadAll(resp.Body)

			So(resp.StatusCode, ShouldEqual, 200)

			var result ApiServices
			err := json.Unmarshal(bodyBytes, &result)
			So(err, ShouldBeNil)
			So(len(result.Services), ShouldEqual, 2)
		})
	})
}

type respRecorder struct {
	*httptest.ResponseRecorder
	closeNotifier chan bool
}

func (r *respRecorder) CloseNotify() <-chan bool {
	return r.closeNotifier
}

func Test_watchHandler(t *testing.T) {
	Convey("When invoking the watcher handler", t, func() {
		dummyReq := httptest.NewRequest("GET", "/watch", nil)

		dummyResp := &respRecorder{
			ResponseRecorder: httptest.NewRecorder(),
			closeNotifier:    make(chan bool, 1),
		}

		dummyState := catalog.NewServicesState()

		currentTime := time.Now().UTC()
		dummyState.AddServiceEntry(
			service.Service{
				ID:       "42",
				Name:     "dummy_service",
				Image:    "dummy_image",
				Created:  currentTime,
				Hostname: "dummy_host",
				Updated:  currentTime,
				Status:   service.ALIVE,
			},
		)

		api := &SidecarApi{state: dummyState}

		Convey("Returns state", func() {
			expectedPayload, err := json.Marshal(dummyState)
			if err != nil {
				So(err, ShouldBeNil)
			}

			q := dummyReq.URL.Query()
			q.Add("by_service", "false")
			dummyReq.URL.RawQuery = q.Encode()

			dummyResp.closeNotifier <- true
			api.watchHandler(dummyResp, dummyReq, nil)

			So(dummyResp.Body.String(), ShouldEqual, string(expectedPayload))
		})

		Convey("Returns state by service", func() {
			expectedPayload, err := json.Marshal(dummyState.ByService())
			if err != nil {
				So(err, ShouldBeNil)
			}

			dummyResp.closeNotifier <- true
			api.watchHandler(dummyResp, dummyReq, nil)

			So(dummyResp.Body.String(), ShouldEqual, string(expectedPayload))
		})
	})
}
