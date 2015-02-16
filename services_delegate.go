package main

import (
	"log"
)

type servicesDelegate struct {
	state *ServicesState
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
	data := Decode(message)
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
			println("Sending broadcast")
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
}
