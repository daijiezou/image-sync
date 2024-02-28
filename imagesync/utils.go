package imagesync

import (
	"bufio"
	"encoding/json"
	"fmt"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"os"
	"path"
	"strings"
	"time"
)

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

func GetSyncSucceedImageList(outputPath string) []DataImage {
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

func GetSyncSucceedImageMap(outputPath string) map[string]struct{} {
	result := make(map[string]struct{})
	file, err := os.Open(outputPath)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var image DataImage
		err := json.Unmarshal(scanner.Bytes(), &image)
		if err != nil {
			continue
		}
		result[image.ID] = struct{}{}
	}
	return result
}

func splitImageNameToProjAndRepo(name string) (projectName string, repoName string) {
	// projectName/repositoryName
	projectName = strings.Split(name, "/")[0]
	repoName = strings.TrimPrefix(name, projectName+"/")
	return projectName, repoName
}

func removeDuplicateElement(originList []int64) []int64 {
	result := make([]int64, 0, len(originList))
	temp := map[int64]struct{}{}
	for _, item := range originList {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
