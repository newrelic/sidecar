package sidecarhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sort"
	"time"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/service"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type ApiServer struct {
	Name         string
	LastUpdated  time.Time
	ServiceCount int
}

type ApiServices struct {
	Services       map[string][]*service.Service
	ClusterMembers map[string]*ApiServer `json:",omitempty"`
	ClusterName    string
}

type SidecarApi struct {
	list  *memberlist.Memberlist
	state *catalog.ServicesState
}

func (s *SidecarApi) HttpMux() http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/services/{name}.{extension}", wrap(s.oneServiceHandler)).Methods("GET")
	router.HandleFunc("/services.{extension}", wrap(s.servicesHandler)).Methods("GET")
	router.HandleFunc("/state.{extension}", wrap(s.stateHandler)).Methods("GET")
	router.HandleFunc("/watch", wrap(s.watchHandler)).Methods("GET")
	router.HandleFunc("/{path}", s.optionsHandler).Methods("OPTIONS")

	return router
}

// optionsHandler sends CORS headers
func (s *SidecarApi) optionsHandler(response http.ResponseWriter, req *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")
	return
}

// watchHandler takes an optional GET parameter, "by_service"
// By default, watchHandler returns `json.Marshal(state.ByService())` payloads
// If the client passes "by_service=false", watchHandler returns `json.Marshal(state)` payloads
func (s *SidecarApi) watchHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	listener := NewHttpListener()

	// Find out when the http connection closed so we can stop
	notify := response.(http.CloseNotifier).CloseNotify()

	// Let's subscribe to state change events
	// AddListener and RemoveListener are thread safe
	s.state.AddListener(listener)
	defer s.state.RemoveListener(listener.Name())

	byService := true
	if req.URL.Query().Get("by_service") == "false" {
		byService = false
	}

	var jsonBytes []byte
	pushUpdate := func() error {
		if byService {
			s.state.RLock()
			var err error
			jsonBytes, err = json.Marshal(s.state.ByService())
			s.state.RUnlock()

			if err != nil {
				return err
			}
		} else {
			s.state.RLock()
			jsonBytes = s.state.Encode()
			s.state.RUnlock()
		}

		// In order to flush immediately, we have to cast to a Flusher.
		// The normal HTTP library supports this but not all do, so we
		// check just in case.
		response.Write(jsonBytes)
		if f, ok := response.(http.Flusher); ok {
			f.Flush()
		}

		return nil
	}

	// Push the first update right away
	err := pushUpdate()
	if err != nil {
		log.Errorf("Error marshaling state in watchHandler: %s", err.Error())
		return
	}

	// Watch for further updates on the channel
	for {
		select {
		case <-notify:
			return

		case <-listener.Chan():
			err = pushUpdate()
			if err != nil {
				log.Errorf("Error marshaling state in watchHandler: %s", err.Error())
				return
			}
		}
	}
}

// oneServiceHandler takes the name of a single service and returns results for just
// that service.
func (s *SidecarApi) oneServiceHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	if params["extension"] != "json" {
		sendJsonError(response, 404, "Not Found - Invalid content type extension")
		return
	}

	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")
	response.Header().Set("Content-Type", "application/json")

	name, ok := params["name"]
	if !ok {
		sendJsonError(response, 404, "Not Found - No service name provided")
		return
	}

	if s.state == nil {
		sendJsonError(response, 500, "Internal Server Error - Something went terribly wrong")
		return
	}

	var instances []*service.Service
	// Enter critical section
	s.state.RLock()
	defer s.state.RUnlock()
	s.state.EachService(func(hostname *string, id *string, svc *service.Service) {
		if svc.Name == name {
			instances = append(instances, svc)
		}
	})

	// Did we have any entries for this service in the catalog?
	if len(instances) == 0 {
		sendJsonError(response, 404, fmt.Sprintf("no instances of %s found", name))
		return
	}

	clusterName := ""
	if s.list != nil {
		clusterName = s.list.ClusterName()
	}

	// Everything went fine, we found entries for this service.
	// Send the json back.
	svcInstances := make(map[string][]*service.Service)
	svcInstances[name] = instances
	result := ApiServices{
		Services:    svcInstances,
		ClusterName: clusterName,
	}

	jsonBytes, err := json.MarshalIndent(&result, "", "  ")
	if err != nil {
		log.Errorf("Error marshaling state in oneServiceHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	response.Write(jsonBytes)
}

// serviceHandler returns the results for all the services we know about
func (s *SidecarApi) servicesHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")

	// We only support JSON
	if params["extension"] != "json" {
		sendJsonError(response, 404, "Not Found - Invalid content type extension")
		return
	}

	response.Header().Set("Content-Type", "application/json")

	listMembers := s.list.Members()
	sort.Sort(listByName(listMembers))
	members := make(map[string]*ApiServer, len(listMembers))

	var jsonBytes []byte
	var err error

	func() { // Wrap critical section
		s.state.RLock()
		defer s.state.RUnlock()

		for _, member := range listMembers {
			if s.state.HasServer(member.Name) {
				members[member.Name] = &ApiServer{
					Name:         member.Name,
					LastUpdated:  s.state.Servers[member.Name].LastUpdated,
					ServiceCount: len(s.state.Servers[member.Name].Services),
				}
			} else {
				members[member.Name] = &ApiServer{
					Name:         member.Name,
					LastUpdated:  time.Unix(0, 0),
					ServiceCount: 0,
				}
			}
		}

		result := ApiServices{
			Services:       s.state.ByService(),
			ClusterMembers: members,
			ClusterName:    s.list.ClusterName(),
		}

		jsonBytes, err = json.MarshalIndent(&result, "", "  ")
	}()

	if err != nil {
		log.Errorf("Error marshaling state in servicesHandler: %s", err.Error())
		sendJsonError(response, 500, "Internal server error")
		return
	}

	response.Write(jsonBytes)
}

// stateHandler simply dumps the JSON output of the whole state object. This is
// useful for listeners or other clients that need a full state dump on startup.
func (s *SidecarApi) stateHandler(response http.ResponseWriter, req *http.Request, params map[string]string) {
	defer req.Body.Close()

	s.state.RLock()
	defer s.state.RUnlock()

	if params["extension"] == "json" {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		response.Header().Set("Access-Control-Allow-Methods", "GET")
		response.Write(s.state.Encode())
		return
	}
}

// Send back a JSON encoded error and message
func sendJsonError(response http.ResponseWriter, status int, message string) {
	output := map[string]string{
		"status":  "error",
		"message": message,
	}

	jsonBytes, err := json.Marshal(output)

	if err != nil {
		log.Errorf("Error encoding json error response: %s", err.Error())
		response.WriteHeader(500)
		response.Write([]byte("Interval server error"))
		return
	}

	http.Error(response, string(jsonBytes), status)
}

func wrap(fn func(http.ResponseWriter, *http.Request, map[string]string)) http.HandlerFunc {
	return func(response http.ResponseWriter, req *http.Request) {
		fn(response, req, mux.Vars(req))
	}
}

// Used by the servicesHandler to sort Memberlist cluster nodes
type listByName []*memberlist.Node

func (a listByName) Len() int           { return len(a) }
func (a listByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a listByName) Less(i, j int) bool { return a[i].Name < a[j].Name }
