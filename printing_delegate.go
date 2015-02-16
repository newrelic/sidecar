package main

import (
	"fmt"
)

type printingDelegate struct {}

func (d *printingDelegate) NodeMeta(limit int) []byte {
	fmt.Printf("NodeMeta(): %d\n", limit)
	return []byte(`{ "State": "Running" }`)
}

func (d *printingDelegate) NotifyMsg(message []byte) {
	fmt.Printf("NotifyMsg(): %s\n", string(message))
}

func (d *printingDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	fmt.Printf("GetBroadcasts(): %d %d\n", overhead, limit)
	return nil
}

func (d *printingDelegate) LocalState(join bool) []byte {
	fmt.Printf("LocalState(): %b\n", join)
	return []byte("Some state")
}

func (d *printingDelegate) MergeRemoteState(buf []byte, join bool) {
	fmt.Printf("MergeRemoteState(): %s %b\n", string(buf), join)
}
