package imagesync

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	MaxIdleConns = 20
	Timeout      = 20 * time.Second
)

type RegistryServer struct {
	addr       string
	authServer string
	service    string
	username   string
	password   string
}

type ManifestsResponse struct {
	Layers []LayerInfo `json:"layers"`
}

type LayerInfo struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

var targetRegistryServer *RegistryServer

func initTargetServer(registryAddr string, authPath string) {
	authServer, service := getAuthServerAndService("https://" + registryAddr)
	username, password := getRegistryAuthInfo(registryAddr, authPath)
	targetRegistryServer = &RegistryServer{
		addr:       "https://" + registryAddr,
		authServer: authServer,
		service:    service,
		username:   username,
		password:   password,
	}
}

func (r *RegistryServer) GetImageDetail(ctx context.Context, projectName, repoName, tag string) (imageSize int64, err error) {
	imageName := projectName + "/" + repoName
	path := fmt.Sprintf("/%s/manifests/%s", imageName, tag)
	url := r.addr + "/v2" + path
	token, err := r.GetToken(GetScope(imageName))
	if err != nil {
		return 0, err
	}
	resp, err := registryHttpRequest(url, http.MethodGet, token, ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("query not ok")
	}
	body, _ := io.ReadAll(resp.Body)
	var manifestsResponse ManifestsResponse
	if err := json.Unmarshal(body, &manifestsResponse); err != nil {
		return 0, err
	}
	totalSize := int64(0)
	for _, layer := range manifestsResponse.Layers {
		totalSize += layer.Size
	}
	return totalSize, nil
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

func (r *RegistryServer) GetToken(scope string) (string, error) {
	// request auth server get token
	// example pullRequest 	http://10.12.10.149/service/token?account=daijun&scope=repository:djdemo/myds:pull&service=harbor-registry
	addr := fmt.Sprintf("%s?account=%s&scope=%s&service=%s", r.authServer, r.username, scope, r.service)
	req, err := http.NewRequest(http.MethodGet, addr, http.NoBody)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(r.username, r.password)
	resBody, err := HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resBody.Body.Close()
	type RcrRes struct {
		Code        string `json:"code"`
		Message     string `json:"message"`
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if resBody.StatusCode != http.StatusOK {
		return "", errors.New("auth error")
	}
	res := new(RcrRes)
	body, err := io.ReadAll(resBody.Body)
	if err = json.Unmarshal(body, &res); err != nil {
		return "", err
	}
	return res.Token, nil
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
	// Bearer realm="http://10.12.10.149/service/token",service="harbor-registry"
	challenge = strings.ReplaceAll(challenge, "\"", "")
	authServer = strings.Split(strings.Split(challenge, ",")[0], "=")[1]
	service = strings.Split(strings.Split(challenge, ",")[1], "=")[1]
	return
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

func GetScope(imageName string) string {
	return "repository:" + imageName + ":pull,push,delete"
}
