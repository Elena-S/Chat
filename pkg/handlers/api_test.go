package handlers

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
)

var apiClient *resty.Client

func init() {
	apiClient = resty.New().SetBaseURL("https://web:8000").SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}

func TestApiNotAllowedMethod(t *testing.T) {
	type aml map[string]struct{}
	type apiMethodDef struct {
		uri            string
		allowedMethods aml
	}

	dummy := struct{}{}

	apiMethods := []apiMethodDef{
		{uri: "/chat/user", allowedMethods: aml{"GET": dummy, "POST": dummy}},
		{uri: "/chat/create", allowedMethods: aml{"POST": dummy}},
		{uri: "/chat/history", allowedMethods: aml{"GET": dummy, "POST": dummy}},
		{uri: "/chat/list", allowedMethods: aml{"GET": dummy, "POST": dummy}},
		{uri: "/chat/search", allowedMethods: aml{"GET": dummy, "POST": dummy}},
		{uri: "/authentication/refresh_tokens", allowedMethods: aml{"GET": dummy, "POST": dummy}}}

	listMethods := [5]string{"GET", "POST", "PUT", "PATCH", "DELETE"}

	for _, apiMethod := range apiMethods {
		for _, httpMethod := range listMethods {
			resp, err := apiClient.R().Execute(httpMethod, apiMethod.uri)
			if err != nil {
				t.Errorf("handlers: API method: %s, http request method: %s. %s", apiMethod.uri, httpMethod, err)
				continue
			}
			if _, ok := apiMethod.allowedMethods[httpMethod]; ok {
				if resp.StatusCode() == http.StatusMethodNotAllowed {
					t.Errorf("handlers: API method: %s, http request method: %s. HTTP method is allowed, but http status code is %d", apiMethod.uri, httpMethod, http.StatusMethodNotAllowed)
				}
				continue
			}
			if resp.StatusCode() != http.StatusMethodNotAllowed {
				t.Errorf("handlers: API method: %s, http request method: %s. Got the wrong status code: estimated - %d, gotten - %d", apiMethod.uri, httpMethod, http.StatusMethodNotAllowed, resp.StatusCode())
			}
		}
	}
}
