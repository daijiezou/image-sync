package registryserver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"io"
	"net/http"
)

type Server struct {
	addr       string
	authServer string
	service    string
	username   string
	password   string
}

func Init(registryAddr string, authPath string) *Server {
	InitHttpClient()

	authServer, service := getAuthServerAndService("https://" + registryAddr)
	username, password := getRegistryAuthInfo(registryAddr, authPath)

	return &Server{
		addr:       "https://" + registryAddr,
		authServer: authServer,
		service:    service,
		username:   username,
		password:   password,
	}
}

type ManifestsResponse struct {
	Layers []LayerInfo `json:"layers"`
}

type LayerInfo struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

func (r *Server) GetImageDetail(
	ctx context.Context,
	projectName string,
	repoName string,
	tag string) (imageSize int64, err error) {

	imageName := projectName + "/" + repoName
	url := r.addr + "/v2" + fmt.Sprintf("/%s/manifests/%s", imageName, tag)
	var token string
	if r.authServer != "" {
		token, err = r.getToken(getScope(imageName))
		if err != nil {
			glog.Errorf("get token error, err:%s", err.Error())
			return 0, errors.WithStack(err)
		}
	}

	resp, err := registryHttpRequest(url, http.MethodGet, token, ctx)
	if err != nil {
		glog.Errorf("url:%s, err:%s", url)
		return 0, errors.WithStack(err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("query not ok")
	}
	body, _ := io.ReadAll(resp.Body)
	var manifestsResponse ManifestsResponse
	if err = json.Unmarshal(body, &manifestsResponse); err != nil {
		return 0, errors.WithStack(err)
	}
	totalSize := int64(0)
	for _, layer := range manifestsResponse.Layers {
		totalSize += layer.Size
	}
	return totalSize, nil
}

func (r *Server) getToken(scope string) (string, error) {
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
