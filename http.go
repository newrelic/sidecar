package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hashicorp/memberlist"
)


func makeQueryHandler(fn func (http.ResponseWriter, *http.Request, *memberlist.Memberlist), list *memberlist.Memberlist) http.HandlerFunc {
	return func(response http.ResponseWriter, req *http.Request) {
		fn(response, req, list)
	}
}

func servicesQueryHandler(response http.ResponseWriter, req *http.Request, list *memberlist.Memberlist) {
	//params := mux.Vars(req)

	defer req.Body.Close()

	response.Header().Set("Content-Type", "text/html")
	response.Write(
		[]byte(`
 			<head>
 			<meta http-equiv="refresh" content="4">
 			</head> 
	    	<pre>` + formatServices(list) + "</pre>"))
}

func serveHttp(list *memberlist.Memberlist) {
	router := mux.NewRouter()

	router.HandleFunc(
		"/services", makeQueryHandler(servicesQueryHandler, list),
	).Methods("GET")

	http.Handle("/", router)

	err := http.ListenAndServe("0.0.0.0:7777", nil)
	exitWithError(err, "Can't start HTTP server")
}
