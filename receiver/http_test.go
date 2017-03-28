package receiver

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	"github.com/mohae/deepcopy"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_updateHandler(t *testing.T) {
	Convey("updateHandler()", t, func() {
		rcvr := &Receiver{
			ReloadChan: make(chan time.Time, 10),
		}
		rcvr.OnUpdate = func(state *catalog.ServicesState) { rcvr.EnqueueUpdate() }

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

		req := httptest.NewRequest("POST", "/update", nil)
		recorder := httptest.NewRecorder()

		rcvr.CurrentState = state

		Convey("returns an error on an invalid JSON body", func() {
			UpdateHandler(recorder, req, rcvr)

			resp := recorder.Result()
			So(resp.StatusCode, ShouldEqual, 500)

			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			So(string(bodyBytes), ShouldContainSubstring, "unexpected end of JSON input")
		})

		Convey("updates the state", func() {
			startTime := rcvr.CurrentState.LastChanged

			var evtState *catalog.ServicesState
			evtState = deepcopy.Copy(state).(*catalog.ServicesState)
			evtState.LastChanged = time.Now().UTC()

			change := catalog.StateChangedEvent{
				State: *evtState,
				ChangeEvent: catalog.ChangeEvent{
					Service: service.Service{
						ID:      "10101010101",
						Updated: time.Now().UTC(),
						Status:  service.ALIVE,
					},
					PreviousStatus: service.TOMBSTONE,
				},
			}

			encoded, _ := json.Marshal(change)
			req := httptest.NewRequest("POST", "/update", bytes.NewBuffer(encoded))

			UpdateHandler(recorder, req, rcvr)
			resp := recorder.Result()

			So(resp.StatusCode, ShouldEqual, 200)
			So(startTime.Before(rcvr.CurrentState.LastChanged), ShouldBeTrue)
			So(rcvr.CurrentState.LastChanged, ShouldResemble, evtState.LastChanged)
			So(rcvr.LastSvcChanged, ShouldResemble, &change.ChangeEvent.Service)
			So(len(rcvr.ReloadChan), ShouldEqual, 1)
		})
	})
}
