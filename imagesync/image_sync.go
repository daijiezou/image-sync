package imagesync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/constant"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-geminidb/model"
	"gopkg.in/yaml.v3"
	"image-sync/config"
	"image-sync/dao"
	"image-sync/registryserver"
	"io"
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
	SyncSucceedResult = "inished, 0 tasks failed"

	OfficialRepo = 1
	Published    = 1
)

var (
	SyncSize           int64 //此次同步镜像大小,单位B
	syncFailedCount    int
	totalNeedSyncCount int
)

type SyncImageManager struct {
	sourceRegistryAddr   string
	targetRegistryAddr   string
	syncerPath           string
	authPath             string
	pullGoroutineChan    chan struct{}
	exitChan             chan struct{}
	lock                 sync.Mutex
	syncStartTime        time.Time
	currentNeedSyncCount int
	targetRegistryServer *registryserver.Server
}

func NewSyncImageManager(
	syncerPath string,
	authPath string) *SyncImageManager {

	return &SyncImageManager{
		sourceRegistryAddr:   config.IMConfig.SourceRegistryAddr,
		targetRegistryAddr:   config.IMConfig.TargetRegistryAddr,
		syncerPath:           syncerPath,
		authPath:             authPath,
		pullGoroutineChan:    make(chan struct{}, config.IMConfig.Proc),
		exitChan:             make(chan struct{}, 1),
		targetRegistryServer: registryserver.Init(config.IMConfig.TargetRegistryAddr, authPath),
	}
}

func (s *SyncImageManager) GetNeedSyncImageMetaList() (needSyncImageMetaList []DataImage, err error) {
	var imageList []DataImage
	cm := config.IMConfig
	switch cm.Mode {
	case "sync":
		imageList, err = s.getNeedSyncImage(cm.StartTime, cm.EndTime, cm.TargetAzId)
		if err != nil {
			return imageList, err
		}
	case "migration":
		imageList, err = s.getNeedMigrationImage(cm.SourceAzId)
		if err != nil {
			return imageList, err
		}
	}

	//过滤已经同步成功的镜像
	syncSucceedImageMap := GetSyncSucceedImageMap(path.Join(config.IMConfig.OutputPath, "sync-succeed"))
	var unSyncImageList []DataImage
	for i := 0; i < len(imageList); i++ {
		if _, ok := syncSucceedImageMap[imageList[i].ID]; !ok {
			unSyncImageList = append(unSyncImageList, imageList[i])
		} else {
			glog.Infof("image %s already sync succeed", imageList[i].ID)
		}
	}
	s.syncStartTime = time.Now()
	glog.Infof("start sync image,total image:%d", len(unSyncImageList))
	totalNeedSyncCount = len(unSyncImageList)
	s.currentNeedSyncCount = len(unSyncImageList)
	return unSyncImageList, nil
}

func (s *SyncImageManager) Sync(needSyncImageMetaList []DataImage) {
	if needSyncImageMetaList == nil || len(needSyncImageMetaList) == 0 {
		glog.Info("sync finished")
		return
	}
	go func() {
		for i := 0; i < totalNeedSyncCount; i++ {
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

func (s *SyncImageManager) sync(imageMeta DataImage) {
	defer func() {
		<-s.pullGoroutineChan
		removeImageYaml(imageMeta.Name, imageMeta.Tag, BasePath)
		s.decrNeedSyncCount()
		glog.Infof("current need to sync image count:%d,total image count:%d", s.currentNeedSyncCount, totalNeedSyncCount)
		costTimeSec := time.Now().Sub(s.syncStartTime).Seconds()
		glog.Infof("synced image size:%v GB,synced time:%v,sync speed:%.2f MB/s\n", SyncSize>>30,
			formatDuration(time.Since(s.syncStartTime)),
			float64(SyncSize>>20)/costTimeSec)
	}()

	glog.Info("start sync image", logMeta(imageMeta))
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
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	var syncOutput string
	if err = cmd.Start(); err != nil {
		s.checkSyncResult(imageMeta, syncOutput)
		return
	}
	var imageSyncEOFCount int
	reader := bufio.NewReader(stdout)
	for {
		line, err2 := reader.ReadString('\n')
		if err2 != nil {
			if err2 != io.EOF {
				glog.Errorf("cmd exec failed,err:", err.Error())
			}
			break
		}
		fmt.Println(line)
		// when sync progress occurs this error,this process will hang
		if strings.Contains(line, "unexpected EOF") {
			imageSyncEOFCount++
			if imageSyncEOFCount == 5 {
				glog.Errorf("sync image failed,err:unexpected EOF,image source data maybe corruption")
				s.checkSyncResult(imageMeta, syncOutput)
				return
			}
		}
		syncOutput += line
	}
	if err = cmd.Wait(); err != nil {
		glog.Errorf("cmd exec failed:%v", err.Error())
		s.checkSyncResult(imageMeta, syncOutput)
		return
	}
	s.checkSyncResult(imageMeta, syncOutput)
}

// get images used between startTime and endTime and official image,and targetAz registry don't have this image
func (s *SyncImageManager) getNeedSyncImage(
	startTime string,
	endTime string,
	targetAzId string) (needSyncImageMetaList []DataImage, err error) {

	// 按照起始、结束时间过滤任务使用过的镜像
	var imageIds []int64
	err = dao.MySQL().Table("pro_job").Distinct("image_id").Select("image_id").
		Where("create_time > ?", startTime).
		And("create_time < ?", endTime).
		Find(&imageIds)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 查询所有官方镜像
	var officialImageIds []int64
	err = dao.MySQL().Table("data_image").
		Select("data_image.image_id").
		Join("RIGHT", "data_image_repository",
			"data_image_repository.image_repository_id = data_image.image_repository_id ").
		And("data_image_repository.publish_status = ?", Published).
		And("data_image_repository.is_official= ?", OfficialRepo).
		And("data_image.libra_status = ?", constant.PavoStatusNormal).
		Find(&officialImageIds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 按照ID镜像去重
	imageIds = append(imageIds, officialImageIds...)
	imageIds = removeDuplicateElement(imageIds)

	var imageList []DataImage
	err = dao.MySQL().Table("data_image").
		Select("image_id,image_name,image_tag,image_size").
		Where("libra_status = ?", constant.PavoStatusNormal).
		In("image_id", imageIds).
		Find(&imageList)
	if err != nil {
		return imageList, errors.WithStack(err)
	}

	var result []DataImage
	// 选择目标集群没有的那些镜像
	for _, image := range imageList {
		has, err := dao.MySQL().Table("image_metadata").
			Where("name = ?", image.Name).
			And("tag = ?", image.Tag).
			And("az_id = ?", targetAzId).Get(new(model.ImageMetadata))
		if err != nil {
			glog.Errorf("get image metadata failed,err:%v", err)
			continue
		}
		if !has {
			result = append(result, image)
		}
	}
	return result, nil
}

func (s *SyncImageManager) getNeedMigrationImage(offlineAzId string) (needSyncImageMetaList []DataImage, err error) {
	if offlineAzId == "" {
		return needSyncImageMetaList, errors.New("offline az id can not be empty")
	}
	var imageMetas []ImageMetadata
	err = dao.MySQL().Table("image_metadata").
		Where("az_id = ?", offlineAzId).
		And("sync_status = 1"). // 1:未同步回中控
		Find(&imageMetas)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	imageList := make([]DataImage, 0, len(imageMetas))
	for _, imageMeta := range imageMetas {
		dataImage := new(DataImage)
		has, err := dao.MySQL().Table("data_image").Select("image_id,image_name,image_tag,image_size").
			Where("image_name = ?", imageMeta.Name).And("image_tag = ?", imageMeta.Tag).Get(dataImage)
		if !has {
			glog.Warnf("image %s:%s not exist", imageMeta.Name, imageMeta.Tag)
			continue
		}
		if err != nil {
			glog.Errorf("get image  %s:%s error:%s", imageMeta.Name, imageMeta.Tag, err.Error())
			return imageList, errors.WithStack(err)
		}
		imageList = append(imageList, *dataImage)
	}
	return imageList, nil
}
func (s *SyncImageManager) checkSyncResult(imageMeta DataImage, syncOutput string) {
	if strings.Contains(syncOutput, SyncSucceedResult) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		projectName, repoName := splitImageNameToProjAndRepo(imageMeta.Name)
		// 查看目标镜像仓库，确定镜像是否迁移成功
		imageSize, err := s.targetRegistryServer.GetImageDetail(ctx, projectName, repoName, imageMeta.Tag)
		if err != nil {
			glog.Warnf("get image detail failed:%+v", err, logMeta(imageMeta))
			imageMeta.Status = SyncFailed
		}
		if imageSize <= 0 {
			imageMeta.Status = SyncFailed
		} else {
			s.lock.Lock()
			SyncSize += imageSize
			s.lock.Unlock()
			imageMeta.Size = strconv.FormatInt(imageSize, 10)
			imageMeta.Status = SyncSucceed
		}
	} else {
		imageMeta.Status = SyncFailed
	}
	s.recordImageSyncResult(imageMeta)
	return
}

func (s *SyncImageManager) decrNeedSyncCount() {
	s.lock.Lock()
	s.currentNeedSyncCount--
	s.lock.Unlock()
	if s.currentNeedSyncCount == 0 {
		s.exitChan <- struct{}{}
	}
}

func (s *SyncImageManager) genImageYaml(imageName, imageTag, bathPath string) error {
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

func (s *SyncImageManager) recordImageSyncResult(imageMeta DataImage) {
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
		file, err = os.OpenFile(path.Join(config.IMConfig.OutputPath, "sync-succeed"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			glog.Warnw("open file failed", logError(err), logMeta(imageMeta))
			return
		}
	} else {
		syncFailedCount++
		glog.Errorw("image sync failed", logMeta(imageMeta))
		glog.Infof("image sync failed count:%v", syncFailedCount)
		file, err = os.OpenFile(path.Join(config.IMConfig.OutputPath, "sync-failed"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			glog.Warnw("open file failed", logError(err), logMeta(imageMeta))
			return
		}
	}
	defer file.Close()
	// 写入要追加的内容
	_, err = file.Write(append(data, []byte("\n")...))
	if err != nil {
		glog.Warnw("write file failed", logError(err), logMeta(imageMeta))
		return
	}
}
