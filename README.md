# image-sync

# 使用方法说明

1. 创建一个记录迁移日志的文件
 - `touch sync.log`
2. 开始迁移
   - `./image-migration --auth ./auth.yaml --config ./config.yaml --syncerPath ./image-syncer >> sync.log `