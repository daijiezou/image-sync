package registryserver

import (
	"crypto/tls"
	"net/http"
	"time"
)

var HttpClient *http.Client

func InitHttpClient() {
	HttpClient = &http.Client{
		Timeout: 20 * time.Second,
		Transport: func() *http.Transport {
			return &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				MaxIdleConns:    20,
			}
		}(),
	}
}
