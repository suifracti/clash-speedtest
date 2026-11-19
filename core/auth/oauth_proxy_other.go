//go:build !windows

package auth

import (
	"net/http"
	"time"
)

func newOAuthHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
}
