package imagesync

import "time"

type DataImage struct {
	ID         string `json:"image_id"`
	Name       string `json:"image_name"`
	Tag        string `json:"image_tag"`
	Size       string `json:"image_size"`
	Status     int    //1:同步成功 2:同步失败
	CreateTime time.Time
}

const (
	SyncSucceed = 1
	SyncFailed  = 2
)
