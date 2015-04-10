package main

import (
	"encoding/json"
	"log"

	"github.com/hashicorp/memberlist"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
)

type servicesDelegate struct {
	state             *catalog.ServicesState
	pendingBroadcasts [][]byte
	notifications     chan *service.Service
	inProcess         bool
	Metadata          NodeMetadata
}

type NodeMetadata struct {
	ClusterName string
	State       string
}

func NewServicesDelegate(state *catalog.ServicesState) *servicesDelegate {
	delegate := servicesDelegate{
		state:             state,
		pendingBroadcasts: make([][]byte, 0),
		notifications:     make(chan *service.Service, 25),
		inProcess:         false,
		Metadata:          NodeMetadata{ClusterName: "default"},
	}

	return &delegate
}

func (d *servicesDelegate) NodeMeta(limit int) []byte {
	log.Printf("NodeMeta(): %d\n", limit)
	data, err := json.Marshal(d.Metadata)
	if err != nil {
		log.Println("Error encoding Node metadata!")
		data = []byte("{}")
	}
	return data
}

func (d *servicesDelegate) NotifyMsg(message []byte) {
	if len(message) < 1 {
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

	// Lazily kick off goroutine
	if !d.inProcess {
		go func() {
			for entry := range d.notifications {
				d.state.AddServiceEntry(*entry)
			}
		}()
		d.inProcess = true
	}
	d.notifications <- data
}

func (d *servicesDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	log.Printf("GetBroadcasts(): %d %d\n", overhead, limit)

	broadcast := make([][]byte, 0, 1)

	select {
	case broadcast = <-d.state.Broadcasts:
	default:
		if len(d.pendingBroadcasts) < 1 {
			return nil
		}
	}

	// Prefer newest messages (TODO what about tombstones?)
	broadcast = append(broadcast, d.pendingBroadcasts...)
	d.pendingBroadcasts = make([][]byte, 0, 1)

	broadcast, leftover := packPacket(broadcast, limit, overhead)
	if len(leftover) > 0 {
		d.pendingBroadcasts = leftover
	}

	if broadcast == nil || len(broadcast) < 1 {
		log.Println("Note: Not enough space to fit any messages or message was nil")
		return nil
	}

	log.Printf("Sending broadcast %d msgs %d 1st length\n",
		len(broadcast), len(broadcast[0]),
	)
	if len(leftover) > 0 {
		log.Printf("Leaving %d messages unsent\n", len(leftover))
	}

	return broadcast
}

func (d *servicesDelegate) LocalState(join bool) []byte {
	log.Printf("LocalState(): %b\n", join)
	return d.state.Encode()
}

func (d *servicesDelegate) MergeRemoteState(buf []byte, join bool) {
	log.Printf("MergeRemoteState(): %s %b\n", string(buf), join)

	otherState, err := catalog.Decode(buf)
	if err != nil {
		log.Printf("Failed to MergeRemoteState(): %s", err.Error())
		return
	}

	log.Printf("Merging state: %s", otherState.Format(nil))

	d.state.Merge(otherState)
}

func (d *servicesDelegate) NotifyJoin(node *memberlist.Node) {
	log.Printf("NotifyJoin(): %s %s\n", node.Name, string(node.Meta))
}

func (d *servicesDelegate) NotifyLeave(node *memberlist.Node) {
	log.Printf("NotifyLeave(): %s\n", node.Name)
	go d.state.ExpireServer(node.Name)
}

func (d *servicesDelegate) NotifyUpdate(node *memberlist.Node) {
	log.Printf("NotifyUpdate(): %s\n", node.Name)
}

func packPacket(broadcasts [][]byte, limit int, overhead int) (packet [][]byte, leftover [][]byte) {
	total := 0
	leftover = make([][]byte, 0) // So we don't return unallocated buffer
	for _, message := range broadcasts {
		if total+len(message)+overhead < limit {
			packet = append(packet, message)
			total += len(message) + overhead
		} else {
			leftover = append(leftover, message)
		}
	}

	return packet, leftover
}
