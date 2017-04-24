package main

import (
	"encoding/json"
	"time"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	log "github.com/Sirupsen/logrus"
	"github.com/armon/go-metrics"
	"github.com/pquerna/ffjson/ffjson"
)

const (
	MAX_PENDING_LENGTH = 100 // Number of messages we can replace into the pending queue
)

type servicesDelegate struct {
	state             *catalog.ServicesState
	pendingBroadcasts [][]byte
	notifications     chan []byte
	Started           bool
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
		notifications:     make(chan []byte, 25),
		Metadata:          NodeMetadata{ClusterName: "default"},
	}

	return &delegate
}

// Start kicks off the goroutine that will process incoming notifications of services
func (d *servicesDelegate) Start() {
	go func() {
		for message := range d.notifications {
			entry := service.Decode(message)
			if entry == nil {
				log.Errorf("NotifyMsg(): error decoding!")
				continue
			}
			d.state.ServiceMsgs <- *entry
		}
	}()

	d.Started = true
}

func (d *servicesDelegate) NodeMeta(limit int) []byte {
	log.Debugf("NodeMeta(): %d", limit)
	data, err := json.Marshal(d.Metadata)
	if err != nil {
		log.Error("Error encoding Node metadata!")
		data = []byte("{}")
	}
	return data
}

func (d *servicesDelegate) NotifyMsg(message []byte) {
	defer metrics.MeasureSince([]string{"delegate", "NotifyMsg"}, time.Now())

	if len(message) < 1 {
		log.Debug("NotifyMsg(): empty")
		return
	}

	log.Debugf("NotifyMsg(): %s", string(message))

	// TODO don't just send container structs, send message structs
	d.notifications <- message
}

func (d *servicesDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	defer metrics.MeasureSince([]string{"delegate", "GetBroadcasts"}, time.Now())
	metrics.SetGauge([]string{"delegate", "pendingBroadcasts"}, float32(len(d.pendingBroadcasts)))

	log.Debugf("GetBroadcasts(): %d %d", overhead, limit)

	var broadcast [][]byte

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
		// We don't want to store old messages forever, or starve ourselves to death
		if len(leftover) > MAX_PENDING_LENGTH {
			d.pendingBroadcasts = leftover[:MAX_PENDING_LENGTH]
		} else {
			d.pendingBroadcasts = leftover
		}
	}

	if broadcast == nil || len(broadcast) < 1 {
		log.Debug("Note: Not enough space to fit any messages or message was nil")
		return nil
	}

	log.Debugf("Sending broadcast %d msgs %d 1st length",
		len(broadcast), len(broadcast[0]),
	)
	if len(leftover) > 0 {
		log.Debugf("Leaving %d messages unsent", len(leftover))
	}

	// TODO testing only!!
	go func(broadcast [][]byte) {
		time.Sleep(5*time.Millisecond)
		for _, entry := range broadcast {
			ffjson.Pool(entry)
		}
	}(broadcast)

	return broadcast
}

func (d *servicesDelegate) LocalState(join bool) []byte {
	log.Debugf("LocalState(): %b", join)
	d.state.RLock()
	defer d.state.RUnlock()
	return d.state.Encode()
}

func (d *servicesDelegate) MergeRemoteState(buf []byte, join bool) {
	defer metrics.MeasureSince([]string{"delegate", "MergeRemoteState"}, time.Now())

	log.Debugf("MergeRemoteState(): %s %t", string(buf), join)

	otherState, err := catalog.Decode(buf)
	if err != nil {
		log.Errorf("Failed to MergeRemoteState(): %s", err.Error())
		return
	}

	log.Debugf("Merging state: %s", otherState.Format(nil))

	d.state.Merge(otherState)
}

func (d *servicesDelegate) NotifyJoin(node *memberlist.Node) {
	log.Debugf("NotifyJoin(): %s %s", node.Name, string(node.Meta))
}

func (d *servicesDelegate) NotifyLeave(node *memberlist.Node) {
	log.Debugf("NotifyLeave(): %s", node.Name)
	go func() {
		// TODO move this to a channel and remove lock
		d.state.Lock()
		defer d.state.Unlock()
		d.state.ExpireServer(node.Name)
	}()
}

func (d *servicesDelegate) NotifyUpdate(node *memberlist.Node) {
	log.Debugf("NotifyUpdate(): %s", node.Name)
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
