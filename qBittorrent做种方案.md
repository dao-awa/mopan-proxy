# qBittorrent 云盘做种方案

## 概述

利用 MopanProxy + rclone + qBittorrent 实现：
- 下载直接存储到和彩云（中国移动云盘）
- 做种从本地缓存读取
- 不占用服务器本地存储

## 架构

```
BT 种子下载
    ↓
qBittorrent (/mnt/yidong/qb)
    ↓
rclone mount (2GB VFS 缓存)
    ↓
MopanProxy WebDAV (端口 9091)
    ↓
和彩云（云端加密存储）
```

## 配置详情

### 1. qBittorrent

- Web UI: https://qb.96110.li
- 用户名: 1552481797
- 密码: 1552481797
- 下载路径: `/mnt/yidong/qb`

### 2. rclone 缓存配置

```ini
[yidong]
type = webdav
url = http://127.0.0.1:9091/dav/
vfs_cache_mode = full
vfs_cache_max_size = 2147483648    # 2GB
vfs_cache_max_age = 168h           # 7天
vfs_read_chunk_size = 16M
vfs_read_chunk_size_limit = 512M
dir_cache_time = 5m
```

### 3. 工作原理

#### 下载流程
1. qBittorrent 添加种子，保存到 `/mnt/yidong/qb`
2. rclone 拦截写入，数据先写入本地缓存 (`~/.cache/rclone/`)
3. 后台异步上传到和彩云
4. 上传完成后缓存可清理（但保留用于做种）

#### 做种流程
1. BT 客户端请求数据块
2. rclone 先检查本地缓存
3. 缓存命中 → 直接读取返回（速度快）
4. 缓存未命中 → 从云端下载到缓存 → 返回（速度慢）

## 优势

### 1. 存储在云端
- 文件存储在和彩云，不占用本地磁盘
- 本地只有 2GB 缓存空间

### 2. 做种不依赖网络
- 7天内做种的数据都在本地缓存
- 做种速度接近本地磁盘速度
- 超过 7 天的旧数据做种需要重新从云端下载

### 3. 加密保护
- 所有文件名和内容都经过 AES-256-GCM 加密
- 云端无法读取任何信息

## 注意事项

### 1. 缓存空间
- 2GB 缓存最多同时做种约 2-3 个中等大小的种子
- 添加新种子时会自动淘汰旧缓存
- 如果需要更多做种数量，可以增加 `vfs_cache_max_size`

### 2. 做种策略
- 新添加的种子：做种速度快（缓存命中）
- 老种子（超过7天）：做种速度慢（需要重新下载）
- 建议：批量做种时按时间排序，先做新的

### 3. 网络要求
- 上传带宽：取决于和彩云的上传速度
- 下载带宽：做种时如果缓存未命中需要重新下载

### 4. Token 有效期
- 和彩云 Token 约 30 天过期
- 过期后 rclone 无法访问云端，需要重新获取 Token
- Token 过期不会影响已有缓存的做种

## 使用建议

### 1. 添加种子
1. 登录 qBittorrent Web UI
2. 点击 "添加 Torrent 链接" 或上传 .torrent 文件
3. 确保保存路径是 `/mnt/yidong/qb`
4. 点击 "下载"

### 2. 查看缓存状态
```bash
# 查看缓存大小
du -sh ~/.cache/rclone/

# 查看挂载状态
df -h /mnt/yidong
mount | grep yidong
```

### 3. 手动清理缓存（如果需要）
```bash
# 停止 rclone
fusermount -u /mnt/yidong

# 清理缓存（可选）
rm -rf ~/.cache/rclone/yidong/

# 重新挂载
rclone mount yidong: /mnt/yidong --daemon --allow-other
```

### 4. 重启服务
```bash
# 重启 qBittorrent
systemctl restart qbittorrent

# 重启 rclone（如果挂载异常）
fusermount -u /mnt/yidong
rclone mount yidong: /mnt/yidong --daemon --allow-other
```

## 常见问题

### Q: 下载速度很慢？
A: 检查 rclone 缓存状态，新种子首次下载需要同时写入缓存和上传到云端

### Q: 做种速度很慢？
A: 如果是老种子（超过7天），数据不在缓存中，需要从云端重新下载

### Q: 下载完成后找不到文件？
A: 文件在云端，查看 Web UI 或通过 rclone 挂载目录查看

### Q: 如何增加做种数量？
A: 增加 `vfs_cache_max_size`，但会占用更多服务器磁盘
