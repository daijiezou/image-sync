package update

import (
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-geminidb/model"
	"image-sync/config"
	"image-sync/dao"
	"image-sync/imagesync"
	"path"
	"strconv"
)

const centralAz = "az1"

func UpdateImageMeta() {
	imageList := imagesync.GetSyncSucceedImageList(path.Join(config.IMConfig.OutputPath, "sync-succeed"))

	for _, image := range imageList {
		size, _ := strconv.Atoi(image.Size)
		imageMeta := model.ImageMetadata{
			Name:       image.Name,
			Tag:        image.Tag,
			Size:       int64(size),
			AzId:       config.IMConfig.TargetAzId,
			Status:     2, // 2:offline
			SyncStatus: 3, // 3:已同步回中控
		}
		if config.IMConfig.TargetAzId == centralAz {
			imageMeta.Status = 1 // 1:online
		}
		has, err := dao.MySQL().Where("name = ?", imageMeta.Name).And("tag = ?", imageMeta.Tag).Get(new(model.ImageMetadata))
		if err != nil {
			glog.Error("get image meta failed", glog.String("error", err.Error()), glog.String("image", image.Name+":"+image.Tag))
		}
		if has {
			glog.Infof("image meta already exists", glog.String("image", image.Name+":"+image.Tag))
			continue
		}
		_, err = dao.MySQL().Insert(&imageMeta)
		if err != nil {
			glog.Error("insert image meta failed", glog.String("error", err.Error()), glog.String("image", image.Name+":"+image.Tag))
		}
	}

	if config.IMConfig.TargetAzId == centralAz {
		for _, image := range imageList {
			imageMeta := model.ImageMetadata{
				Name:       image.Name,
				Tag:        image.Tag,
				SyncStatus: 3, // 3:已同步回中控
			}
			_, err := dao.MySQL().Cols("sync_status").
				Where("name = ?", imageMeta.Name).
				And("tag = ?", imageMeta.Tag).
				And("az_id = ?", config.IMConfig.SourceAzId).Update(imageMeta)
			if err != nil {
				glog.Error("update image meta failed", glog.String("error", err.Error()), glog.String("image", image.Name+":"+image.Tag))
			}
		}
	}

}
