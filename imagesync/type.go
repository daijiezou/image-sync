package imagesync

import "time"

type DataImage struct {
	ID         string `json:"image_id" xorm:"'image_id'"`
	Name       string `json:"image_name"  xorm:"'image_name'"`
	Tag        string `json:"image_tag"  xorm:"'image_tag'"`
	Size       string `json:"image_size"  xorm:"'image_size'"`
	Status     int    //1:同步成功 2:同步失败
	CreateTime time.Time
}

const (
	SyncSucceed = 1
	SyncFailed  = 2
)
