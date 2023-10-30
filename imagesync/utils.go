package imagesync

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"net/http"
	"os"
	"path"
	"strings"
)

var HttpClient *http.Client

func init() {
	HttpClient = createHttpClient()
}

func createHttpClient() *http.Client {
	client := &http.Client{
		Timeout: Timeout,
		Transport: func() *http.Transport {
			return &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				MaxIdleConns:    MaxIdleConns,
			}
		}(),
	}
	return client
}

func logMeta(meta DataImage) glog.Field {
	return glog.String("imageName", meta.Name+":"+meta.Tag)
}

func logError(err error) glog.Field {
	return glog.String("err", err.Error())
}

func imageYamlPath(imageName, imageTag, basePath string) string {
	imageName = strings.Replace(imageName, "/", "-", -1)
	return path.Join(basePath, imageName+":"+imageTag+".yaml")
}

func removeImageYaml(imageName, imageTag, basePath string) {
	err := os.Remove(imageYamlPath(imageName, imageTag, basePath))
	if err != nil {
		glog.Warn("remove image yaml failed", glog.String("error", err.Error()))
		return
	}
}

func getSyncSucceedImageList(outputPath string) []DataImage {
	file, err := os.Open(outputPath)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var imageList []DataImage
	for scanner.Scan() {
		var image DataImage
		err := json.Unmarshal(scanner.Bytes(), &image)
		if err != nil {
			continue
		}
		imageList = append(imageList, image)
	}
	return imageList
}

func splitImageNameToProjAndRepo(name string) (projectName string, repoName string) {
	// projectName/repositoryName
	projectName = strings.Split(name, "/")[0]
	repoName = strings.TrimPrefix(name, projectName+"/")
	return projectName, repoName
}
