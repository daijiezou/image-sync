package main

import (
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-geminidb/model"
	"image-sync/config"
	"image-sync/dao"
	"image-sync/imagesync"
	"path"
	"strconv"
)

func UpdateImageMeta() {
	imageList := imagesync.GetSyncSucceedImageList(path.Join(config.IMConfig.OutputPath, "sync-succeed"))
	if config.IMConfig.TargetAzId == "" {
		glog.Error("target az id is empty")
		return
	}
	for _, image := range imageList {
		size, _ := strconv.Atoi(image.Size)
		imageMeta := model.ImageMetadata{
			Name:       image.Name,
			Tag:        image.Tag,
			Size:       int64(size),
			AzId:       config.IMConfig.TargetAzId,
			Status:     1,
			SyncStatus: 3,
		}
		_, err := dao.MySQL().Insert(&imageMeta)
		if err != nil {
			glog.Error("insert image meta failed", glog.String("error", err.Error()), glog.String("image", image.Name+":"+image.Tag))
		}
	}
}
