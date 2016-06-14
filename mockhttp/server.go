package mockhttp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

func NewMockedTransport(server *httptest.Server) *http.Transport {
	return &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}
}

type HttpExpectation struct {
	Expect  string
	Send    string
	Err     error
	Content string
}

func NewMockedServer(expectations []HttpExpectation) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, e := range expectations {
			if strings.Contains(r.RequestURI, e.Expect) {
				// Handle error returns
				if e.Err != nil {
					http.Error(w, e.Err.Error(), http.StatusInternalServerError)
					return
				}

				//w.WriteHeader(status)
				w.Header().Set("Content-Type", e.Content)
				w.Write([]byte(e.Send))
			}
		}
	}))
}

func ClientWithExpectations(expectations []HttpExpectation) *http.Client {
	return &http.Client{Transport: NewMockedTransport(NewMockedServer(expectations))}
}
