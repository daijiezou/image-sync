package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image-sync/config"
	"image-sync/mysql"
	"io"
	"strings"
	"sync"

	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-gemini/glog"
	"gitlab.yellow.virtaitech.com/gemini-platform/public-geminidb/model"
)

const (
	SyncSucceed = 1
	SyncFailed  = 2
)

var (
	pullImageChan = make(chan model.DataImage)
	pushImageChan = make(chan model.DataImage)
	exitChan      = make(chan struct{})
	configFile    = flag.String("config", "./config/config.yaml", "The path of the configFile")
	lock          sync.Mutex
)

func init() {
	config.ParseConfig("image-sync", *configFile)
	glog.InfoFatalw(mysql.InitMySQL(config.Config.DbDsn), "InitMySQL")
	mysql.CreateTable()
}

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}
	// 1.获取镜像列表
	res, count, err := mysql.GetImageList(config.Config.StartImageId, config.Config.EndImageId)
	if err != nil {
		panic(err)
	}
	loopCount := int(count)
	glog.Infof("image count:%d", count)
	pullImageChan = make(chan model.DataImage, count)
	pushImageChan = make(chan model.DataImage, count)
	go func() {
		for {
			select {
			case dataImage := <-pullImageChan:
				go func(dataImage model.DataImage) {
					err := PushImage(context.Background(), RemoteImageName(dataImage),
						config.Config.RemoteRegistry,
						config.Config.RegistryUsername,
						config.Config.RegistryPassword, cli)

					syncImage := mysql.SyncDataImage{
						ImageId:    dataImage.ImageId,
						ImageName:  dataImage.ImageName,
						ImageTag:   dataImage.ImageTag,
						ImageSize:  dataImage.ImageSize,
						CreateTime: time.Now(),
						Status:     SyncSucceed,
					}
					if err != nil {
						glog.Errorf("push image failed:err:%s", err.Error())
						syncImage.Status = SyncFailed
						syncImage.Errmsg = fmt.Sprintf("push image failed:err:%s", err.Error())

					}
					mysql.SyncedImage(syncImage)
					pushImageChan <- dataImage
				}(dataImage)
			}
		}
	}()

	go func() {
		for {
			select {
			case dataImage := <-pushImageChan:
				// 这里删除的顺序是固定的
				err := DeleteTmpFile(OriginImageName(dataImage), RemoteImageName(dataImage), cli)
				if err != nil {
					glog.Errorf("delete image failed:err:%v", err)
				}
				lock.Lock()
				count--
				lock.Unlock()
				glog.Infof("pushing image count:%d", count)
				if count == 0 {
					exitChan <- struct{}{}
				}
			}
		}
	}()

	// todo: 这里可以多个协程一起

	for i := 0; i < loopCount; i++ {
		err := PullImage(context.Background(), OriginImageName(res[i]), config.Config.RemoteRegistry,
			config.Config.LocalRegistryUsername,
			config.Config.LocalRegistryPassword, cli)
		if err != nil {
			glog.Errorf("pull image failed:err:%s", err.Error())
			syncImage := mysql.SyncDataImage{
				ImageId:    res[i].ImageId,
				ImageName:  res[i].ImageName,
				ImageTag:   res[i].ImageTag,
				ImageSize:  res[i].ImageSize,
				CreateTime: time.Now(),
				Status:     SyncFailed,
				Errmsg:     fmt.Sprintf("pull image failed:err:%s", err.Error()),
			}
			lock.Lock()
			count--
			lock.Unlock()
			mysql.SyncedImage(syncImage)
			continue
		}
		TagImage(context.Background(), OriginImageName(res[i]), RemoteImageName(res[i]), cli)
		pullImageChan <- res[i]
	}

	for {
		select {
		case <-exitChan:
			fmt.Println("推送完成退出了")
			return
		default:
		}
	}
}

func OriginImageName(image model.DataImage) string {
	return config.Config.LocalRegistry + "/" + image.ImageName + ":" + image.ImageTag
}

func RemoteImageName(image model.DataImage) string {
	return config.Config.RemoteRegistry + "/" + config.Config.Namespace + "/" + image.ImageName + ":" + image.ImageTag
}

func TagImage(ctx context.Context, originImage, targerImage string, cli *client.Client) (err error) {
	glog.Infof("image tag originImage:%s,targerImage:%s", originImage, targerImage)
	err = cli.ImageTag(ctx, originImage, targerImage)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil

}

func PullImage(ctx context.Context, originImage, registryServer, registryUsername, registryPassword string, cli *client.Client) error {
	glog.Infow("pull image start", "imageName", originImage)
	authConfigForPull := types.AuthConfig{
		Username:      registryUsername,
		Password:      registryPassword,
		ServerAddress: registryServer,
	}
	encodedJSONForPull, err := json.Marshal(authConfigForPull)
	if err != nil {
		return errors.WithStack(err)
	}
	authStrForPull := base64.URLEncoding.EncodeToString(encodedJSONForPull)
	pullOut, err := cli.ImagePull(ctx, originImage, types.ImagePullOptions{RegistryAuth: authStrForPull})
	if err != nil {
		return errors.Wrapf(err, "image pull failed")
	}
	defer pullOut.Close()
	br := bufio.NewReader(pullOut)
	for {
		line, _, err := br.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.WithStack(err)
		}
		// fmt.Println(string(line))
		if strings.Contains(string(line), "error") {
			return errors.Wrapf(err, "image pull failed")
		}
	}
	glog.Infow("pull image succeed", "imageName", originImage)
	return nil
}

func PushImage(ctx context.Context, dstFullImage, registryServer, registryUsername, registryPassword string, cli *client.Client) error {
	glog.Infow("push image start", "imageName", dstFullImage)
	//push image 使用内置的用于push镜像用户名
	authConfigForPush := types.AuthConfig{
		Username:      registryUsername,
		Password:      registryPassword,
		ServerAddress: registryServer,
	}
	encodedJSONForPush, err := json.Marshal(authConfigForPush)
	if err != nil {
		return errors.WithStack(err)
	}
	authStrForPush := base64.URLEncoding.EncodeToString(encodedJSONForPush)
	pushOut, err := cli.ImagePush(ctx, dstFullImage, types.ImagePushOptions{RegistryAuth: authStrForPush})
	if err != nil {
		errors.Wrapf(err, "image push failed")
	}
	defer pushOut.Close()
	br := bufio.NewReader(pushOut)
	for {
		line, _, err := br.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.WithStack(err)
		}
		// fmt.Println(string(line))
		if strings.Contains(string(line), "error") {
			errors.Wrapf(err, "image push failed")
		}
	}
	glog.Infow("push image succeed", "imageName", dstFullImage)
	return nil
}

func DeleteTmpFile(originImage, tagImageName string, cli *client.Client) error {
	filters1 := filters.NewArgs()
	filters1.Add("reference", originImage)
	images, err := cli.ImageList(context.Background(), types.ImageListOptions{
		All:     true,
		Filters: filters1,
	})

	if err != nil {
		return errors.WithMessage(err, "filter imageList failed")
	}
	if len(images) != 0 {
		//build镜像删除
		_, err = cli.ImageRemove(context.Background(), images[0].ID, types.ImageRemoveOptions{Force: true, PruneChildren: true})
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("remove Image failed,image:%s", originImage))
		}
		glog.Infof("delete tmp file succeed:%s", originImage)
	}

	// 通过镜像Name获取镜像ID
	filters2 := filters.NewArgs()
	filters2.Add("reference", tagImageName)
	images, err = cli.ImageList(context.Background(), types.ImageListOptions{
		All:     true,
		Filters: filters2,
	})
	if err != nil {
		return errors.WithMessage(err, "filter imageList failed")
	}
	if len(images) != 0 {
		//build镜像删除
		_, err = cli.ImageRemove(context.Background(), images[0].ID, types.ImageRemoveOptions{Force: true, PruneChildren: true})
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("remove Image failed,image:%s", tagImageName))
		}
		glog.Infof("delete tmp file succeed:%s", tagImageName)
	}

	return nil
}
