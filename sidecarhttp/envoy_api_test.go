package sidecarhttp

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	hostname = "chaucer"
	state = catalog.NewServicesState()

	baseTime = time.Now().UTC()

	svcId = "deadbeef123"
	svcId2 = "deadbeef456"
	svcId3 = "deadbeef666"

	svc = service.Service{
		ID:       svcId,
		Name:     "bocaccio",
		Image:    "101deadbeef",
		Created:  baseTime,
		Hostname: hostname,
		Updated:  baseTime,
		Status:   service.ALIVE,
		Ports: []service.Port{
			{ IP: "127.0.0.1", Port: 9999, ServicePort: 10100 },
		},
	}

	svc2 = service.Service{
		ID:       svcId2,
		Name:     "shakespeare",
		Image:    "202deadbeef",
		Created:  baseTime,
		Hostname: hostname,
		Updated:  baseTime,
		Status:   service.UNHEALTHY,
		Ports: []service.Port{
			{ IP: "127.0.0.1", Port: 9000, ServicePort: 10111 },
		},
	}

	svc3 = service.Service{
		ID:       svcId3,
		Name:     "dante",
		Image:    "666deadbeef",
		Created:  baseTime,
		Hostname: hostname,
		Updated:  baseTime,
		Status:   service.ALIVE,
	}
)

func Test_clustersHandler(t *testing.T) {
	Convey("clustersHandler()", t, func() {
		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)
		state.AddServiceEntry(svc3)

		req := httptest.NewRequest("GET", "/clusters", nil)
		recorder := httptest.NewRecorder()

		bindIP := "192.168.168.168"

		api := &EnvoyApi{state: state, config: &HttpConfig{BindIP: bindIP}}

		Convey("returns information for alive services", func() {
			api.clustersHandler(recorder, req, nil)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldContainSubstring, "bocaccio")
		})

		Convey("does not include unhealthy services", func() {
			api.clustersHandler(recorder, req, nil)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldNotContainSubstring, "shakespeare")
		})

		Convey("does not include services without a ServicePort", func() {
			api.clustersHandler(recorder, req, nil)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldNotContainSubstring, "dante")
		})
	})
}

func Test_registrationHandler(t *testing.T) {
	Convey("registrationHandler()", t, func() {
		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)
		state.AddServiceEntry(svc3)

		recorder := httptest.NewRecorder()

		bindIP := "192.168.168.168"

		api := &EnvoyApi{state: state, config: &HttpConfig{BindIP: bindIP}}

		Convey("returns an error unless a service is provided", func() {
			req := httptest.NewRequest("GET", "/registration/", nil)
			api.registrationHandler(recorder, req, nil)
			status, _, _ := getResult(recorder)

			So(status, ShouldEqual, 404)
		})

		Convey("returns an error unless port is appended", func() {
			req := httptest.NewRequest("GET", "/registration/", nil)
			params := map[string]string{
				"service":      "bocaccio",
			}
			api.registrationHandler(recorder, req, params)
			status, _, _ := getResult(recorder)

			So(status, ShouldEqual, 404)
		})

		Convey("returns information for alive services", func() {
			req := httptest.NewRequest("GET", "/registration/bocaccio:10100", nil)
			params := map[string]string{
				"service":      "bocaccio:10100",
			}
			api.registrationHandler(recorder, req, params)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldContainSubstring, "bocaccio")
		})

		Convey("does not include services without a ServicePort", func() {
			req := httptest.NewRequest("GET", "/registration/dante:12323", nil)
			params := map[string]string{
				"service":      "dante:12323",
			}
			api.registrationHandler(recorder, req, params)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 404)
			So(body, ShouldContainSubstring, "no instances of dante")
		})

		Convey("does not include unhealthy services", func() {
			req := httptest.NewRequest("GET", "/registration/shakespeare:10111", nil)
			params := map[string]string{
				"service":      "shakespeare:10111",
			}
			api.registrationHandler(recorder, req, params)
			status, _, body := getResult(recorder)

			So(body, ShouldContainSubstring, "no instances")
			So(status, ShouldEqual, 404)
		})
	})
}

func Test_listenersHandler(t *testing.T) {
	Convey("listenersHandler()", t, func() {
		state.AddServiceEntry(svc)
		state.AddServiceEntry(svc2)
		state.AddServiceEntry(svc3)

		recorder := httptest.NewRecorder()

		bindIP := "192.168.168.168"

		api := &EnvoyApi{state: state, config: &HttpConfig{BindIP: bindIP}}

		Convey("returns listeners for alive services", func() {
			req := httptest.NewRequest("GET", "/listeners/", nil)
			api.listenersHandler(recorder, req, nil)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldContainSubstring, "bocaccio")
		})

		Convey("doesn't return listeners for unhealthy services", func() {
			req := httptest.NewRequest("GET", "/listeners/", nil)
			api.listenersHandler(recorder, req, nil)
			status, _, body := getResult(recorder)

			So(status, ShouldEqual, 200)
			So(body, ShouldNotContainSubstring, "shakespeare")
		})
	})
}

// getResult fetchs the status code, headers, and body from a recorder
func getResult(recorder *httptest.ResponseRecorder) (code int, headers *http.Header, body string) {
	resp := recorder.Result()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	body = string(bodyBytes)

	return resp.StatusCode, &resp.Header, body
}
