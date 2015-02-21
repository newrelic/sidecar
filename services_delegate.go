package main

import (
	"fmt"
	"log"

	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/services_state"
)

type servicesDelegate struct {
	state *services_state.ServicesState
}

func (d *servicesDelegate) NodeMeta(limit int) []byte {
	log.Printf("NodeMeta(): %d\n", limit)
	return []byte(`{ "State": "Running" }`)
}

func (d *servicesDelegate) NotifyMsg(message []byte) {
	if len(message) <  1 {
		log.Println("NotifyMsg(): empty")
		return
	}

	log.Printf("NotifyMsg(): %s\n", string(message))

	// TODO don't just send container structs, send message structs
	data := service.Decode(message)
	if data == nil {
		log.Printf("NotifyMsg(): error decoding!\n")
		return
	}

	d.state.AddServiceEntry(*data)
}

func (d *servicesDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	log.Printf("GetBroadcasts(): %d %d\n", overhead, limit)

	select {
		case broadcast := <-broadcasts:
			if len(broadcast) < 1 {
				println("Got empty broadcast")
				return nil
			}
			fmt.Printf("Sending broadcast %d msgs %d 1st length\n", len(broadcast), len(broadcast[0]))
			return broadcast
		default:
			return nil
	}
}

func (d *servicesDelegate) LocalState(join bool) []byte {
	log.Printf("LocalState(): %b\n", join)
	return d.state.Encode()
}

func (d *servicesDelegate) MergeRemoteState(buf []byte, join bool) {
	log.Printf("MergeRemoteState(): %s %b\n", string(buf), join)

	otherState, err := services_state.Decode(buf)
	if err != nil {
		log.Printf("Failed to MergeRemoteState(): %s", err.Error())
		return
	}

	log.Printf("Merging state: %s", otherState.Format(nil))

	d.state.Merge(otherState)
}
