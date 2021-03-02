package sidecarhttp

//go:generate ffjson $GOFILE

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/envoy/adapter"
	"github.com/Nitro/sidecar/service"
	"github.com/gorilla/mux"
	"github.com/pquerna/ffjson/ffjson"
	log "github.com/sirupsen/logrus"
)

// This file implements the Envoy proxy V1 API on top of a Sidecar
// service discovery cluster.

// Envoy API definitions --------------------------------------------------

// See https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/sds.html
type EnvoyService struct {
	IPAddress       string            `json:"ip_address"`
	LastCheckIn     string            `json:"last_check_in"`
	Port            int64             `json:"port"`
	Revision        string            `json:"revision"`
	Service         string            `json:"service"`
	ServiceRepoName string            `json:"service_repo_name"`
	Tags            map[string]string `json:"tags"`
}

// See https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster.html
type EnvoyCluster struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	ConnectTimeoutMs int64  `json:"connect_timeout_ms"`
	LBType           string `json:"lb_type"`
	ServiceName      string `json:"service_name"`
	// Many optional fields omitted
}

// https://www.envoyproxy.io/docs/envoy/latest/api-v1/listeners/listeners.html
type EnvoyListener struct {
	Name    string         `json:"name"`
	Address string         `json:"address"`
	Filters []*EnvoyFilter `json:"filters"` // TODO support filters?
	// Many optional fields omitted
}

// A basic Envoy Route Filter
type EnvoyFilter struct {
	Name   string             `json:"name"`
	Config *EnvoyFilterConfig `json:"config"`
}

type EnvoyFilterConfig struct {
	CodecType   string            `json:"codec_type,omitempty"`
	StatPrefix  string            `json:"stat_prefix,omitempty"`
	RouteConfig *EnvoyRouteConfig `json:"route_config,omitempty"`
	Filters     []*EnvoyFilter    `json:"filters,omitempty"`
}

type EnvoyHTTPVirtualHost struct {
	Name    string        `json:"name"`
	Domains []string      `json:"domains"`
	Routes  []*EnvoyRoute `json:"routes"`
}

type EnvoyRouteConfig struct {
	VirtualHosts []*EnvoyHTTPVirtualHost `json:"virtual_hosts,omitempty"` // Used for HTTP
	Routes       []*EnvoyTCPRoute        `json:"routes,omitempty"`        // Use for TCP
}

type EnvoyRoute struct {
	TimeoutMs   int    `json:"timeout_ms"`
	Prefix      string `json:"prefix"`
	HostRewrite string `json:"host_rewrite"`
	Cluster     string `json:"cluster"`
}

type EnvoyTCPRoute struct {
	Cluster           string   `json:"cluster"`
	DestinationIPList []string `json:"destination_ip_list,omitempty"`
	DestinationPorts  string   `json:"destination_ports,omitempty"`
	SourceIPList      []string `json:"source_ip_list,omitempty"`
	SourcePorts       []string `json:"source_ports,omitempty"`
}

// ------------------------------------------------------------------------

type EnvoyApi struct {
	list   *memberlist.Memberlist
	state  *catalog.ServicesState
	config *HttpConfig
}

// optionsHandler sends CORS headers
func (s *EnvoyApi) optionsHandler(response http.ResponseWriter, req *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")
}

type SDSResult struct {
	Env     string          `json:"env"`
	Hosts   []*EnvoyService `json:"hosts"`
	Service string          `json:"service"`
}

// registrationHandler takes the name of a single service and returns results for just
// that service. It implements the Envoy SDS API V1.
func (s *EnvoyApi) registrationHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	name, ok := params["service"]
	if !ok {
		log.Debug("No service name provided to Envoy registrationHandler")
		sendJsonError(response, 404, "Not Found - No service name provided")
		return
	}

	svcName, svcPort, err := adapter.SvcNameSplit(name)
	if err != nil {
		log.Debugf("Envoy Service '%s' not found in registrationHandler: %s", name, err)
		sendJsonError(response, 404, "Not Found - "+err.Error())
		return
	}

	instances := make([]*EnvoyService, 0)
	// Enter critical section
	func() {
		s.state.RLock()
		defer s.state.RUnlock()
		s.state.EachService(func(hostname *string, id *string, svc *service.Service) {
			if svc.Name == svcName && svc.IsAlive() {
				newInstance := s.EnvoyServiceFromService(svc, svcPort)
				if newInstance != nil {
					instances = append(instances, newInstance)
				}
			}
		})
	}()

	clusterName := ""
	if s.list != nil {
		clusterName = s.list.ClusterName()
	}

	result := SDSResult{
		Env:     clusterName,
		Hosts:   instances,
		Service: name,
	}

	jsonBytes, err := result.MarshalJSON()
	defer ffjson.Pool(jsonBytes)
	if err != nil {
		log.Errorf("Error marshaling state in registrationHandler: %s", err)
		sendJsonError(response, 500, "Internal server error")
		return
	}

	_, err = response.Write(jsonBytes)
	if err != nil {
		log.Errorf("Error writing registration response to client: %s", err)
	}
}

type CDSResult struct {
	Clusters []*EnvoyCluster `json:"clusters"`
}

// clustersHandler returns cluster information for all Sidecar services. It
// implements the Envoy CDS API V1.
func (s *EnvoyApi) clustersHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	clusters := s.EnvoyClustersFromState()

	log.Debugf("Reporting Envoy cluster information for cluster '%s' and node '%s'",
		params["service_cluster"], params["service_node"])

	result := CDSResult{clusters}

	jsonBytes, err := result.MarshalJSON()
	defer ffjson.Pool(jsonBytes)
	if err != nil {
		log.Errorf("Error marshaling state in servicesHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	_, err = response.Write(jsonBytes)
	if err != nil {
		log.Errorf("Error writing clusters response to client: %s", err)
	}
}

type LDSResult struct {
	Listeners []*EnvoyListener `json:"listeners"`
}

// listenersHandler returns a list of listeners for all ServicePorts. It
// implements the Envoy LDS API V1.
func (s *EnvoyApi) listenersHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	log.Debugf("Reporting Envoy cluster information for cluster '%s' and node '%s'",
		params["service_cluster"], params["service_node"])

	listeners := s.EnvoyListenersFromState()

	result := LDSResult{listeners}
	jsonBytes, err := result.MarshalJSON()
	defer ffjson.Pool(jsonBytes)
	if err != nil {
		log.Errorf("Error marshaling state in servicesHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	_, err = response.Write(jsonBytes)
	if err != nil {
		log.Errorf("Error writing listeners response to client: %s", err)
	}
}

// EnvoyServiceFromService converts a Sidecar service to an Envoy
// API service for reporting to the proxy
func (s *EnvoyApi) EnvoyServiceFromService(svc *service.Service, svcPort int64) *EnvoyService {
	if len(svc.Ports) < 1 {
		return nil
	}

	for _, port := range svc.Ports {
		// No sense worrying about unexposed ports
		if port.ServicePort == svcPort {
			address := port.IP

			// NOT recommended... this is very slow. Useful in dev modes where you
			// need to resolve to a different IP address only.
			if s.config.UseHostnames {
				if host, err := adapter.LookupHost(svc.Hostname); err == nil {
					address = host
				} else {
					log.Warnf("Unable to resolve %s, using IP address", svc.Hostname)
				}
			}

			return &EnvoyService{
				IPAddress:       address,
				LastCheckIn:     svc.Updated.String(),
				Port:            port.Port,
				Revision:        svc.Version(),
				Service:         adapter.SvcName(svc.Name, port.ServicePort),
				ServiceRepoName: svc.Image,
				Tags:            map[string]string{},
			}
		}
	}

	return nil
}

// EnvoyClustersFromState genenerates a set of Envoy API cluster
// definitions from Sidecar state
func (s *EnvoyApi) EnvoyClustersFromState() []*EnvoyCluster {
	clusters := make([]*EnvoyCluster, 0)

	s.state.RLock()
	defer s.state.RUnlock()

	svcs := s.state.ByService()
	for svcName, endpoints := range svcs {
		if len(endpoints) < 1 {
			continue
		}

		var svc *service.Service
		for _, endpoint := range endpoints {
			if endpoint.IsAlive() {
				svc = endpoint
				break
			}
		}

		if svc == nil {
			continue
		}

		for _, port := range svc.Ports {
			if port.ServicePort < 1 {
				continue
			}

			clusters = append(clusters, &EnvoyCluster{
				Name:             adapter.SvcName(svcName, port.ServicePort),
				Type:             "sds", // use Sidecar's SDS endpoint for the hosts
				ConnectTimeoutMs: 500,
				LBType:           "round_robin", // TODO figure this out!
				ServiceName:      adapter.SvcName(svcName, port.ServicePort),
			})
		}
	}

	return clusters
}

// EnvoyListenerFromService takes a Sidecar service and formats it into
// the API format for an Envoy proxy listener (LDS API v1)
func (s *EnvoyApi) EnvoyListenerFromService(svc *service.Service, port int64) *EnvoyListener {
	apiName := adapter.SvcName(svc.Name, port)

	listener := &EnvoyListener{
		Name:    apiName,
		Address: fmt.Sprintf("tcp://%s:%d", s.config.BindIP, port),
	}

	if svc.ProxyMode == "http" {
		listener.Filters = []*EnvoyFilter{
			{
				Name: "envoy.http_connection_manager",
				Config: &EnvoyFilterConfig{
					CodecType:  "auto",
					StatPrefix: "ingress_http",
					Filters: []*EnvoyFilter{
						{
							Name:   "router",
							Config: &EnvoyFilterConfig{},
						},
					},
					RouteConfig: &EnvoyRouteConfig{
						VirtualHosts: []*EnvoyHTTPVirtualHost{
							{
								Name:    svc.Name,
								Domains: []string{"*"},
								Routes: []*EnvoyRoute{
									{
										TimeoutMs: 0, // No timeout!
										Prefix:    "/",
										Cluster:   apiName,
									},
								},
							},
						},
					},
				},
			},
		}
	} else { // == "tcp"
		listener.Filters = []*EnvoyFilter{
			{
				Name: "envoy.tcp_proxy",
				Config: &EnvoyFilterConfig{
					StatPrefix: "ingress_tcp",
					RouteConfig: &EnvoyRouteConfig{
						Routes: []*EnvoyTCPRoute{
							{
								Cluster: apiName,
							},
						},
					},
				},
			},
		}
	}

	// NOTE: We are not adding support for Websockets here due to this code being
	// deprecated already. We expect this JSON API to be removed within 2021.

	return listener
}

// EnvoyListenersFromState creates a set of Enovy API listener
// definitions from all the ServicePorts in the Sidecar state.
func (s *EnvoyApi) EnvoyListenersFromState() []*EnvoyListener {
	listeners := make([]*EnvoyListener, 0)

	s.state.RLock()
	defer s.state.RUnlock()

	svcs := s.state.ByService()
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

			listeners = append(listeners, s.EnvoyListenerFromService(svc, port.ServicePort))
		}
	}

	return listeners
}

// HttpMux returns a configured Gorilla mux to handle all the endpoints
// for the Envoy API.
func (s *EnvoyApi) HttpMux() http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/registration/{service}", wrap(s.registrationHandler)).Methods("GET")
	router.HandleFunc("/clusters/{service_cluster}/{service_node}", wrap(s.clustersHandler)).Methods("GET")
	router.HandleFunc("/clusters", wrap(s.clustersHandler)).Methods("GET")
	router.HandleFunc("/listeners/{service_cluster}/{service_node}", wrap(s.listenersHandler)).Methods("GET")
	router.HandleFunc("/listeners", wrap(s.listenersHandler)).Methods("GET")
	router.HandleFunc("/{path}", s.optionsHandler).Methods("OPTIONS")

	return router
}
