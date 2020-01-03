package envoy

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/config"
	"github.com/Nitro/sidecar/envoy/adapter"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type xdsCallbacks struct{}

func (*xdsCallbacks) OnStreamOpen(context.Context, int64, string) error  { return nil }
func (*xdsCallbacks) OnStreamClosed(int64)                               {}
func (*xdsCallbacks) OnStreamRequest(int64, *api.DiscoveryRequest) error { return nil }
func (*xdsCallbacks) OnStreamResponse(_ int64, req *api.DiscoveryRequest, _ *api.DiscoveryResponse) {
	if req.GetErrorDetail().GetCode() != 0 {
		log.Errorf("Received Envoy error code %d: %s",
			req.GetErrorDetail().GetCode(),
			strings.ReplaceAll(req.GetErrorDetail().GetMessage(), "\n", ""),
		)
	}
}
func (*xdsCallbacks) OnFetchRequest(context.Context, *api.DiscoveryRequest) error   { return nil }
func (*xdsCallbacks) OnFetchResponse(*api.DiscoveryRequest, *api.DiscoveryResponse) {}

// Server is a wrapper around Envoy's control plane xDS gRPC server and it uses
// the Aggregated Discovery Service (ADS) mechanism.
type Server struct {
	Listener      catalog.Listener
	config        config.EnvoyConfig
	state         *catalog.ServicesState
	snapshotCache cache.SnapshotCache
	xdsServer     xds.Server
}

// Run sets up the Sidecar listener event loop and starts the Envoy gRPC server
func (s *Server) Run(ctx context.Context, looper director.Looper, grpcListener net.Listener) {
	grpcServer := grpc.NewServer()
	envoy_discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, s.xdsServer)

	go looper.Loop(func() error {
		// Block until we get an event indicating a state change.
		// We discard the event since we need a snapshot of the entire state.
		<-s.Listener.Chan()

		// When a server is expired in catalog/services_state.go -> ExpireServer(),
		// the listener will receive an event for each expired service. We want to
		// flush the channel to prevent rapid-fire updates to Envoy.
		// This was inspired from receiver/receiver.go -> ProcessUpdates().
		// TODO: Think of a more aggressive / reliable way of draining since we
		// used a larger value for listenerEventBufferSize.
		pendingEventCount := len(s.Listener.Chan())
		for i := 0; i < pendingEventCount; i++ {
			<-s.Listener.Chan()
		}

		listeners, err := adapter.EnvoyListenersFromState(s.state, s.config.BindIP)
		if err != nil {
			log.Errorf("Failed to create Envoy listeners: %s", err)
			return nil
		}

		// We are using `time.Now().UnixNano()` to ensure that all versions we send to
		// Envoy are unique. Otherwise, Envoy will skip the update.
		version := strconv.FormatInt(time.Now().UnixNano(), 10)
		err = s.snapshotCache.SetSnapshot(
			// The hostname needs to match the value passed via `--service-node` to Envoy
			// See https://github.com/envoyproxy/envoy/issues/144#issuecomment-267401271
			s.state.Hostname,
			cache.NewSnapshot(
				version,
				nil,
				adapter.EnvoyClustersFromState(s.state, s.config.UseHostnames),
				nil,
				listeners,
				nil,
			),
		)
		if err != nil {
			log.Errorf("Failed to create new Envoy cache snapshot: %s", err)
			return nil
		}

		log.Infof("Envoy configuration updated to version %s", version)

		return nil
	})

	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("Failed to start Envoy gRPC server: %s", err)
		}
	}()

	// Currently, this will block forever
	<-ctx.Done()
	grpcServer.GracefulStop()
}

// NewServer creates a new Server instance
func NewServer(ctx context.Context, state *catalog.ServicesState, config config.EnvoyConfig) *Server {
	// Instruct the snapshot cache to use Aggregated Discovery Service (ADS)
	// The third parameter can contain a logger instance, but I didn't find
	// those logs particularly useful.
	snapshotCache := cache.NewSnapshotCache(true, cache.IDHash{}, nil)

	return &Server{
		Listener:      NewListener(),
		config:        config,
		state:         state,
		snapshotCache: snapshotCache,
		xdsServer:     xds.NewServer(ctx, snapshotCache, &xdsCallbacks{}),
	}
}
