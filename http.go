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
	"github.com/hashicorp/memberlist"
	"github.com/newrelic/bosun/output"
	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/services_state"
)


func makeHandler(fn func (http.ResponseWriter, *http.Request, *memberlist.Memberlist, *services_state.ServicesState), list *memberlist.Memberlist, state *services_state.ServicesState) http.HandlerFunc {
	return func(response http.ResponseWriter, req *http.Request) {
		fn(response, req, list, state)
	}
}

func servicesHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *services_state.ServicesState) {
	params := mux.Vars(req)

	defer req.Body.Close()

	if params["extension"] == ".json" {
		response.Header().Set("Content-Type", "application/json")
		jsonStr, _ := json.MarshalIndent(state.ByService(), "", "  ")
		response.Write(jsonStr)
		return
	}

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
	default:
		return ""
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

func viewHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist, state *services_state.ServicesState) {
	timeAgo := func(when time.Time) string { return output.TimeAgo(when, time.Now().UTC()) }

	funcMap := template.FuncMap{
		"statusStr": statusStr,
		"timeAgo": timeAgo,
		"portsStr": portsStr,
	}

	t, err := template.New("services").Funcs(funcMap).ParseFiles("views/services.html")
	if err != nil {
		println("Error parsing template: " + err.Error())
	}

	members := list.Members()
	sort.Sort(listByName(members))

	updatedTimes := make([]time.Time, len(members))
	for i, member := range members {
		if _, ok := state.Servers[member.Name]; ok {
			updatedTimes[i] = state.Servers[member.Name].LastUpdated
		} else {
			println("No updated time for " + member.Name)
		}
	}

	viewData := struct {
		Services map[string][]*service.Service
		Members []*memberlist.Node
		UpdatedTimes []time.Time
	}{
		Services: state.ByService(),
		Members:  members,
		UpdatedTimes:  updatedTimes,
	}

	t.ExecuteTemplate(response, "services.html", viewData)
}


func serveHttp(list *memberlist.Memberlist, state *services_state.ServicesState) {
	router := mux.NewRouter()

	router.HandleFunc(
		"/services{extension}", makeHandler(servicesHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/servers", makeHandler(servicesHandler, list, state),
	).Methods("GET")

	router.HandleFunc(
		"/services", makeHandler(viewHandler, list, state),
	).Methods("GET")

	http.Handle("/", router)

	err := http.ListenAndServe("0.0.0.0:7777", nil)
	exitWithError(err, "Can't start HTTP server")
}
