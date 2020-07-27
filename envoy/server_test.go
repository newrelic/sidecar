package envoy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"testing"
	"time"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/config"
	"github.com/Nitro/sidecar/envoy/adapter"
	"github.com/Nitro/sidecar/service"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	envoy_discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc"
)

const (
	bindIP = "192.168.168.168"
)

var (
	validators = map[string]func(*any.Any, service.Service){
		cache.ListenerType: validateListener,
		cache.ClusterType:  validateCluster,
	}
)

func validateListener(serialisedListener *any.Any, svc service.Service) {
	listener := &api.Listener{}
	err := ptypes.UnmarshalAny(serialisedListener, listener)
	So(err, ShouldBeNil)
	So(listener.Name, ShouldEqual, svc.Name)
	So(listener.GetAddress().GetSocketAddress().GetAddress(), ShouldEqual, bindIP)
	So(listener.GetAddress().GetSocketAddress().GetPortValue(), ShouldEqual, svc.Ports[0].ServicePort)
	filterChains := listener.GetFilterChains()
	So(filterChains, ShouldHaveLength, 1)
	filters := filterChains[0].GetFilters()
	So(filters, ShouldHaveLength, 1)

	if svc.ProxyMode == "http" {
		So(filters[0].GetName(), ShouldEqual, wellknown.HTTPConnectionManager)
		// TODO: Switch to ptypes.MarshalAny when updating the Envoy API
		// See the original implementation from 080e510
		//nolint:staticcheck // ignore SA1019 for deprecated code
		connectionManager, err := conversion.MessageToStruct(filters[0].GetConfig())
		So(err, ShouldBeNil)
		So(connectionManager, ShouldNotBeZeroValue)
		So(connectionManager.GetFields()["stat_prefix"].GetStringValue(), ShouldEqual, "ingress_http")
	} else { // tcp
		So(filters[0].GetName(), ShouldEqual, wellknown.TCPProxy)
		// TODO: Switch to ptypes.MarshalAny when updating the Envoy API
		// See the original implementation from 080e510
		//nolint:staticcheck // ignore SA1019 for deprecated code
		connectionManager, err := conversion.MessageToStruct(filters[0].GetConfig())
		So(err, ShouldBeNil)
		So(connectionManager.GetFields()["stat_prefix"].GetStringValue(), ShouldEqual, "ingress_tcp")
		So(connectionManager.GetFields()["cluster"].GetStringValue(), ShouldEqual, adapter.SvcName(svc.Name, svc.Ports[0].ServicePort))
	}
}

func extractClusterEndpoints(serialisedCluster *any.Any, svc service.Service) []*endpoint.LbEndpoint {
	cluster := &api.Cluster{}
	err := ptypes.UnmarshalAny(serialisedCluster, cluster)
	So(err, ShouldBeNil)
	So(cluster.Name, ShouldEqual, adapter.SvcName(svc.Name, svc.Ports[0].ServicePort))
	So(cluster.GetConnectTimeout().GetNanos(), ShouldEqual, 500000000)
	loadAssignment := cluster.GetLoadAssignment()
	So(loadAssignment, ShouldNotBeNil)
	So(loadAssignment.GetClusterName(), ShouldEqual, adapter.SvcName(svc.Name, svc.Ports[0].ServicePort))
	localityEndpoints := loadAssignment.GetEndpoints()
	So(localityEndpoints, ShouldHaveLength, 1)

	return localityEndpoints[0].GetLbEndpoints()
}

func validateCluster(serialisedCluster *any.Any, svc service.Service) {
	endpoints := extractClusterEndpoints(serialisedCluster, svc)
	So(endpoints, ShouldHaveLength, 1)
	So(endpoints[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress(), ShouldEqual, svc.Ports[0].IP)
	So(endpoints[0].GetEndpoint().GetAddress().GetSocketAddress().GetPortValue(), ShouldEqual, svc.Ports[0].Port)
}

// EnvoyMock is used to validate the Envoy state by making the same gRPC stream calls
// to the Server as Envoy would
type EnvoyMock struct {
	nonces map[string]string
}

func NewEnvoyMock() EnvoyMock {
	return EnvoyMock{
		nonces: make(map[string]string),
	}
}

func (sv *EnvoyMock) GetResource(stream envoy_discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, resource string, hostname string) []*any.Any {
	nonce, ok := sv.nonces[resource]
	if !ok {
		// Set the initial nonce to 0 for each resource type. The control plane will increment
		// it after each call, so we need to pass back the value we last received.
		nonce = "0"
	}
	err := stream.Send(&api.DiscoveryRequest{
		VersionInfo: "1",
		Node: &core.Node{
			Id: hostname,
		},
		TypeUrl:       resource,
		ResponseNonce: nonce,
	})
	if err != nil && err != io.EOF {
		So(err, ShouldBeNil)
	}

	// Recv() blocks until the stream ctx expires if the message sent via Send() is not recognised / valid
	response, err := stream.Recv()

	So(err, ShouldBeNil)

	sv.nonces[resource] = response.GetNonce()

	return response.Resources
}

func (sv *EnvoyMock) ValidateResources(stream envoy_discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, svc service.Service, hostname string) {
	for resourceType, validator := range validators {
		resources := sv.GetResource(stream, resourceType, hostname)
		So(resources, ShouldHaveLength, 1)
		validator(resources[0], svc)
	}
}

// SnapshotCache is a light wrapper around cache.SnapshotCache which lets
// us get a notification after calling SetSnapshot via the Waiter chan
type SnapshotCache struct {
	cache.SnapshotCache
	Waiter chan struct{}
}

func (c *SnapshotCache) SetSnapshot(node string, snapshot cache.Snapshot) error {
	err := c.SnapshotCache.SetSnapshot(node, snapshot)

	c.Waiter <- struct{}{}

	return err
}

func NewSnapshotCache() *SnapshotCache {
	return &SnapshotCache{
		SnapshotCache: cache.NewSnapshotCache(true, cache.IDHash{}, nil),
		Waiter:        make(chan struct{}),
	}
}

func Test_PortForServicePort(t *testing.T) {
	Convey("Run()", t, func() {
		config := config.EnvoyConfig{
			UseGRPCAPI: true,
			BindIP:     bindIP,
		}

		state := catalog.NewServicesState()

		dummyHostname := "carcasone"
		baseTime := time.Now().UTC()
		httpSvc := service.Service{
			ID:        "deadbeef123",
			Name:      "bocaccio",
			Created:   baseTime,
			Hostname:  dummyHostname,
			Updated:   baseTime,
			Status:    service.ALIVE,
			ProxyMode: "http",
			Ports: []service.Port{
				{IP: "127.0.0.1", Port: 9990, ServicePort: 10100},
			},
		}

		anotherHTTPSvc := service.Service{
			ID:        "deadbeef456",
			Name:      "bocaccio",
			Created:   baseTime,
			Hostname:  dummyHostname,
			Updated:   baseTime,
			Status:    service.ALIVE,
			ProxyMode: "http",
			Ports: []service.Port{
				{IP: "127.0.0.1", Port: 9991, ServicePort: 10100},
			},
		}

		tcpSvc := service.Service{
			ID:        "undeadbeef",
			Name:      "tolstoy",
			Created:   baseTime,
			Hostname:  state.Hostname,
			Updated:   baseTime,
			Status:    service.ALIVE,
			ProxyMode: "tcp",
			Ports: []service.Port{
				{IP: "127.0.0.1", Port: 666, ServicePort: 10101},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		Reset(func() {
			cancel()
		})

		// Use a custom SnapshotCache in the xdsServer so we can block after updating
		// the state until the Server gets a chance to set a new snapshot in the cache
		snapshotCache := NewSnapshotCache()
		server := &Server{
			config:        config,
			state:         state,
			snapshotCache: snapshotCache,
			xdsServer:     xds.NewServer(ctx, snapshotCache, &xdsCallbacks{}),
		}

		// The gRPC listener will be assigned a random port and will be owned and managed
		// by the gRPC server
		lis, err := net.Listen("tcp", ":0")
		So(err, ShouldBeNil)
		So(lis.Addr(), ShouldHaveSameTypeAs, &net.TCPAddr{})

		// Using a FreeLooper instead would make it run too often, triggering spurious
		// locking on the state, which can cause the tests to time out
		go server.Run(ctx, director.NewTimedLooper(director.FOREVER, 10*time.Millisecond, make(chan error)), lis)

		Convey("sends the Envoy state via gRPC", func() {
			conn, err := grpc.DialContext(ctx,
				fmt.Sprintf(":%d", lis.Addr().(*net.TCPAddr).Port),
				grpc.WithInsecure(), grpc.WithBlock(),
			)
			So(err, ShouldBeNil)

			// 100 milliseconds should give us enough time to run hundreds of server transactions
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			Reset(func() {
				cancel()
			})

			stream, err := envoy_discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
			So(err, ShouldBeNil)

			envoyMock := NewEnvoyMock()

			Convey("for a HTTP service", func() {
				state.AddServiceEntry(httpSvc)
				<-snapshotCache.Waiter

				envoyMock.ValidateResources(stream, httpSvc, state.Hostname)

				Convey("and removes it after it gets tombstoned", func() {
					httpSvc.Tombstone()
					httpSvc.Updated.Add(1 * time.Millisecond)
					state.AddServiceEntry(httpSvc)
					<-snapshotCache.Waiter

					for resourceType := range validators {
						resources := envoyMock.GetResource(stream, resourceType, state.Hostname)
						So(resources, ShouldHaveLength, 0)
					}
				})

				Convey("and places another instance of the same service in the same cluster", func() {
					// Make sure this other service instance was more recently updated than httpSvc
					anotherHTTPSvc.Updated = anotherHTTPSvc.Updated.Add(1 * time.Millisecond)
					state.AddServiceEntry(anotherHTTPSvc)
					<-snapshotCache.Waiter

					resources := envoyMock.GetResource(stream, cache.ClusterType, state.Hostname)
					So(resources, ShouldHaveLength, 1)
					endpoints := extractClusterEndpoints(resources[0], httpSvc)
					So(endpoints, ShouldHaveLength, 2)
					var ports sort.IntSlice
					for _, endpoint := range endpoints {
						ports = append(ports,
							int(endpoint.GetEndpoint().GetAddress().GetSocketAddress().GetPortValue()))
					}
					ports.Sort()
					So(ports, ShouldResemble, sort.IntSlice{9990, 9991})
				})
			})

			Convey("for a TCP service", func() {
				state.AddServiceEntry(tcpSvc)
				<-snapshotCache.Waiter

				envoyMock.ValidateResources(stream, tcpSvc, state.Hostname)
			})

			Convey("and skips tombstones", func() {
				httpSvc.Tombstone()
				state.AddServiceEntry(httpSvc)
				<-snapshotCache.Waiter

				for resourceType := range validators {
					resources := envoyMock.GetResource(stream, resourceType, state.Hostname)
					So(resources, ShouldHaveLength, 0)
				}
			})

			Convey("and triggers an update when expiring a server with only one service running", func(c C) {
				state.AddServiceEntry(httpSvc)
				<-snapshotCache.Waiter

				done := make(chan struct{})
				go func() {
					select {
					case <-snapshotCache.Waiter:
						close(done)
					case <-time.After(100 * time.Millisecond):
						c.So(true, ShouldEqual, false)
					}
				}()

				state.ExpireServer(dummyHostname)
				<-done

				for resourceType := range validators {
					resources := envoyMock.GetResource(stream, resourceType, state.Hostname)
					So(resources, ShouldHaveLength, 0)
				}
			})
		})
	})
}
