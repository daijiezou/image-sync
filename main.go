package main

import (
	"flag"
	"fmt"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"image-sync/config"
	"image-sync/dao"
	"image-sync/imagesync"
	"os"
	"path"
	"time"
)

var (
	syncerPath = flag.String("syncerPath", "./image-syncer", "The path of the image-syncer")
	auth       = flag.String("auth", "./auth.yaml", "The path of the auth configFile")
	configFile = flag.String("config", "./config.yaml", "The path of the auth configFile")
)

func init() {
	flag.Parse()
	config.ParseConfig("image-migration", *configFile)
	glog.Infow("parse config succeed", "config", config.IMConfig)

	err := dao.InitMySQL(config.IMConfig.DbDsn)
	glog.InfoFatalw(err, "init MySQL")

	if !isExist(path.Join(config.IMConfig.OutputPath, "sync-succeed")) {
		os.Create(path.Join(config.IMConfig.OutputPath, "sync-succeed"))
	}

	if isExist(path.Join(config.IMConfig.OutputPath, "sync-failed")) {
		os.Remove(path.Join(config.IMConfig.OutputPath, "sync-failed"))
	}
	os.Create(path.Join(config.IMConfig.OutputPath, "sync-failed"))
}

func main() {
	switch config.IMConfig.Mode {
	case "dryRun":
		sm := imagesync.NewSyncImageManager(*syncerPath, *auth)

		imageList, err := sm.GetNeedSyncImageMetaList()
		if err != nil {
			glog.Errorf("pre sync failed,err:%+v", err)
			return
		}

		for _, image := range imageList {
			fmt.Println(image)
		}
	case "sync":
		startTime := time.Now()
		fmt.Println("start time:", startTime)

		sm := imagesync.NewSyncImageManager(*syncerPath, *auth)
		imageList, err := sm.GetNeedSyncImageMetaList()
		if err != nil {
			glog.Errorf("pre sync failed,err:%+v", err)
			return
		}

		sm.Sync(imageList)

		endTime := time.Now()
		fmt.Println("end time:", endTime)
		fmt.Printf("cost time:%v,sync totalSize:%v GB\n", endTime.Sub(startTime), imagesync.SyncSize>>30)
		costTimeSec := endTime.Sub(startTime).Seconds()
		fmt.Printf("sync speed:%.2f MB/s\n", float64(imagesync.SyncSize>>20)/costTimeSec)
	case "update":
		UpdateImageMeta()
	default:
		glog.Errorf("unsupported mode,:%s", config.IMConfig.Mode)
	}
}

// 判断文件或文件夹是否存在
func isExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		if os.IsNotExist(err) {
			return false
		}
		fmt.Println(err)
		return false
	}
	return true
}
