package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/newrelic-forks/memberlist"
	"github.com/newrelic/sidecar/catalog"
	"github.com/newrelic/sidecar/output"
	"github.com/newrelic/sidecar/service"
)

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

	for {
		if state.LastChanged.After(lastChange) {
			lastChange = state.LastChanged
			jsonStr, _ := json.Marshal(state.ByService())
			response.Write(jsonStr)
			// In order to flush immediately, we have to cast to a Flusher.
			// The normal HTTP library supports this but not all do, so we
			// check just in case.
			if f, ok := response.(http.Flusher); ok {
				f.Flush()
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func servicesHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	params := mux.Vars(req)

	defer req.Body.Close()

	if params["extension"] == ".json" {
		response.Header().Set("Content-Type", "application/json")
		jsonStr, _ := json.MarshalIndent(state.ByService(), "", "  ")
		response.Write(jsonStr)
		return
	}
}

func serversHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *catalog.ServicesState) {
	defer req.Body.Close()

	response.Header().Set("Content-Type", "text/html")
	response.Write(
		[]byte(`
 			<head>
 			<meta http-equiv="refresh" content="4">
 			</head>
	    	<pre>` + state.Format(list) + "</pre>"))
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
		ports = append(ports, strconv.FormatInt(port.Port, 10))
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
		println("Error parsing template: " + err.Error())
	}

	members := list.Members()
	sort.Sort(listByName(members))

	compiledMembers := make([]*Member, len(members))
	for i, member := range members {
		if _, ok := state.Servers[member.Name]; ok {
			compiledMembers[i] = &Member{member, state.Servers[member.Name].LastUpdated}
		} else {
			compiledMembers[i] = &Member{Node: member}
			println("No updated time for " + member.Name)
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

func serveHttp(list *memberlist.Memberlist, state *catalog.ServicesState) {
	router := mux.NewRouter()

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
		"/watch", makeHandler(watchHandler, list, state),
	).Methods("GET")

	http.Handle("/", router)

	err := http.ListenAndServe("0.0.0.0:7777", nil)
	exitWithError(err, "Can't start HTTP server")
}
