package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	_ "net/http/pprof"
	"sort"
	"strings"
	"time"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/output"
	"github.com/Nitro/sidecar/service"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
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

// A ServicesState.Listener that we use for the /watch endpoint
type HttpListener struct {
	eventChan chan catalog.ChangeEvent
	name      string
}

func NewHttpListener() *HttpListener {
	return &HttpListener{
		// This should be fine enough granularity for practical purposes
		name: fmt.Sprintf("httpListener-%d", time.Now().UTC().UnixNano()),
		// Listeners must have buffered channels. We'll use a
		// somewhat larger buffer here because of the slow link
		// problem with http
		eventChan: make(chan catalog.ChangeEvent, 50),
	}
}

func (h *HttpListener) Chan() chan catalog.ChangeEvent {
	return h.eventChan
}

func (h *HttpListener) Name() string {
	return h.name
}

func makeHandler(fn func(http.ResponseWriter, *http.Request,
	*memberlist.Memberlist, *catalog.ServicesState, map[string]string),
	list *memberlist.Memberlist, state *catalog.ServicesState) http.HandlerFunc {

	return func(response http.ResponseWriter, req *http.Request) {
		fn(response, req, list, state, mux.Vars(req))
	}
}

// watchHandler takes an optional GET parameter, "by_service"
// By default, watchHandler returns `json.Marshal(state.ByService())` payloads
// If the client passes "by_service=false", watchHandler returns `json.Marshal(state)` payloads
func watchHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	listener := NewHttpListener()

	// Find out when the http connection closed so we can stop
	notify := response.(http.CloseNotifier).CloseNotify()

	// Let's subscribe to state change events
	state.AddListener(listener)
	defer state.RemoveListener(listener.Name())

	byService := true
	if req.URL.Query().Get("by_service") == "false" {
		byService = false
	}

	var jsonBytes []byte
	pushUpdate := func() error {
		if byService {
			var err error
			jsonBytes, err = json.Marshal(state.ByService())

			if err != nil {
				return err
			}
		} else {
			jsonBytes = state.Encode()
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

// Reply with an error status and message
func sendError(response http.ResponseWriter, status int, message string) {
	response.WriteHeader(status)
	response.Write([]byte(message))
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

	response.WriteHeader(status)
	response.Write(jsonBytes)
}

// Helper for returning an error for an incorrect extension
func invalidContentType(response http.ResponseWriter) {
	sendError(response, 404, "Not Found - Invalid content type extension")
}

func oneServiceHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	defer req.Body.Close()

	if params["extension"] != "json" {
		invalidContentType(response)
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

	if state == nil {
		sendJsonError(response, 500, "Internal Server Error - Something went terribly wrong")
		return
	}

	var instances []*service.Service
	state.EachService(func(hostname *string, id *string, svc *service.Service) {
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
	if list != nil {
		clusterName = list.ClusterName()
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

func servicesHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")

	// We only support JSON
	if params["extension"] != "json" {
		invalidContentType(response)
		return
	}

	response.Header().Set("Content-Type", "application/json")

	listMembers := list.Members()
	sort.Sort(listByName(listMembers))
	members := make(map[string]*ApiServer, len(listMembers))

	var jsonBytes []byte
	var err error

	func() { // Wrap critical section
		state.RLock()
		defer state.RUnlock()

		for _, member := range listMembers {
			if state.HasServer(member.Name) {
				members[member.Name] = &ApiServer{
					Name:         member.Name,
					LastUpdated:  state.Servers[member.Name].LastUpdated,
					ServiceCount: len(state.Servers[member.Name].Services),
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
			Services:       state.ByService(),
			ClusterMembers: members,
			ClusterName:    list.ClusterName(),
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

func serversHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "text/html")
	state.RLock()
	defer state.RUnlock()

	response.Write(
		[]byte(`
 			<head>
 			<meta http-equiv="refresh" content="4">
 			</head>
	    	<pre>` + state.Format(list) + "</pre>"))
}

func stateHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	defer req.Body.Close()

	state.RLock()
	defer state.RUnlock()

	if params["extension"] == "json" {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		response.Header().Set("Access-Control-Allow-Methods", "GET")
		response.Write(state.Encode())
		return
	}
}

func optionsHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")
	return
}

func portsStr(svcPorts []service.Port) string {
	var ports []string

	for _, port := range svcPorts {
		if port.ServicePort != 0 {
			ports = append(ports, fmt.Sprintf("%v->%v", port.ServicePort, port.Port))
		} else {
			ports = append(ports, fmt.Sprintf("%v", port.Port))
		}
	}

	return strings.Join(ports, ", ")
}

type listByName []*memberlist.Node

func (a listByName) Len() int           { return len(a) }
func (a listByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a listByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

type Member struct {
	Node    *memberlist.Node
	Updated time.Time
}

func lineWrapMembers(cols int, fields []*Member) [][]*Member {
	if len(fields) < cols {
		return [][]*Member{fields}
	}

	retval := make([][]*Member, len(fields)/cols+1)
	for i := 0; i < len(fields); i++ {
		row := i / cols
		retval[row] = append(retval[row], fields[i])
	}

	return retval
}

func viewHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState, params map[string]string) {
	timeAgo := func(when time.Time) string { return output.TimeAgo(when, time.Now().UTC()) }

	funcMap := template.FuncMap{
		"statusStr":   service.StatusString,
		"timeAgo":     timeAgo,
		"portsStr":    portsStr,
		"clusterName": func() string { return list.ClusterName() },
	}

	t, err := template.New("services").Funcs(funcMap).ParseFiles("views/services.html")
	if err != nil {
		log.Errorf("Error parsing template: %s", err.Error())
	}

	members := list.Members()
	sort.Sort(listByName(members))

	state.RLock()
	defer state.RUnlock()

	compiledMembers := make([]*Member, len(members))
	for i, member := range members {
		if state.HasServer(member.Name) {
			compiledMembers[i] = &Member{member, state.Servers[member.Name].LastUpdated}
		} else {
			compiledMembers[i] = &Member{Node: member}
			log.Debug("No updated time for " + member.Name)
		}
	}

	wrappedMembers := lineWrapMembers(5, compiledMembers)

	viewData := struct {
		Services map[string][]*service.Service
		Members  [][]*Member
	}{
		Services: state.ByService(),
		Members:  wrappedMembers,
	}

	t.ExecuteTemplate(response, "services.html", viewData)
}

func uiRedirectHandler(response http.ResponseWriter, req *http.Request) {
	http.Redirect(response, req, "/ui/", 301)
}

func serveHttp(list *memberlist.Memberlist, state *catalog.ServicesState) {
	router := mux.NewRouter()

	router.HandleFunc("/", uiRedirectHandler).Methods("GET")

	router.HandleFunc(
		"/services/{name}.{extension}", makeHandler(oneServiceHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/services.{extension}", makeHandler(servicesHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/servers", makeHandler(serversHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/services", makeHandler(viewHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/state.{extension}", makeHandler(stateHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/watch", makeHandler(watchHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/{path}", makeHandler(optionsHandler, list, state),
	).Methods("OPTIONS")

	staticFs := http.FileServer(http.Dir("views/static"))
	router.PathPrefix("/static").Handler(http.StripPrefix("/static", staticFs))

	uiFs := http.FileServer(http.Dir("ui/app"))
	router.PathPrefix("/ui").Handler(http.StripPrefix("/ui", uiFs))

	http.Handle("/", router)

	err := http.ListenAndServe("0.0.0.0:7777", nil)
	exitWithError(err, "Can't start HTTP server")
}
