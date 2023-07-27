package mysql

import (
	"sync"
	"time"

	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-geminidb/model"
)

var lock sync.Mutex

func SyncedImage(image SyncDataImage) {
	lock.Lock()
	defer lock.Unlock()
	_, err := MySQL().Table("sync_data_image").Insert(&image)
	if err != nil {
		glog.Errorf("insert image error:%s", err.Error())
		return
	}
	glog.Infof("insert image succeed:%s", image.ImageName)
	return
}

type SyncDataImage struct {
	ImageId    int64     `json:"image_id" xorm:"not null pk autoincr BIGINT"`
	ImageName  string    `json:"image_name" xorm:"not null VARCHAR(128)"`
	ImageTag   string    `json:"image_tag" xorm:"default ''  VARCHAR(128)"`
	ImageSize  int64     `json:"image_size" xorm:"not null default 0 BIGINT"` // 单位:B
	Status     int32     `json:"status" xorm:"not null default 0 TINYINT"`    //1:成功 2:失败
	Errmsg     string    `json:"errmsg" xorm:"not null default '' VARCHAR(128)"`
	CreateTime time.Time `json:"create_time" xorm:"not null DATETIME"`
}

func (m *SyncDataImage) TableName() string {
	return "sync_data_image"
}

func CreateTable() {
	err := mysqlClient.Sync2(SyncDataImage{})
	if err != nil {
		glog.Fatal("Sync2 DataCode error", glog.String("msg", err.Error()))
	}
}

func GetImageList(startId, endId int) (res []model.DataImage, conut int64, err error) {
	var imagelist []model.DataImage
	seesion := MySQL().Select("image_id, image_name, image_tag,image_size")
	if startId != 0 {
		seesion.Where("image_id > ?", startId)
	}
	if endId != 0 {
		seesion.Where("image_id < ?", endId)
	}
	count, err := seesion.FindAndCount(&imagelist)
	if err != nil {
		glog.Warnf("get image list error:%s", err.Error())
		return nil, 0, err
	}
	return imagelist, count, nil
}
