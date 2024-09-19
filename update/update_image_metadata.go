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
		_, err := dao.MySQL().Insert(&imageMeta)
		if err != nil {
			glog.Error("insert image meta failed", glog.String("error", err.Error()), glog.String("image", image.Name+":"+image.Tag))
		}
	}
}
