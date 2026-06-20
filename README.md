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

### 2. 配置

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml，设置加密密码和和彩云 Token
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

打开 http://localhost:9090，粘贴 Token

### 6. 映射网络驱动器

```
net use Z: \\服务器IP@dav\dav /user:admin your_password
```

或使用 RaiDrive / Cyberduck 等客户端

## 配置

详见 `config.example.yaml` 和 `项目文档.md`

关键配置项：

```yaml
crypto:
  enabled: true
  password: "your-encryption-password"  # ⚠️ 务必修改！

storage:
  root_folder: "/MopanProxy"  # 和彩云专用根目录
```

## 文档

| 文档 | 说明 |
|------|------|
| [项目文档.md](项目文档.md) | 完整项目文档（架构、API、部署、安全） |
| [ENCRYPTION.md](ENCRYPTION.md) | 加密系统详解 |
| [进度文档.md](进度文档.md) | 项目进度和架构 |

## 技术栈

- **语言**: Go 1.25
- **加密**: AES-256-GCM + PBKDF2-SHA256
- **数据库**: SQLite (modernc.org/sqlite)
- **WebDAV**: golang.org/x/net/webdav
- **部署**: systemd + nginx + Cloudflare

## 安全

- 文件名 + 文件内容 AES-256-GCM 加密
- PBKDF2-SHA256 密钥派生（100K 迭代）
- CSRF 保护（X-Requested-With 头）
- HSTS + CSP + X-Frame-Options 安全头
- 配置文件权限 0600
- 优雅关闭（HTTP + SQLite）

详见 [安全说明](项目文档.md#十一安全机制)

## 项目结构

```
mopan-proxy/
├── cmd/mopan-proxy/main.go        # 入口 + 优雅关闭
├── internal/
│   ├── config/config.go           # YAML 配置
│   ├── crypto/aes256gcm.go        # 内容加密 + 名称加密
│   ├── metadata/db.go             # SQLite 元数据
│   ├── mopan/client.go            # 和彩云 API 客户端
│   ├── server/server.go           # HTTP + 认证 + 安全
│   └── server/templates.go        # Web UI
│   └── webdav/webdav.go           # WebDAV FileSystem
├── config.example.yaml            # 配置模板
├── go.mod
└── README.md
```

## 许可证

MIT
