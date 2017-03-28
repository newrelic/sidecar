package receiver

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/Nitro/sidecar/catalog"
	log "github.com/Sirupsen/logrus"
)

type ApiErrors struct {
	Errors []string `json:"errors"`
}

// Receives POSTed state updates from a Sidecar instance
func UpdateHandler(response http.ResponseWriter, req *http.Request, rcvr *Receiver) {
	defer req.Body.Close()
	response.Header().Set("Content-Type", "application/json")

	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		message, _ := json.Marshal(ApiErrors{[]string{err.Error()}})
		response.WriteHeader(http.StatusInternalServerError)
		response.Write(message)
		return
	}

	var evt catalog.StateChangedEvent
	err = json.Unmarshal(data, &evt)
	if err != nil {
		message, _ := json.Marshal(ApiErrors{[]string{err.Error()}})
		response.WriteHeader(http.StatusInternalServerError)
		response.Write(message)
		return
	}

	rcvr.StateLock.Lock()
	defer rcvr.StateLock.Unlock()

	if rcvr.CurrentState == nil || rcvr.CurrentState.LastChanged.Before(evt.State.LastChanged) {
		rcvr.CurrentState = &evt.State
		rcvr.LastSvcChanged = &evt.ChangeEvent.Service

		if ShouldNotify(evt.ChangeEvent.PreviousStatus, evt.ChangeEvent.Service.Status) {
			if rcvr.OnUpdate == nil {
				log.Errorf("No OnUpdate() callback registered!")
				return
			}
			rcvr.OnUpdate(rcvr.CurrentState)
		}
	}
}
