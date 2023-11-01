package imagesync

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/tealeg/xlsx"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/constant"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gopkg.in/yaml.v3"
	"image-sync/config"
	"image-sync/dao"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	BasePath          = "./"
	SyncSucceedResult = "Finished, 0 tasks failed"

	OfficialRepo = 1
	Published    = 1
)

var SyncSize int64 //此次同步镜像大小,单位B

type ThirdPkgSyncImageManager struct {
	sourceRegistryAddr   string
	targetRegistryAddr   string
	totalNeedSyncCount   int
	currentNeedSyncCount int
	lock                 sync.Mutex
	pullGoroutineChan    chan struct{}
	syncerPath           string
	outputPath           string
	authPath             string
	exitChan             chan struct{}
}

func NewThirdPkgSyncImageManager(syncerPath, authPath string) *ThirdPkgSyncImageManager {
	initTargetServer(config.IMConfig.TargetRegistryAddr, authPath)
	return &ThirdPkgSyncImageManager{
		sourceRegistryAddr: config.IMConfig.SourceRegistryAddr,
		targetRegistryAddr: config.IMConfig.TargetRegistryAddr,
		syncerPath:         syncerPath,
		outputPath:         config.IMConfig.OutputPath,
		authPath:           authPath,
		pullGoroutineChan:  make(chan struct{}, config.IMConfig.Proc),
		exitChan:           make(chan struct{}, 1),
	}
}

func (s *ThirdPkgSyncImageManager) GetNeedSyncImageMetaList() (needSyncImageMetaList []DataImage, err error) {
	var imageList []DataImage
	if config.IMConfig.ImageListPath != "" {
		imageList, err = s.preHandleData(config.IMConfig.ImageListPath)
		if err != nil {
			return imageList, err
		}
	} else {
		imageList, err = s.preHandleDataInDb(config.IMConfig.StartTime, config.IMConfig.EndTime)
		if err != nil {
			return imageList, err
		}
	}
	//过滤已经同步成功的镜像
	syncSucceedImageList := GetSyncSucceedImageList(path.Join(s.outputPath, "sync-succeed"))
	for _, image := range syncSucceedImageList {
		for i := 0; i < len(imageList); i++ {
			if image.Name == imageList[i].Name && image.Tag == imageList[i].Tag {
				imageList = append(imageList[:i], imageList[i+1:]...)
				break
			}
		}
	}
	s.totalNeedSyncCount = len(imageList)
	s.currentNeedSyncCount = len(imageList)
	glog.Infof("start sync image,total image:%d", s.totalNeedSyncCount)
	return imageList, nil
}

func (s *ThirdPkgSyncImageManager) preHandleData(imageListPath string) (needSyncImageMetaList []DataImage, err error) {
	xlFile, err := xlsx.OpenFile(imageListPath)
	if err != nil {
		return nil, errors.Wrap(err, "open xlsx file failed")
	}
	var jsonData []map[string]string
	// 遍历工作表
	for _, sheet := range xlFile.Sheets {
		for rowIndex, row := range sheet.Rows {
			if rowIndex == 0 {
				continue // Skip the header row
			}
			rowData := make(map[string]string)
			for cellIndex, cell := range row.Cells {
				headerCell := sheet.Rows[0].Cells[cellIndex] // Assuming the header is in the first row
				headerText := headerCell.String()
				cellText := cell.String()
				rowData[headerText] = cellText
			}
			jsonData = append(jsonData, rowData)
		}
	}
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}
	var imageList []DataImage
	err = json.Unmarshal(jsonBytes, &imageList)
	if err != nil {
		return nil, err
	}

	return imageList, nil
}

func (s *ThirdPkgSyncImageManager) preHandleDataInDb(startTime, endTime string) (needSyncImageMetaList []DataImage, err error) {
	var imageIds []int64
	err = dao.MySQL().Table("pro_job").Distinct("image_id").Select("image_id").
		Where("create_time > ?", startTime).
		And("create_time < ?", endTime).
		Find(&imageIds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var officialImageIds []int64
	err = dao.MySQL().Table("data_image").
		Select("data_image.image_id").
		Join("RIGHT", "data_image_repository", "data_image_repository.image_repository_id = data_image.image_repository_id ").
		And("data_image_repository.publish_status = ?", Published).
		And("data_image_repository.is_official= ?", OfficialRepo).
		And("data_image.libra_status = ?", constant.PavoStatusNormal).Find(&officialImageIds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	imageIds = append(imageIds, officialImageIds...)
	imageIds = removeDuplicateElement(imageIds)
	var imageList []DataImage
	err = dao.MySQL().Table("data_image").Select("image_id,image_name,image_tag,image_size").Where("libra_status = ?", constant.PavoStatusNormal).In("image_id", imageIds).Find(&imageList)
	if err != nil {
		return imageList, errors.WithStack(err)
	}
	return imageList, nil
}

func (s *ThirdPkgSyncImageManager) Sync(needSyncImageMetaList []DataImage) {
	if needSyncImageMetaList == nil || len(needSyncImageMetaList) == 0 {
		glog.Info("sync finished")
		return
	}
	go func() {
		for i := 0; i < s.totalNeedSyncCount; i++ {
			s.pullGoroutineChan <- struct{}{}
			go func(imageMeta DataImage) {
				s.sync(imageMeta)
			}(needSyncImageMetaList[i])
		}
	}()
	for {
		select {
		case <-s.exitChan:
			glog.Info("sync finished")
			return
		default:
			time.Sleep(time.Second * 5)
		}
	}
}

func (s *ThirdPkgSyncImageManager) sync(imageMeta DataImage) {
	defer func() {
		<-s.pullGoroutineChan
		removeImageYaml(imageMeta.Name, imageMeta.Tag, BasePath)
		s.decrNeedSyncCount()
	}()

	// 生成镜像同步规则文件
	// 参考:https://github.com/AliyunContainerService/image-syncer/blob/master/examples/images.yaml
	err := s.genImageYaml(imageMeta.Name, imageMeta.Tag, BasePath)
	if err != nil {
		glog.Warn("gen image yaml failed", logError(err), logMeta(imageMeta))
		return
	}

	cmd := exec.Command("sh")
	if bashPath, err := exec.LookPath("bash"); err == nil && bashPath != "" {
		cmd = exec.Command("bash")
	}
	cmd.Stdin = strings.NewReader("\n" + fmt.Sprintf("%s --images %s --auth %s --retries 3", s.syncerPath,
		imageYamlPath(imageMeta.Name, imageMeta.Tag, BasePath), s.authPath))

	output, err := cmd.CombinedOutput()
	syncOutput := string(output)
	fmt.Println(syncOutput)
	s.checkSyncStatus(imageMeta, syncOutput)
}

func (s *ThirdPkgSyncImageManager) checkSyncStatus(imageMeta DataImage, syncOutput string) {
	if strings.ContainsAny(syncOutput, SyncSucceedResult) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		projectName, repoName := splitImageNameToProjAndRepo(imageMeta.Name)
		// 查看目标镜像仓库，确定镜像是否迁移成功
		imageSize, err := targetRegistryServer.GetImageDetail(ctx, projectName, repoName, imageMeta.Tag)
		if err != nil {
			glog.Warnf("get image detail failed:%v", err.Error())
			imageMeta.Status = SyncFailed

		}
		if imageSize <= 0 {
			imageMeta.Status = SyncFailed
		} else {
			SyncSize += imageSize
			imageMeta.Size = strconv.FormatInt(imageSize, 10)
			imageMeta.Status = SyncSucceed
		}
	} else {
		imageMeta.Status = SyncFailed
	}
	s.UpdateImageSyncStatus(imageMeta)
	return
}

func (s *ThirdPkgSyncImageManager) decrNeedSyncCount() {
	s.lock.Lock()
	s.currentNeedSyncCount--
	glog.Infof("current need to sync image count:%d,total image count:%d", s.currentNeedSyncCount, s.totalNeedSyncCount)
	s.lock.Unlock()
	if s.currentNeedSyncCount == 0 {
		s.exitChan <- struct{}{}
	}
}

func (s *ThirdPkgSyncImageManager) genImageYaml(imageName, imageTag, bathPath string) error {
	imageConf := make(map[string]string)
	imageConf[path.Join(s.sourceRegistryAddr, imageName+":"+imageTag)] = path.Join(s.targetRegistryAddr, imageName)
	data, err := yaml.Marshal(imageConf)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.WriteFile(imageYamlPath(imageName, imageTag, bathPath), data, 0777)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (s *ThirdPkgSyncImageManager) UpdateImageSyncStatus(imageMeta DataImage) {
	imageMeta.CreateTime = time.Now()
	s.lock.Lock()
	defer s.lock.Unlock()
	data, err := json.Marshal(&imageMeta)
	if err != nil {
		glog.Warnw("update image sync status failed", logError(err), logMeta(imageMeta))
		return
	}
	var file *os.File
	if imageMeta.Status == SyncSucceed {
		glog.Infow("image sync succeed", logMeta(imageMeta))
		file, err = os.OpenFile(path.Join(s.outputPath, "sync-succeed"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			glog.Warnw("open file failed", logError(err), logMeta(imageMeta))
			return
		}
	} else {
		glog.Errorw("image sync failed", logMeta(imageMeta))
		file, err = os.OpenFile(path.Join(s.outputPath, "sync-failed"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			glog.Warnw("open file failed", logError(err), logMeta(imageMeta))
			return
		}
	}
	defer file.Close()
	// 写入要追加的内容
	file.WriteString("\n")
	_, err = file.Write(data)
	if err != nil {
		glog.Warnw("write file failed", logError(err), logMeta(imageMeta))
		return
	}
}

func genSavePath(imageMeta DataImage) string {
	imageMeta.Name = strings.Replace(imageMeta.Name, "/", "-", -1)
	//_, err := os.Create(path.Join(config.IMConfig.OutputPath, imageMeta.Name+"-"+imageMeta.Tag))
	//if err != nil {
	//	glog.Errorf("create file failed:%v", err)
	//}
	return path.Join(config.IMConfig.OutputPath, imageMeta.Name+"-"+imageMeta.Tag)
}
