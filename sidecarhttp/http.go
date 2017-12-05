package sidecarhttp

import (
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type HttpConfig struct {
	BindIP       string
	UseHostnames bool
}

func makeHandler(fn func(http.ResponseWriter, *http.Request,
	*memberlist.Memberlist, *catalog.ServicesState, map[string]string),
	list *memberlist.Memberlist, state *catalog.ServicesState) http.HandlerFunc {

	return func(response http.ResponseWriter, req *http.Request) {
		fn(response, req, list, state, mux.Vars(req))
	}
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

type Member struct {
	Node    *memberlist.Node
	Updated time.Time
}

func uiRedirectHandler(response http.ResponseWriter, req *http.Request) {
	http.Redirect(response, req, "/ui/", 301)
}

func ServeHttp(list *memberlist.Memberlist, state *catalog.ServicesState, config *HttpConfig) {
	srvrsHandle := makeHandler(serversHandler, list, state)
	staticFs := http.FileServer(http.Dir("views/static"))
	uiFs := http.FileServer(http.Dir("ui/app"))

	api := &SidecarApi{state: state, list: list}
	envoyApi := &EnvoyApi{state: state, list: list, config: config}

	router := mux.NewRouter()
	router.HandleFunc("/", uiRedirectHandler).Methods("GET")
	router.HandleFunc("/servers", srvrsHandle).Methods("GET")
	router.PathPrefix("/static").Handler(http.StripPrefix("/static", staticFs))
	router.PathPrefix("/ui").Handler(http.StripPrefix("/ui", uiFs))
	router.PathPrefix("/api").Handler(http.StripPrefix("/api", api.HttpMux()))
	router.PathPrefix("/v1").Handler(http.StripPrefix("/v1", envoyApi.HttpMux()))

	// DEPRECATED - to be removed once common clients are updated
	router.HandleFunc("/state.{extension}", wrap(api.stateHandler)).Methods("GET")
	router.HandleFunc("/watch", wrap(api.watchHandler)).Methods("GET")
	// ------------------------------------------------------------

	http.Handle("/", router)

	err := http.ListenAndServe("0.0.0.0:7777", nil)
	if err != nil {
		log.Fatalf("Can't start HTTP server: %s", err)
	}
}
