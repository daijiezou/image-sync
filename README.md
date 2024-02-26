# image-sync

# 使用方法说明
0. 填写配置文件
**config.yaml**
```
image-migration:
   dbDsn: "root:xxxx@tcp(10.12.101.14:33306)/geminidev?charset=utf8" # 数据库连接方式
   startTime: 2023-09-01 00:00:00 #镜像的最早使用时间，只会迁移在此时间之后使用过的镜像
   endTime: 2023-09-10 00:00:00 #镜像的最晚使用时间，只会迁移在此时间之前使用过的镜像
   sourceRegistryAddr: 10.12.101.14:32402
   targetRegistryAddr: 10.12.101.13:32402
   sourceAzId: "az1"
   targetAzId: "az2"
   outputPath: /data/output
   proc: 3
   mode: sync #sync:同步镜像 update:更改镜像元数据 dryRun:输出此次会同步的镜像
```

**auth.yaml**
```
10.12.101.14:32402:32402:
  username: xxxx   #registry 用户名，没有可不填
  password: xxxx   #registry 密码，没有可不填
  insecure: true
10.12.101.13:32402:32402:
  username: xxx
  password: xxxx
  insecure: true
```

1. 创建一个记录迁移日志的文件
 - `touch sync.log`
2. 开始迁移
   - `./image-migration --auth ./auth.yaml --config ./config.yaml --syncerPath ./image-syncer >> sync.log `