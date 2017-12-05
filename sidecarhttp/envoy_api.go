package sidecarhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"strings"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const (
	// Used to join service name and port. Must not occur in service names
	ServiceNameSeparator = ":"
)

// This file implements the Envoy proxy V1 API on top of a Sidecar
// service discovery cluster.

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

// A basic Envoy Http Route Filter
type EnvoyFilter struct {
	Name   string                 `json:"name"`
	Config *EnvoyHttpFilterConfig `json:"config"`
}

type EnvoyHttpFilterConfig struct {
	CodecType   string            `json:"codec_type,omitempty"`
	StatPrefix  string            `json:"stat_prefix,omitempty"`
	RouteConfig *EnvoyRouteConfig `json:"route_config,omitempty"`
	Filters     []*EnvoyFilter    `json:"filters,omitempty"`
}

type EnvoyVirtualHost struct {
	Name    string        `json:"name"`
	Domains []string      `json:"domains"`
	Routes  []*EnvoyRoute `json:"routes"`
}

type EnvoyRouteConfig struct {
	VirtualHosts []*EnvoyVirtualHost `json:"virtual_hosts"`
}

type EnvoyRoute struct {
	TimeoutMs   int    `json:"timeout_ms"`
	Prefix      string `json:"prefix"`
	HostRewrite string `json:"host_rewrite"`
	Cluster     string `json:"cluster"`
}

type EnvoyApi struct {
	list   *memberlist.Memberlist
	state  *catalog.ServicesState
	config *HttpConfig
}

// optionsHandler sends CORS headers
func (s *EnvoyApi) optionsHandler(response http.ResponseWriter, req *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")
	return
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

	svcName, svcPort, err := SvcNameSplit(name)
	if err != nil {
		log.Debugf("Envoy Service '%s' not found in registrationHandler: %s", name, err)
		sendJsonError(response, 404, "Not Found - "+err.Error())
		return
	}

	var instances []*EnvoyService

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

	// Did we have any entries for this service in the catalog?
	if len(instances) == 0 {
		log.Debugf("Envoy Service '%s' has no instances!", name)
		sendJsonError(response, 404, fmt.Sprintf("no instances of %s found", name))
		return
	}

	clusterName := ""
	if s.list != nil {
		clusterName = s.list.ClusterName()
	}

	result := struct {
		Env     string          `json:"env"`
		Hosts   []*EnvoyService `json:"hosts"`
		Service string          `json:"service"`
	}{
		clusterName,
		instances,
		name,
	}

	jsonBytes, err := json.MarshalIndent(&result, "", "  ")
	if err != nil {
		log.Errorf("Error marshaling state in registrationHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	response.Write(jsonBytes)
}

// clustersHandler returns cluster information for all Sidecar services. It
// implements the Envoy CDS API V1.
func (s *EnvoyApi) clustersHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	clusters := s.EnvoyClustersFromState()

	log.Debugf("Reporting Envoy cluster information for cluster '%s' and node '%s'",
		params["service_cluster"], params["service_node"])

	result := struct {
		Clusters []*EnvoyCluster `json:"clusters"`
	}{
		clusters,
	}

	jsonBytes, err := json.MarshalIndent(&result, "", "  ")

	if err != nil {
		log.Errorf("Error marshaling state in servicesHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	response.Write(jsonBytes)
}

// listenersHandler returns a list of listeners for all ServicePorts. It
// implements the Envoy LDS API V1.
func (s *EnvoyApi) listenersHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	log.Debugf("Reporting Envoy cluster information for cluster '%s' and node '%s'",
		params["service_cluster"], params["service_node"])

	listeners := s.EnvoyListenersFromState()

	result := struct {
		Listeners []*EnvoyListener `json:"listeners"`
	}{
		listeners,
	}

	jsonBytes, err := json.MarshalIndent(&result, "", "  ")
	if err != nil {
		log.Errorf("Error marshaling state in servicesHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	response.Write(jsonBytes)
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
			return &EnvoyService{
				IPAddress:       port.IP,
				LastCheckIn:     svc.Updated.String(),
				Port:            port.Port,
				Revision:        svc.Version(),
				Service:         SvcName(svc.Name, port.ServicePort),
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
	var clusters []*EnvoyCluster

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
				Name:             SvcName(svcName, port.ServicePort),
				Type:             "sds", // use Sidecar's SDS endpoint for the hosts
				ConnectTimeoutMs: 500,
				LBType:           "round_robin", // TODO figure this out!
				ServiceName:      SvcName(svcName, port.ServicePort),
			})
		}
	}

	return clusters
}

func (s *EnvoyApi) EnvoyListenerFromService(svc *service.Service, port int64) *EnvoyListener {
	apiName := SvcName(svc.Name, port)
	// Holy indentation, Bat Man!
	return &EnvoyListener{
		Name: apiName,
		// TODO need to do something similar to HAPROXY_USE_HOSTNAMES here
		Address: fmt.Sprintf("tcp://%s:%d", s.config.BindIP, port),
		Filters: []*EnvoyFilter{
			{
				Name: "envoy.http_connection_manager",
				Config: &EnvoyHttpFilterConfig{
					CodecType:  "auto",
					StatPrefix: "ingress_http",
					Filters: []*EnvoyFilter{
						{
							Name: "router",
							Config: &EnvoyHttpFilterConfig{},
						},
					},
					RouteConfig: &EnvoyRouteConfig{
						VirtualHosts: []*EnvoyVirtualHost{
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
		},
	}
}

// EnvoyListenersFromState creates a set of Enovy API listener
// definitions from all the ServicePorts in the Sidecar state.
func (s *EnvoyApi) EnvoyListenersFromState() []*EnvoyListener {
	var listeners []*EnvoyListener

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

// Format an Envoy service name from our service name and port
func SvcName(name string, port int64) string {
	return fmt.Sprintf("%s%s%d", name, ServiceNameSeparator, port)
}

// Split an Enovy service name into our service name and port
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
