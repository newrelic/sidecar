package main

import (
	"encoding/json"
	"time"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	"github.com/armon/go-metrics"
	"github.com/pquerna/ffjson/ffjson"
	log "github.com/sirupsen/logrus"
)

const (
	MAX_PENDING_LENGTH = 100 // Number of messages we can replace into the pending queue
)

type servicesDelegate struct {
	state             *catalog.ServicesState
	pendingBroadcasts [][]byte
	notifications     chan []byte
	Started           bool
	StartedAt         time.Time
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
			entry, err := service.Decode(message)
			if err != nil {
				log.Errorf("Start(): error decoding message: %s", err)
				continue
			}
			d.state.UpdateService(*entry)
		}
	}()

	d.Started = true
	d.StartedAt = time.Now().UTC()
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

	// Prefer newest messages (TODO what about tombstones?). We use the one new
	// broadcast and then append all the pending ones to see if we can get
	// them into the packet.
	if len(d.pendingBroadcasts) > 0 {
		broadcast = append(broadcast, d.pendingBroadcasts...)
	}
	broadcast, leftover := d.packPacket(broadcast, limit, overhead)

	if len(leftover) > 0 {
		// We don't want to store old messages forever, or starve ourselves to death
		if len(leftover) > MAX_PENDING_LENGTH {
			d.pendingBroadcasts = leftover[:MAX_PENDING_LENGTH]
		} else {
			d.pendingBroadcasts = leftover
		}

		log.Debugf("Leaving %d messages unsent", len(leftover))
	} else {
		d.pendingBroadcasts = [][]byte{}
	}

	if broadcast == nil || len(broadcast) < 1 {
		log.Debug("Note: Not enough space to fit any messages or message was nil")
		return nil
	}

	log.Debugf("Sending broadcast %d msgs %d 1st length",
		len(broadcast), len(broadcast[0]),
	)

	// Unfortunately Memberlist does not provide a callback after broadcasts were
	// accepted so we have no direct way to return these to the pool. However, it
	// immediately copies what we return into a new buffer. So, it's not perfectly,
	// but is reasonably safe to wait awhile and then re-add our buffer to the
	// ffjson pool.
	go func(broadcast [][]byte) {
		time.Sleep(25 * time.Millisecond) // Lots of safety margin in this number
		for i := 0; i < len(broadcast); i++ {
			ffjson.Pool(broadcast[i])
		}
	}(broadcast)

	return broadcast
}

func (d *servicesDelegate) LocalState(join bool) []byte {
	log.Debugf("LocalState(): %t", join)
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
		d.state.Lock()
		defer d.state.Unlock()
		d.state.ExpireServer(node.Name)
	}()
}

func (d *servicesDelegate) NotifyUpdate(node *memberlist.Node) {
	log.Debugf("NotifyUpdate(): %s", node.Name)
}

// Try to pack as many messages into the packet as we can. Note that this
// assumes that no messages will be longer than the normal UDP packet size.
// This means that max message length is somewhere around 1398 when taking
// messaging overhead into account.
func (d *servicesDelegate) packPacket(broadcasts [][]byte, limit int, overhead int) (packet [][]byte, leftover [][]byte) {
	total := 0
	lastItem := -1

	// Find the index of the last item that fits into the packet we're building
	for i, message := range broadcasts {
		if total+len(message)+overhead > limit {
			break
		}

		lastItem = i
		total += len(message) + overhead
	}

	if lastItem < 0 && len(broadcasts) > 0 {
		// Don't warn on startup... it's fairly normal
		gracePeriod := time.Now().UTC().Add(0 - (5 * time.Second))
		if d.StartedAt.Before(gracePeriod) {
			log.Warnf("All messages were too long to fit! No broadcasts!")
		}

		// There could be a scenario here where one hugely long broadcast could
		// get stuck forever and prevent anything else from going out. There
		// may be a better way to handle this. Scanning for the next message that
		// does fit results in lots of memory copying and doesn't perform at scale.
		return nil, broadcasts
	}

	// Save the leftover messages after the last one that fit. If this is too
	// much, then set it to the lastItem.
	firstLeftover := lastItem + 1

	return broadcasts[:lastItem+1], broadcasts[firstLeftover:]
}
