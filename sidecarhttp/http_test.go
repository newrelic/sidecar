package sidecarhttp

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
)

// getResult fetches the status code, headers, and body from a recorder
func getResult(recorder *httptest.ResponseRecorder) (code int, headers *http.Header, body string) {
	resp := recorder.Result()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	body = string(bodyBytes)

	return resp.StatusCode, &resp.Header, body
}
