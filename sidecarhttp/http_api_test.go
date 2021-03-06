package sidecarhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NinesStack/sidecar/catalog"
	"github.com/NinesStack/sidecar/service"
	director "github.com/relistan/go-director"
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

func Test_watchHandler(t *testing.T) {
	Convey("When invoking the watcher handler", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		dummyReq := httptest.NewRequest("GET", "/watch", nil)
		dummyReq = dummyReq.WithContext(ctx)

		dummyResp := httptest.NewRecorder()

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

			cancel()
			api.watchHandler(dummyResp, dummyReq, nil)

			So(dummyResp.Body.String(), ShouldEqual, string(expectedPayload))
		})

		Convey("Returns state by service", func() {
			expectedPayload, err := json.Marshal(dummyState.ByService())
			if err != nil {
				So(err, ShouldBeNil)
			}

			cancel()
			api.watchHandler(dummyResp, dummyReq, nil)

			So(dummyResp.Body.String(), ShouldEqual, string(expectedPayload))
		})
	})
}

func Test_drainServiceHandler(t *testing.T) {
	Convey("When invoking the drainService handler", t, func() {
		hostname := "chaucer"
		state := catalog.NewServicesState()
		state.Hostname = hostname
		state.Servers[hostname] = catalog.NewServer(hostname)

		baseTime := time.Now().UTC().Add(0 - 1*time.Minute)

		svcId := "deadbeef123"
		svc := service.Service{
			ID:       svcId,
			Name:     "bocaccio",
			Image:    "101deadbeef",
			Created:  baseTime,
			Hostname: hostname,
			Updated:  baseTime,
			Status:   service.ALIVE,
		}

		state.AddServiceEntry(svc)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/services/%s/drain", svcId), nil)
		recorder := httptest.NewRecorder()

		api := &SidecarApi{state: state}

		params := map[string]string{
			"id": svcId,
		}

		Convey("Marks the service as DRAINING", func() {
			api.drainServiceHandler(recorder, req, params)

			// Make sure we merge the state update
			state.ProcessServiceMsgs(director.NewFreeLooper(director.ONCE, nil))

			status, _, body := getResult(recorder)
			So(status, ShouldEqual, 202)
			So(body, ShouldContainSubstring, "set to DRAINING")

			So(state.Servers[hostname].HasService(svcId), ShouldBeTrue)
			So(state.Servers[hostname].Services[svcId].Status, ShouldEqual, service.DRAINING)

			Convey("and the service doesn't flip back to ALIVE", func() {
				svc.Status = service.ALIVE
				svc.Updated = time.Now().UTC()
				state.UpdateService(svc)

				// Make sure we merge the state update
				state.ProcessServiceMsgs(director.NewFreeLooper(director.ONCE, nil))

				So(state.Servers[hostname].HasService(svcId), ShouldBeTrue)
				So(state.Servers[hostname].Services[svcId].Status, ShouldEqual, service.DRAINING)
			})
		})

		Convey("Returns an error for non-POST requests", func() {
			req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/services/%s/drain", svcId), nil)

			api.drainServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)
			So(status, ShouldEqual, 400)
			So(body, ShouldContainSubstring, "not allowed")
		})

		Convey("Returns an error if the service ID is not provided", func() {
			delete(params, "id")
			api.drainServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)
			So(status, ShouldEqual, 404)
			So(body, ShouldContainSubstring, "No service ID provided")
		})

		Convey("Returns an error if the state is nil", func() {
			api.state = nil
			api.drainServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)
			So(status, ShouldEqual, 500)
			So(body, ShouldContainSubstring, "Something went terribly wrong")
		})

		Convey("Returns an error if no service is found for the received ID", func() {
			params["id"] = "missing"
			api.drainServiceHandler(recorder, req, params)

			status, _, body := getResult(recorder)
			So(status, ShouldEqual, 404)
			So(body, ShouldContainSubstring, "not found")
		})
	})
}
