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
	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_updateHandler(t *testing.T) {
	Convey("updateHandler()", t, func() {
		var received bool
		var lastReceivedState *catalog.ServicesState

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

		// Make it possible to see if we got an update, and to wait for it to happen
		rcvr := NewReceiver(10, func(state *catalog.ServicesState) { received = true; lastReceivedState = state })
		rcvr.Looper = director.NewFreeLooper(director.ONCE, nil)
		rcvr.CurrentState = state

		Convey("returns an error on an invalid JSON body", func() {
			UpdateHandler(recorder, req, rcvr)

			resp := recorder.Result()
			So(resp.StatusCode, ShouldEqual, 500)

			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			So(string(bodyBytes), ShouldContainSubstring, "unexpected end of JSON input")
		})

		Convey("updates the state and enqueues an update", func() {
			startTime := rcvr.CurrentState.LastChanged

			evtState := deepcopy.Copy(state).(*catalog.ServicesState)
			evtState.LastChanged = time.Now().UTC()

			change := catalog.StateChangedEvent{
				State: evtState,
				ChangeEvent: catalog.ChangeEvent{
					Service: service.Service{
						ID:      "10101010101",
						Updated: time.Now().UTC(),
						Created: time.Now().UTC(),
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

			So(received, ShouldBeFalse)
			rcvr.ProcessUpdates()
			So(rcvr.CurrentState.LastChanged, ShouldResemble, evtState.LastChanged)
			So(rcvr.LastSvcChanged, ShouldResemble, &change.ChangeEvent.Service)
			So(received, ShouldBeTrue)
		})

		Convey("enqueus all updates if no Subscriptions are provided", func() {
			evtState := deepcopy.Copy(state).(*catalog.ServicesState)
			evtState.LastChanged = time.Now().UTC()

			change := catalog.StateChangedEvent{
				State: evtState,
				ChangeEvent: catalog.ChangeEvent{
					Service: service.Service{
						Name:    "nobody-wants-this",
						ID:      "10101010101",
						Updated: time.Now().UTC(),
						Created: time.Now().UTC(),
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
			So(len(rcvr.ReloadChan), ShouldEqual, 1)
		})

		Convey("does not enqueue updates if the service is not subscribed to", func() {
			evtState := deepcopy.Copy(state).(*catalog.ServicesState)
			evtState.LastChanged = time.Now().UTC()

			change := catalog.StateChangedEvent{
				State: evtState,
				ChangeEvent: catalog.ChangeEvent{
					Service: service.Service{
						Name:    "nobody-wants-this",
						ID:      "10101010101",
						Updated: time.Now().UTC(),
						Created: time.Now().UTC(),
						Status:  service.ALIVE,
					},
					PreviousStatus: service.TOMBSTONE,
				},
			}

			rcvr.Subscribe("another-service")

			encoded, _ := json.Marshal(change)
			req := httptest.NewRequest("POST", "/update", bytes.NewBuffer(encoded))

			UpdateHandler(recorder, req, rcvr)
			resp := recorder.Result()

			So(resp.StatusCode, ShouldEqual, 200)
			So(len(rcvr.ReloadChan), ShouldEqual, 0)
		})

		Convey("enqueues updates if the service is subscribed to", func() {
			evtState := deepcopy.Copy(state).(*catalog.ServicesState)
			evtState.LastChanged = time.Now().UTC()

			change := catalog.StateChangedEvent{
				State: evtState,
				ChangeEvent: catalog.ChangeEvent{
					Service: service.Service{
						Name:    "subscribed-service",
						ID:      "10101010101",
						Updated: time.Now().UTC(),
						Created: time.Now().UTC(),
						Status:  service.ALIVE,
					},
					PreviousStatus: service.TOMBSTONE,
				},
			}

			rcvr.Subscribe("subscribed-service")

			encoded, _ := json.Marshal(change)
			req := httptest.NewRequest("POST", "/update", bytes.NewBuffer(encoded))

			UpdateHandler(recorder, req, rcvr)
			resp := recorder.Result()

			So(resp.StatusCode, ShouldEqual, 200)
			So(len(rcvr.ReloadChan), ShouldEqual, 1)
		})

		Convey("a copy of the state is passed to the OnUpdate func", func() {
			evtState := deepcopy.Copy(state).(*catalog.ServicesState)
			evtState.LastChanged = time.Now().UTC()

			change := catalog.StateChangedEvent{
				State: evtState,
				ChangeEvent: catalog.ChangeEvent{
					Service: service.Service{
						ID:      "10101010101",
						Updated: time.Now().UTC(),
						Created: time.Now().UTC(),
						Status:  service.ALIVE,
					},
					PreviousStatus: service.TOMBSTONE,
				},
			}

			encoded, _ := json.Marshal(change)
			req := httptest.NewRequest("POST", "/update", bytes.NewBuffer(encoded))

			UpdateHandler(recorder, req, rcvr)

			rcvr.ProcessUpdates()

			// Make sure ongoing state changes don't affect the state the receiver passes on
			state.LastChanged = time.Now().UTC()
			state.Servers["chaucer"].Services = make(map[string]*service.Service)
			So(lastReceivedState.LastChanged.Before(state.LastChanged), ShouldBeTrue)
			So(len(lastReceivedState.Servers["chaucer"].Services), ShouldEqual, 2)
		})
	})
}
