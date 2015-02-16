package main

import (
	"encoding/json"
	"fmt"
)

type servicesDelegate struct {
	state *ServicesState
}

func (d *servicesDelegate) NodeMeta(limit int) []byte {
	fmt.Printf("NodeMeta(): %d\n", limit)
	return []byte(`{ "State": "Running" }`)
}

func (d *servicesDelegate) NotifyMsg(message []byte) {
	if len(message) <  1 {
		fmt.Println("NotifyMsg(): empty")
		return
	}

	fmt.Printf("NotifyMsg(): %s\n", string(message))

	// TODO don't just send container structs, send message structs
	data := Decode(message)
	if data == nil {
		fmt.Printf("NotifyMsg(): error decoding!\n")
		return
	}

	d.state.AddServiceEntry(*data)
}

func (d *servicesDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	fmt.Printf("GetBroadcasts(): %d %d\n", overhead, limit)

	select {
		case broadcast := <-broadcasts:
			println("Sending broadcast")
			return broadcast
		default:
			return nil
	}
}

func (d *servicesDelegate) LocalState(join bool) []byte {
	fmt.Printf("LocalState(): %b\n", join)
	jsonData, err := json.Marshal(d.state.Servers)
	if err != nil {
		return []byte{}
	}

	return jsonData
}

func (d *servicesDelegate) MergeRemoteState(buf []byte, join bool) {
	fmt.Printf("MergeRemoteState(): %s %b\n", string(buf), join)
}
