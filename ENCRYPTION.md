# MopanProxy 加密功能文档

## 概述

MopanProxy 实现了 **AES-256-GCM 端到端加密**，包括：

1. **文件内容加密** — 文件在上传前加密，下载后解密
2. **文件名加密** — 文件名和文件夹名在云端完全不可辨别
3. **文件类型隐藏** — 所有加密文件后缀统一为 `.bin`

**效果：** 任何人直接访问和彩云，只能看到不可读的 `E_` 前缀 hex 字符串，无法知道原始文件名、文件夹名或文件类型。

---

## 加密架构

```
用户 → [WebUI/WebDAV] → MopanProxy
                            ↓
                    ┌───────┴───────┐
                    │  名称加密       │  "report.pdf" → "E_50db1161...bin"
                    │  内容加密       │  "Hello" → [nonce][密文][tag]
                    └───────┬───────┘
                            ↓
                    和彩云 API → COS 存储
                            ↓
                    SQLite 元数据 (本地)
                    E_50db1161...bin → report.pdf
```

---

## 文件内容加密

### 算法

| 项目 | 值 |
|------|-----|
| 算法 | AES-256-GCM（认证加密） |
| 密钥派生 | PBKDF2-SHA256 |
| 迭代次数 | 100,000 |
| 密钥长度 | 32 字节 (AES-256) |
| Nonce | 12 字节随机数 |
| GCM Tag | 16 字节认证标签 |
| 总开销 | 28 字节/文件 |

### 加密格式

```
┌────────────────────────────────────────────┐
│ 12B Nonce │     Ciphertext     │ 16B GCM Tag │
│ (随机)    │ (AES-256-GCM 加密)  │ (认证标签)   │
└────────────────────────────────────────────┘
```

### 密钥派生

```
password (用户密码)
    ↓
PBKDF2-SHA256(password, salt, 100000, 32)
    ↓
32 字节 AES-256 密钥
```

- Salt 16 字节，首次启动自动生成
- 密钥不存储，从密码派生
- 密码和 salt 保存在 config.yaml（权限 0600）

---

## 名称加密（核心特性）

### 为什么需要名称加密？

传统的加密方案只加密文件内容，文件名仍然可见：
```
云端看到: Documents/report.pdf.enc   ← 还能看出是 PDF 文档
```

MopanProxy 加密文件名本身：
```
云端看到: E_b28ce2c8.../E_50db1161...bin  ← 完全无法辨别
```

### 加密规则

| 类型 | 原始名 | 云端名格式 | 示例 |
|------|--------|-----------|------|
| 文件夹 | `Documents` | `E_` + hex | `E_b28ce2c86ef5804d326dd8fbfe5ffe72...` |
| 文件 | `report.pdf` | `E_` + hex + `.bin` | `E_50db1161a60377ce...a24f.bin` |

### 加密算法

```go
// 1. 从名称派生确定性 nonce
nonce = SHA256(name + "_name_nonce")[:12]

// 2. AES-256-GCM 加密名称
ciphertext = GCM.Seal(nonce, plaintext=name)

// 3. 拼接并编码
encrypted = hex(nonce + ciphertext + tag)

// 4. 添加前缀
folderResult = "E_" + encrypted           // 文件夹
fileResult   = "E_" + encrypted + ".bin"  // 文件
```

**确定性 nonce**：相同名称总是产生相同密文，便于查找和匹配。

### 云端目录结构示例

```
和彩云
└── MopanProxy/
    ├── E_b28ce2c86ef5804d326dd8fbfe5ffe726d8d51250b5d70077a2055ebd258440d23f7d5c2f9a2
    │   └── E_2566249bcadbf4ee9520d3eacfe2aef5cd363ff6ab22d87cacf09e6f2d420d7764e1219e.bin
    ├── E_50db1161a60377cec9ee89d132dde32fee5dfa9932ad88d13eaa0ed5d5c1d9cca5c0b753a24f.bin
    └── Documents_enc/              ← 旧版兼容
```

用户通过 MopanProxy 看到的：
```
/MopanProxy/
├── SecretDocs/
│   └── conf.txt
├── secret.txt
└── Documents/
```

### 名称还原材料

```
云端名 → SQLite 查 orig_name → 返回给用户

E_50db1161...bin → SELECT orig_name WHERE cloud_name='E_50db1161...bin' → "secret.txt"
E_b28ce2c8...    → SELECT orig_name WHERE cloud_name='E_b28ce2c8...'    → "SecretDocs"
Documents_enc    → 旧版兼容（去掉 _enc）                               → "Documents"
report.pdf.enc   → 旧版兼容（去掉 .enc）                               → "report.pdf"
```

### 旧版兼容

名称加密实现后，旧版加密名称仍可识别和访问：

| 旧版格式 | 识别方式 |
|----------|---------|
| `Documents_enc` | `_enc` 后缀 |
| `report.pdf.enc` | `.enc` 后缀 |

新上传的文件/文件夹使用 `E_` 前缀格式。

---

## 完整加密流程

### 上传

```
原始文件: report.pdf (1024 bytes)
    ↓
名称加密: "report.pdf" → "E_50db1161...a24f.bin"
    ↓
内容加密: [1024 bytes] → [12B nonce][密文][16B tag] = 1052 bytes
    ↓
计算 SHA256(密文) → contentHash
    ↓
POST /hcy/file/create (name:"E_xxx.bin", hash:密文hash)
    ↓
PUT uploadUrl (密文数据)
    ↓
POST /hcy/file/complete (hash:密文hash)
    ↓
INSERT INTO SQLite (cloud_name:"E_xxx.bin", orig_name:"report.pdf")
```

### 下载

```
用户请求: report.pdf
    ↓
SQLite: report.pdf → cloud_file_id → E_50db1161...a24f.bin
    ↓
POST /hcy/file/getDownloadUrl → CDN URL
    ↓
GET CDN URL → 密文数据
    ↓
AES-256-GCM 解密 → 明文
    ↓
返回: report.pdf (1024 bytes)
```

### WebDAV 读写

```
写入 (PUT /dav/secret.txt):
  数据 → 名称加密 + 内容加密 → 上传到和彩云

读取 (GET /dav/secret.txt):
  查询 SQLite → 下载密文 → 解密内容 → 返回明文

目录列表 (PROPFIND /dav/):
  云端 E_xxx → SQLite 还原 → 返回原始文件名列表
```

---

## 安全特性汇总

| 特性 | 说明 |
|------|------|
| **文件名不可读** | 云端只看到 `E_` + hex |
| **文件类型隐藏** | 所有文件后缀统一为 `.bin` |
| **文件夹名不可读** | 云端只看到 `E_` + hex |
| **内容加密** | AES-256-GCM 认证加密 |
| **篡改检测** | GCM 认证标签 |
| **密钥安全** | PBKDF2 100K 迭代派生 |
| **密钥不存储** | 内存中不留明文密钥 |
| **配置保护** | 文件权限 0600 |

---

## 配置

### config.yaml

```yaml
crypto:
  enabled: true
  password: "your-password"    # ⚠️ 请修改默认值！
  salt_hex: ""                 # 自动生成
  iterations: 100000

storage:
  root_folder: /MopanProxy
```

### ⚠️ 重要提示

1. **密码丢失 = 数据丢失**：已加密文件无法解密
2. **salt 不可修改**：修改 salt 等于更换密钥
3. **备份 config.yaml**：包含密码和 salt
4. **备份 metadata.db**：包含名称映射

---

## 兼容性

### 已有未加密文件

- 显示为 `is_encrypted: false`
- 下载时直接重定向（不解密）

### 关闭加密

设置 `crypto.enabled: false`：
- 新文件明文存储
- 已加密文件仍可解密下载

### 迁移

1. 拷贝 `config.yaml`（密码 + salt）
2. 拷贝 `data/metadata.db`（名称映射）
3. 部署到新服务器

---

## 代码实现

### crypto/aes256gcm.go

```go
// 名称加密
enc.EncryptName(name)     // → hex(nonce + ciphertext + tag)
enc.EncNameForFolder(name) // → "E_" + hex
enc.EncNameForFile(name)   // → "E_" + hex + ".bin"
enc.DecNameForFolder(cloud) // → (origName, true)
enc.DecNameForFile(cloud)   // → (origName, true)

// 内容加密
enc.Encrypt(plaintext)   // → [nonce][ciphertext][tag]
enc.Decrypt(ciphertext)  // → plaintext
```

### SQLite 元数据

```sql
-- 名称映射
SELECT orig_name FROM files WHERE cloud_name = 'E_50db1161...bin';
-- → "report.pdf"

-- 完整记录
SELECT cloud_file_id, cloud_name, orig_name, type, orig_size, enc_size, nonce
FROM files WHERE cloud_file_id = 'xxx';
```
