package main

import (
	"flag"
	"fmt"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"image-sync/config"
	"image-sync/dao"
	"image-sync/imagesync"
	"time"
)

var (
	syncerPath = flag.String("syncerPath", "./image-syncer", "The path of the image-syncer")
	auth       = flag.String("auth", "./auth.yaml", "The path of the auth configFile")
	configFile = flag.String("config", "./config.yaml", "The path of the auth configFile")
)

func init() {
	flag.Parse()
	config.ParseConfig("image-sync", *configFile)
	glog.Infow("parse config succeed", "config", config.IMConfig)
	if config.IMConfig.DbDsn != "" {
		dao.InitMySQL(config.IMConfig.DbDsn)
	}
}

func main() {
	startTime := time.Now()
	fmt.Println("start time:", startTime)
	sm := imagesync.NewThirdPkgSyncImageManager(*syncerPath, *auth)
	imageList, err := sm.GetNeedSyncImageMetaList()
	if err != nil {
		glog.Errorf("pre sync failed,err:%+v", err)
		return
	}
	sm.Sync(imageList)
	endTime := time.Now()
	fmt.Println("end time:", endTime)
	fmt.Printf("cost time:%v,sync totalSize:%v MB\n", endTime.Sub(startTime), imagesync.SyncSize>>20)
	costTimeSec := endTime.Sub(startTime).Seconds()
	fmt.Println("迁移速度", costTimeSec/float64(imagesync.SyncSize>>20))
}
