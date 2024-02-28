package registryserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gopkg.in/yaml.v3"
	"net/http"
	"os"
	"strings"
)

func getScope(imageName string) string {
	return "repository:" + imageName + ":pull,push,delete"
}

type RegistryAuthInfo struct {
	Username string
	Password string
}

func getRegistryAuthInfo(registryAddr string, authPath string) (username, password string) {
	dataBytes, err := os.ReadFile(authPath)
	if err != nil {
		glog.Fatal(err.Error())
	}
	config := make(map[string]RegistryAuthInfo)
	err = yaml.Unmarshal(dataBytes, &config)
	if err != nil {
		glog.Fatal(err.Error())
	}
	return config[registryAddr].Username, config[registryAddr].Password
}

func registryHttpRequest(url, method, token string, ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	//增加header选项
	setDefaultHttpHeader(req, token)
	response, err := HttpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return response, nil
}

func setDefaultHttpHeader(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
}

func getAuthServerAndService(addr string) (authServer, service string) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequest(http.MethodGet, addr+"/v2/", nil)
	if err != nil {
		glog.Fatal(err.Error())
	}
	resp, err := client.Do(req)
	if err != nil {
		glog.Fatal(err.Error())
	}
	defer resp.Body.Close()
	challenge := resp.Header.Get("Www-Authenticate")
	if challenge == "" {
		return "", ""
	}
	// Bearer realm="http://10.12.10.149/service/token",service="harbor-registry"
	challenge = strings.ReplaceAll(challenge, "\"", "")
	authServer = strings.Split(strings.Split(challenge, ",")[0], "=")[1]
	service = strings.Split(strings.Split(challenge, ",")[1], "=")[1]
	return
}
