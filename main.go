package main

import (
	"flag"
	"fmt"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"image-sync/imagesync"
	"time"
)

var (
	sourceRegistryAddr = flag.String("sourceRegistryAddr", "cluster2-test3.virtaicloud.com:32402", "")
	targetRegistryAddr = flag.String("targetRegistryAddr", "10.12.101.13:32402", "")
	syncerPath         = flag.String("syncerPath", "./image-syncer", "The path of the image-syncer")
	outputBasePath     = flag.String("output", "./output", "The base path of the output configFile")
	auth               = flag.String("config", "./auth.yaml", "The path of the auth configFile")
	imageListPath      = flag.String("imageList", "./example2.xlsx", "The path of the imageList,is a xlsx file")
)

func init() {
	flag.Parse()
}

func main() {
	startTime := time.Now()
	fmt.Println("start time:", startTime)
	sm := imagesync.NewThirdPkgSyncImageManager(*sourceRegistryAddr, *targetRegistryAddr, *syncerPath, *outputBasePath, *auth, 1)
	imageList, err := sm.PreHandleData(*imageListPath)
	if err != nil {
		glog.Error("pre sync failed", glog.String("error", err.Error()))
		return
	}
	sm.Sync(imageList)
	endTime := time.Now()
	fmt.Println("end time:", endTime)
	fmt.Printf("cost time:%v,sync totalSize:%v MB\n", endTime.Sub(startTime), imagesync.SyncSize<<20)
	costTimeSec := endTime.Sub(startTime).Seconds()
	fmt.Println("迁移速度", costTimeSec/float64(imagesync.SyncSize<<20))
}
