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
	ClusterMembers map[string]*ApiServer
	ClusterName    string
}

func makeHandler(fn func(http.ResponseWriter, *http.Request,
	*memberlist.Memberlist, *catalog.ServicesState),
	list *memberlist.Memberlist, state *catalog.ServicesState) http.HandlerFunc {

	return func(response http.ResponseWriter, req *http.Request) {
		fn(response, req, list, state)
	}
}

func watchHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "application/json")

	lastChange := time.Unix(0, 0)
	var jsonBytes []byte
	var err error

	//Check for closed client and flip the breakLoop flag
	notify := response.(http.CloseNotifier).CloseNotify()
	breakLoop := false
	go func() {
		<-notify
		breakLoop = true
	}()

	for {
		var changed bool

		func() { // Wrap critical section
			state.RLock()
			defer state.RUnlock()

			if state.LastChanged.After(lastChange) {
				lastChange = state.LastChanged
				jsonBytes, err = json.Marshal(state.ByService())
				if err != nil {
					log.Errorf("Error marshaling state in watchHandler: %s", err.Error())
					return
				}
				changed = true // Trigger sending new encoding
			}
		}()

		if changed {
			// In order to flush immediately, we have to cast to a Flusher.
			// The normal HTTP library supports this but not all do, so we
			// check just in case.
			response.Write(jsonBytes)
			if f, ok := response.(http.Flusher); ok {
				f.Flush()
			}
		}
		time.Sleep(250 * time.Millisecond)

		if breakLoop { //if client has been closed, break the loop
			log.Debugf("HTTP connection just closed.")
			break
		}
	}
}

func servicesHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	params := mux.Vars(req)
	defer req.Body.Close()

	if params["extension"] == ".json" {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		response.Header().Set("Access-Control-Allow-Methods", "GET")

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
		}

		response.Write(jsonBytes)
		return
	}
}

func serversHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
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

func stateHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	params := mux.Vars(req)
	defer req.Body.Close()

	state.RLock()
	defer state.RUnlock()

	if params["extension"] == ".json" {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		response.Header().Set("Access-Control-Allow-Methods", "GET")
		response.Write(state.Encode())
		return
	}
}

func optionsHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "GET")
	return
}

func statusStr(status int) string {
	switch status {
	case 0:
		return "Alive"
	case 1:
		return "Tombstone"
	case 2:
		return "Unhealthy"
	default:
		return "Unknown"
	}
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

func viewHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	timeAgo := func(when time.Time) string { return output.TimeAgo(when, time.Now().UTC()) }

	funcMap := template.FuncMap{
		"statusStr":   statusStr,
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
		"/services{extension}", makeHandler(servicesHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/servers", makeHandler(serversHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/services", makeHandler(viewHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/state{extension}", makeHandler(stateHandler, list, state),
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
