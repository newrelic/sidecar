package receiver

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/NinesStack/sidecar/catalog"
	log "github.com/sirupsen/logrus"
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
		_, err := response.Write(message)
		if err != nil {
			log.Errorf("Error replying to client when failed to read the request body: %s", err)
		}
		return
	}

	var evt catalog.StateChangedEvent
	err = json.Unmarshal(data, &evt)
	if err != nil {
		message, _ := json.Marshal(ApiErrors{[]string{err.Error()}})
		response.WriteHeader(http.StatusInternalServerError)
		_, err := response.Write(message)
		if err != nil {
			log.Errorf("Error replying to client when failed to unmarshal the request JSON: %s", err)
		}
		return
	}

	rcvr.StateLock.Lock()
	defer rcvr.StateLock.Unlock()

	if rcvr.CurrentState == nil || rcvr.CurrentState.LastChanged.Before(evt.State.LastChanged) {
		rcvr.CurrentState = evt.State
		rcvr.LastSvcChanged = &evt.ChangeEvent.Service

		if ShouldNotify(evt.ChangeEvent.PreviousStatus, evt.ChangeEvent.Service.Status) {
			if !rcvr.IsSubscribed(evt.ChangeEvent.Service.Name) {
				return
			}

			if rcvr.OnUpdate == nil {
				log.Errorf("No OnUpdate() callback registered!")
				return
			}
			rcvr.EnqueueUpdate()
		}
	}
}
