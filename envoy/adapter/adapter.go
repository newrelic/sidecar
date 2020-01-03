package adapter

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	tcpp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	log "github.com/sirupsen/logrus"
)

const (
	// ServiceNameSeparator is used to join service name and port. Must not occur in service names.
	ServiceNameSeparator = ":"
)

// SvcName formats an Envoy service name from our service name and port
func SvcName(name string, port int64) string {
	return fmt.Sprintf("%s%s%d", name, ServiceNameSeparator, port)
}

// SvcNameSplit an Enovy service name into our service name and port
func SvcNameSplit(name string) (string, int64, error) {
	parts := strings.Split(name, ServiceNameSeparator)
	if len(parts) < 2 {
		return "", -1, fmt.Errorf("%s", "Unable to split service name and port!")
	}

	svcName := parts[0]
	svcPort, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", -1, fmt.Errorf("%s", "Unable to parse port!")
	}

	return svcName, svcPort, nil
}

// LookupHost does a vv slow lookup of the DNS host for a service. Totally
// not optimized for high throughput. You should only do this in development
// scenarios.
func LookupHost(hostname string) (string, error) {
	addrs, err := net.LookupHost(hostname)

	if err != nil {
		return "", err
	}
	return addrs[0], nil
}

// EnvoyListenersFromState creates a set of Enovy API listener
// definitions from all the ServicePorts in the Sidecar state.
func EnvoyListenersFromState(state *catalog.ServicesState, bindIP string) ([]cache.Resource, error) {
	var listeners []cache.Resource

	state.RLock()
	defer state.RUnlock()

	svcs := state.ByService()
	// Loop over all the services by service name
	for _, endpoints := range svcs {
		if len(endpoints) < 1 {
			continue
		}

		var svc *service.Service
		// Find the first alive service and use that as the definition.
		// If none are alive, we won't open the port.
		for _, endpoint := range endpoints {
			if endpoint.IsAlive() {
				svc = endpoint
				break
			}
		}

		if svc == nil {
			continue
		}

		// Loop over the ports and generate a named listener for
		// each port.
		for _, port := range svc.Ports {
			// Only listen on ServicePorts
			if port.ServicePort < 1 {
				continue
			}

			listener, err := EnvoyListenerFromService(svc, port.ServicePort, bindIP)
			if err != nil {
				return nil, fmt.Errorf("failed to create listener from service: %s", err)
			}
			listeners = append(listeners, listener)
		}
	}

	log.Debugf("Created %d Envoy listeners", len(listeners))

	return listeners, nil
}

// EnvoyListenerFromService creates an Envoy listener from a service instance
func EnvoyListenerFromService(svc *service.Service, port int64, bindIP string) (cache.Resource, error) {
	apiName := SvcName(svc.Name, port)

	var connectionManagerName string
	var connectionManager proto.Message
	switch svc.ProxyMode {
	case "http":
		connectionManagerName = wellknown.HTTPConnectionManager

		connectionManager = &hcm.HttpConnectionManager{
			StatPrefix: "ingress_http",
			HttpFilters: []*hcm.HttpFilter{{
				Name: wellknown.Router,
			}},
			RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
				RouteConfig: &api.RouteConfiguration{
					VirtualHosts: []*route.VirtualHost{{
						Name:    apiName,
						Domains: []string{"*"},
						Routes: []*route.Route{{
							Match: &route.RouteMatch{
								PathSpecifier: &route.RouteMatch_Prefix{
									Prefix: "/",
								},
							},
							Action: &route.Route_Route{
								Route: &route.RouteAction{
									ClusterSpecifier: &route.RouteAction_Cluster{
										Cluster: apiName,
									},
									Timeout: &duration.Duration{},
								},
							},
						}},
					}},
				},
			},
		}
	case "tcp":
		connectionManagerName = wellknown.TCPProxy

		connectionManager = &tcpp.TcpProxy{
			StatPrefix: "ingress_tcp",
			ClusterSpecifier: &tcpp.TcpProxy_Cluster{
				Cluster: apiName,
			},
		}
	default:
		return nil, fmt.Errorf("unrecognised proxy mode: %s", svc.ProxyMode)
	}

	serialisedConnectionManager, err := ptypes.MarshalAny(connectionManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create the connection manager: %s", err)
	}

	return &api.Listener{
		Name: svc.Name,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Address: bindIP,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: connectionManagerName,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: serialisedConnectionManager,
				},
			}},
		}},
	}, nil
}

// EnvoyClustersFromState genenerates a list of Envoy clusters from the
// current Sidecar state
func EnvoyClustersFromState(state *catalog.ServicesState, useHostnames bool) []cache.Resource {
	state.RLock()
	defer state.RUnlock()

	// `s.state.ByService()` returns the list of service endpoints for each service.
	// Since some services can expose multiple service ports, we need to create a
	// separate cluster for each (service, servicePort) pair. If a service doesn't
	// have any endpoints that are alive, we don't want to create a cluster for it.
	//
	// Note that in `EnvoyClustersFromState()` for the REST API we only need
	// the first non-nil alive endpoint instance to construct the cluster
	// because, in that case, SDS (now EDS) fetches the actual endpoints in a
	// separate call.
	var clusters []cache.Resource
	clustersMap := make(map[string]*api.Cluster)
	for svcName, svcEndpoints := range state.ByService() {
		if len(svcEndpoints) < 1 {
			continue
		}

		for _, svcEndpoint := range svcEndpoints {
			if svcEndpoint == nil || !svcEndpoint.IsAlive() {
				continue
			}

			for _, port := range svcEndpoint.Ports {
				if port.ServicePort < 1 {
					continue
				}

				envoyServiceName := SvcName(svcName, port.ServicePort)

				if cluster, ok := clustersMap[envoyServiceName]; ok {
					cluster.LoadAssignment.Endpoints[0].LbEndpoints =
						append(cluster.LoadAssignment.Endpoints[0].LbEndpoints,
							envoyServiceFromService(svcEndpoint, port.ServicePort, useHostnames)...)
				} else {
					envoyCluster := &api.Cluster{
						Name:                 envoyServiceName,
						ConnectTimeout:       &duration.Duration{Nanos: 500000000},        // 500ms
						ClusterDiscoveryType: &api.Cluster_Type{Type: api.Cluster_STATIC}, // Use IPs only
						ProtocolSelection:    api.Cluster_USE_CONFIGURED_PROTOCOL,
						// Setting the endpoints here directly bypasses EDS, so we can
						// avoid having to configure that as well
						LoadAssignment: &api.ClusterLoadAssignment{
							ClusterName: envoyServiceName,
							Endpoints: []*endpoint.LocalityLbEndpoints{{
								LbEndpoints: envoyServiceFromService(svcEndpoint, port.ServicePort, useHostnames),
							}},
						},
						// Contour believes the IdleTimeout should be set to 60s. Not sure if we also need to enable these.
						// See here: https://github.com/projectcontour/contour/blob/2858fec20d26f56cc75a19d91b61d625a86f36de/internal/envoy/listener.go#L102-L106
						// CommonHttpProtocolOptions: &core.HttpProtocolOptions{
						// 	IdleTimeout:           &duration.Duration{Seconds: 60},
						// 	MaxConnectionDuration: &duration.Duration{Seconds: 60},
						// },
						// If this needs to be enabled, we might also need to set `ProtocolSelection: api.USE_DOWNSTREAM_PROTOCOL`.
						// Http2ProtocolOptions: &core.Http2ProtocolOptions{},
					}

					clustersMap[envoyServiceName] = envoyCluster
					clusters = append(clusters, envoyCluster)
				}
			}
		}
	}

	log.Debugf("Created %d Envoy clusters", len(clusters))

	return clusters
}

// envoyServiceFromService converts a Sidecar service to an Envoy
// API service for reporting to the proxy
func envoyServiceFromService(svc *service.Service, svcPort int64, useHostnames bool) []*endpoint.LbEndpoint {
	var endpoints []*endpoint.LbEndpoint
	for _, port := range svc.Ports {
		// No sense worrying about unexposed ports
		if port.ServicePort == svcPort {
			address := port.IP

			// NOT recommended... this is very slow. Useful in dev modes where you
			// need to resolve to a different IP address only.
			if useHostnames {
				if host, err := LookupHost(svc.Hostname); err == nil {
					address = host
				} else {
					log.Warnf("Unable to resolve %s, using IP address", svc.Hostname)
				}
			}

			endpoints = append(endpoints, &endpoint.LbEndpoint{
				HostIdentifier: &endpoint.LbEndpoint_Endpoint{
					Endpoint: &endpoint.Endpoint{
						Address: &core.Address{
							Address: &core.Address_SocketAddress{
								SocketAddress: &core.SocketAddress{
									Address: address,
									PortSpecifier: &core.SocketAddress_PortValue{
										PortValue: uint32(port.Port),
									},
								},
							},
						},
					},
				},
			})
		}
	}

	return endpoints
}
