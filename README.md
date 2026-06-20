# MopanProxy

和彩云（中国移动云盘 yun.139.com）**加密代理服务器**

提供 Web UI 文件管理 + WebDAV 网络驱动器 + **AES-256-GCM 端到端加密**

> 文件名、文件夹名、文件内容全部加密，云端无法辨别任何信息

## 功能

- **文件内容加密** — AES-256-GCM 认证加密
- **名称完全加密** — 文件名在云端显示为 `E_` + 不可读 hex
- **文件类型隐藏** — 所有文件后缀统一为 `.bin`
- **Web UI** — 浏览器登录、文件浏览/上传/下载/删除/新建文件夹
- **WebDAV 服务器** — 支持 Windows/macOS/Linux 映射为网络驱动器
- **Web UI 登录保护** — 用户名/密码认证，Session Cookie，防未授权访问
- **安全认证** — 全局认证中间件 + CSRF 保护 + 安全响应头

## 效果

| 位置 | 显示 |
|------|------|
| **云端（和彩云）** | `E_b28ce2c86ef5804d.../E_50db1161a60377ce...bin` |
| **用户（MopanProxy）** | `Documents/report.pdf` |

## 快速开始

### 1. 编译

```bash
go build -o mopan-proxy ./cmd/mopan-proxy/
```

交叉编译（Linux）：

```bash
GOOS=linux GOARCH=amd64 go build -o mopan-proxy-linux ./cmd/mopan-proxy/
```

### 2. 配置

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml，设置加密密码、和彩云 Token、登录凭据
```

### 3. 启动

```bash
./mopan-proxy -config config.yaml
```

### 4. 获取和彩云 Token

1. 浏览器打开 https://yun.139.com/w/
2. 手机号 + 短信验证码登录
3. F12 → Network → 复制任意请求的 `Authorization` 头

### 5. 登录 Web UI

打开浏览器访问服务器地址，输入用户名和密码登录。

## 配置

```yaml
# 服务器
port: 9090
host: "0.0.0.0"

# 和彩云认证（从浏览器 F12 获取）
authorization: "Basic base64(pc:手机号:token)"

# Web UI 登录保护（默认开启）
auth:
  enabled: true
  username: "admin"
  password: "admin123"    # ⚠️ 请修改！

# WebDAV
webdav:
  enabled: true
  port: 9091
  username: "admin"
  password: "admin123"    # ⚠️ 请修改！

# 加密（AES-256-GCM）
crypto:
  enabled: true
  password: "your-password"  # ⚠️ 务必修改！丢失此密码无法解密
  salt_hex: ""               # 自动生成
  iterations: 100000

# 存储
storage:
  cache_dir: "./data/cache"
  meta_path: "./data/metadata.db"
  max_upload: 10737418240    # 10GB
  root_folder: "/MopanProxy"
```

详见 `config.example.yaml`。

## 部署

### Systemd 服务

```ini
[Unit]
Description=MopanProxy - 和彩云加密代理
After=network.target

[Service]
Type=simple
ExecStart=/opt/mopan-proxy/mopan-proxy -config /opt/mopan-proxy/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Nginx 反向代理（含 SSL）

```nginx
server {
    listen 443 ssl http2;
    server_name webdav.96110.li;

    ssl_certificate     /etc/nginx/certs/webdav.96110.li.pem;
    ssl_certificate_key /etc/nginx/certs/webdav.96110.li.key;

    client_max_body_size 10g;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;
    proxy_buffering off;

    location /dav/ {
        proxy_pass http://127.0.0.1:9091;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass_header Authorization;
    }

    location / {
        proxy_pass http://127.0.0.1:9090;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name webdav.96110.li;
    return 301 https://$host$request_uri;
}
```

### WebDAV 挂载到本地

使用 rclone 挂载为本地目录：

```bash
# 安装 rclone
apt install rclone

# 配置
cat > ~/.config/rclone/rclone.conf << 'EOF'
[yidong]
type = webdav
url = https://webdav.96110.li/dav/
vendor = other
user = admin
pass = $(rclone obscure "your_password")
EOF

# 挂载
rclone mount yidong: /mnt/yidong --daemon --vfs-cache-mode full --allow-other
```

持久化挂载（开机自启）：

```bash
# 添加到 /etc/rc.local
/usr/bin/rclone mount yidong: /mnt/yidong --daemon --allow-other 2>/dev/null
```

## API

| 路由 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/api/login` | POST | 无 | `{username, password}` → session cookie |
| `/api/logout` | POST | 无 | 清除 session |
| `/api/status` | GET | 无 | 返回认证状态、账号、加密状态 |
| `/api/set-token` | POST | ✅ | 设置和彩云 Authorization Token |
| `/api/files` | GET | ✅ | `?path=/docs&cursor=` 列出文件 |
| `/api/upload` | POST | ✅ | 上传文件（支持加密） |
| `/api/download/{id}` | GET | ✅ | 下载/解密文件 |
| `/api/delete` | POST | ✅ | 批量删除 |
| `/api/mkdir` | POST | ✅ | 新建文件夹 |
| `/api/settings` | GET | ✅ | 获取设置 |
| `/api/settings/update` | POST | ✅ | 更新 WebDAV 设置 |
| `/api/stats` | GET | ✅ | 加密统计 |

## 安全

- **登录保护** — 用户名/密码 + Session Cookie（7 天有效期）
- **文件加密** — AES-256-GCM 认证加密 + PBKDF2-SHA256 密钥派生（100K 迭代）
- **名称加密** — 文件名在云端完全不可读
- **CSRF 保护** — POST 请求要求 `X-Requested-With` 头 + Origin/Referer 校验
- **安全响应头** — HSTS + CSP + X-Frame-Options + X-Content-Type-Options
- **配置文件权限** — 0600（仅所有者可读写）
- **优雅关闭** — HTTP + SQLite 安全关闭

## 项目结构

```
mopan-proxy/
├── cmd/mopan-proxy/main.go        # 入口 + 优雅关闭
├── internal/
│   ├── config/config.go           # YAML 配置（含 Auth）
│   ├── crypto/aes256gcm.go        # 内容加密 + 名称加密
│   ├── metadata/db.go             # SQLite 元数据
│   ├── mopan/client.go            # 和彩云 API 客户端
│   ├── server/server.go           # HTTP + 登录认证 + 安全
│   ├── server/templates.go        # Web UI（登录页 + 管理页）
│   └── webdav/webdav.go           # WebDAV FileSystem
├── config.example.yaml            # 配置模板
├── go.mod
└── README.md
```

## 技术栈

- **语言**: Go 1.25
- **加密**: AES-256-GCM + PBKDF2-SHA256
- **数据库**: SQLite (modernc.org/sqlite)
- **WebDAV**: golang.org/x/net/webdav
- **部署**: systemd + nginx + Cloudflare

## 许可证

MIT
