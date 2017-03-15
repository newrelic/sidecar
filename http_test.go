package main

import (
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
			Status:   service.ALIVE,
		}

		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)

		req := httptest.NewRequest("GET", "/services/boccacio.json", nil)
		recorder := httptest.NewRecorder()

		params := map[string]string{
			"name": "bocaccio",
			"extension": "json",
		}

		Convey("only returns JSON", func() {
			params["extension"] = "asdf"
			req := httptest.NewRequest("GET", "/services/bocaccio.asdf", nil)
			oneServiceHandler(recorder, req, nil, nil, params)

			resp := recorder.Result()
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			body := string(bodyBytes)

			So(resp.StatusCode, ShouldEqual, 404)
			So(resp.Header.Get("Content-Type"), ShouldEqual, "")
			So(body, ShouldContainSubstring, "Invalid content")
		})

		Convey("has CORS headers", func() {
			delete(params, "name")
			oneServiceHandler(recorder, req, nil, state, params)
			resp := recorder.Result()

			So(resp.StatusCode, ShouldEqual, 404)
			So(resp.Header.Get("Access-Control-Allow-Origin"), ShouldEqual, "*")
			So(resp.Header.Get("Access-Control-Allow-Methods"), ShouldEqual, "GET")
		})

		Convey("protects against a nil state", func() {
			oneServiceHandler(recorder, req, nil, nil, params)
			resp := recorder.Result()
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			body := string(bodyBytes)

			So(resp.StatusCode, ShouldEqual, 500)
			So(body, ShouldContainSubstring, "terribly wrong")
		})

		Convey("returns the contents for the service queried", func() {
			oneServiceHandler(recorder, req, nil, state, params)
			resp := recorder.Result()
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			body := string(bodyBytes)

			So(resp.StatusCode, ShouldEqual, 200)
			So(body, ShouldContainSubstring, `"bocaccio": [`)
			So(body, ShouldNotContainSubstring, `"shakespeare"`)
		})
	})
}
