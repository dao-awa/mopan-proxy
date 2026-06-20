package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mopan-proxy/internal/crypto"
	"mopan-proxy/internal/metadata"
	"mopan-proxy/internal/mopan"

	"golang.org/x/net/webdav"
)

// MopanFS 实现 WebDAV 所需的文件系统接口
type MopanFS struct {
	client *mopan.Client
	enc    *crypto.Encryptor
	store  *metadata.Store
	rootID string
}

func NewMopanFS(client *mopan.Client) *MopanFS {
	return &MopanFS{client: client, rootID: "/"}
}

func NewMopanFSWithEncryption(client *mopan.Client, enc *crypto.Encryptor, store *metadata.Store, rootID string) *MopanFS {
	if rootID == "" {
		rootID = "/"
	}
	return &MopanFS{client: client, enc: enc, store: store, rootID: rootID}
}

// origName 从云端名称还原原始名称（支持新版加密 + 旧版后缀）
func (fs *MopanFS) origName(cloudName string, isFolder bool) string {
	if fs.enc == nil {
		return cloudName
	}
	if isFolder {
		if orig, ok := fs.enc.DecNameForFolder(cloudName); ok {
			return orig
		}
	} else {
		if orig, ok := fs.enc.DecNameForFile(cloudName); ok {
			return orig
		}
	}
	// 旧版兼容
	if isFolder && strings.HasSuffix(cloudName, "_enc") {
		return strings.TrimSuffix(cloudName, "_enc")
	}
	if !isFolder && strings.HasSuffix(cloudName, ".enc") {
		return strings.TrimSuffix(cloudName, ".enc")
	}
	return cloudName
}

// ========== 目录操作 ==========

func (fs *MopanFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	parent, base := splitPath(name)
	if base == "" {
		return nil
	}
	parentID, err := fs.resolvePath(parent)
	if err != nil {
		return err
	}

	// 加密文件夹名
	cloudName := base
	if fs.enc != nil {
		cloudName = fs.enc.EncNameForFolder(base)
	}

	if err := fs.client.Mkdir(parentID, cloudName); err != nil {
		return err
	}

	// 轮询重试存储元数据
	if fs.store != nil && fs.enc != nil {
		for retry := 0; retry < 10; retry++ {
			time.Sleep(time.Duration(300+retry*200) * time.Millisecond)
			resp, err := fs.client.ListFiles(parentID, "")
			if err != nil {
				continue
			}
			for _, item := range resp.Data.Items {
				if item.Name == cloudName {
					fs.store.PutFile(&metadata.FileInfo{
						CloudFileID:   item.FileId,
						ParentCloudID: parentID,
						CloudName:     cloudName,
						OrigName:      base,
						Type:          "folder",
						IsEncrypted:   true,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					})
					return nil
				}
			}
		}
	}

	return nil
}

func (fs *MopanFS) RemoveAll(ctx context.Context, name string) error {
	id, err := fs.resolvePath(name)
	if err != nil {
		return err
	}
	if err := fs.client.DeleteFiles([]string{id}); err != nil {
		return err
	}
	if fs.store != nil {
		fs.store.DeleteFile(id)
	}
	return nil
}

// Rename 根据原文件类型选择加密名称格式
func (fs *MopanFS) Rename(ctx context.Context, oldName, newName string) error {
	oldID, err := fs.resolvePath(oldName)
	if err != nil {
		return err
	}
	_, newBase := splitPath(newName)
	if fs.enc != nil {
		isFolder := fs.isFolderByID(oldID)
		if isFolder {
			newBase = fs.enc.EncNameForFolder(newBase)
		} else {
			newBase = fs.enc.EncNameForFile(newBase)
		}
	}
	return fs.client.RenameFile(oldID, newBase)
}

func (fs *MopanFS) isFolderByID(fileID string) bool {
	if fs.store != nil {
		info, _ := fs.store.GetFileByID(fileID)
		if info != nil {
			return info.Type == "folder"
		}
	}
	return false
}

// ========== 文件读取 ==========

func (fs *MopanFS) Open(ctx context.Context, name string, flags int, perm os.FileMode) (webdav.File, error) {
	return fs.OpenFile(ctx, name, flags, perm)
}

func (fs *MopanFS) OpenFile(ctx context.Context, name string, flags int, perm os.FileMode) (webdav.File, error) {
	if flags&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0 {
		return fs.Create(ctx, name, flags, perm)
	}

	info, err := fs.stat(name)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return &mopanDir{
			info:  info,
			name:  name,
			files: []os.FileInfo{},
			fs:    fs,
		}, nil
	}

	id, err := fs.resolvePath(name)
	if err != nil {
		return nil, err
	}

	downloadUrl, err := fs.client.GetDownloadUrl(id)
	if err != nil {
		return nil, err
	}

	tmpFile, err := os.CreateTemp("", "mopan-*")
	if err != nil {
		return nil, err
	}
	// H-004: 临时文件权限设为仅所有者可读写
	os.Chmod(tmpFile.Name(), 0600)

	resp, err := http.Get(downloadUrl)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	defer resp.Body.Close()

	// L-023: 检查 io.Copy 错误
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	tmpFile.Seek(0, 0)

	// 解密
	if fs.enc != nil && fs.store != nil {
		cloudInfo, _ := fs.store.GetFileByID(id)
		if cloudInfo != nil && cloudInfo.IsEncrypted {
			tmpFile.Seek(0, 0)
			ciphertext, err := io.ReadAll(tmpFile)
			tmpFile.Close()
			if err != nil {
				os.Remove(tmpFile.Name())
				return nil, err
			}
			plaintext, err := fs.enc.Decrypt(ciphertext)
			if err != nil {
				os.Remove(tmpFile.Name())
				return nil, err
			}
			decFile, err := os.CreateTemp("", "mopan-dec-*")
			if err != nil {
				os.Remove(tmpFile.Name())
				return nil, err
			}
			os.Chmod(decFile.Name(), 0600)
			decFile.Write(plaintext)
			decFile.Seek(0, 0)
			os.Remove(tmpFile.Name())
			return &mopanFile{
				File: decFile,
				info: &fileInfo{name: cloudInfo.OrigName, size: cloudInfo.OrigSize, modTime: cloudInfo.UpdatedAt},
				name: name,
			}, nil
		}
	}

	return &mopanFile{File: tmpFile, info: info, name: name}, nil
}

// ========== 文件写入 ==========

func (fs *MopanFS) Create(ctx context.Context, name string, flags int, perm os.FileMode) (webdav.File, error) {
	tmpFile, err := os.CreateTemp("", "mopan-write-*")
	if err != nil {
		return nil, err
	}
	os.Chmod(tmpFile.Name(), 0600)
	return &mopanWriteFile{File: tmpFile, fs: fs, name: name}, nil
}

// ========== 文件信息 ==========

func (fs *MopanFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return fs.stat(name)
}

func (fs *MopanFS) stat(name string) (os.FileInfo, error) {
	if name == "/" || name == "" {
		return &dirInfo{name: "/", size: 0, modTime: time.Now()}, nil
	}

	parent, base := splitPath(name)
	parentID, err := fs.resolvePath(parent)
	if err != nil {
		return nil, err
	}

	resp, err := fs.client.ListFiles(parentID, "")
	if err != nil {
		return nil, err
	}

	for _, item := range resp.Data.Items {
		isFolder := item.Type == "folder"
		origName := fs.origName(item.Name, isFolder)

		if origName == base {
			if isFolder {
				return &dirInfo{name: base, size: 0, modTime: parseTime(item.UpdatedAt)}, nil
			}
			fSize := item.Size
			if fs.store != nil {
				info, _ := fs.store.GetFileByID(item.FileId)
				if info != nil {
					fSize = info.OrigSize
				}
			}
			return &fileInfo{name: base, size: fSize, modTime: parseTime(item.UpdatedAt)}, nil
		}
	}

	return nil, os.ErrNotExist
}

// ========== 路径解析 ==========

func (fs *MopanFS) resolvePath(name string) (string, error) {
	if name == "/" || name == "" {
		return fs.rootID, nil
	}

	parts := strings.Split(strings.Trim(name, "/"), "/")
	currentID := fs.rootID

	for _, part := range parts {
		if part == "" {
			continue
		}

		resp, err := fs.client.ListFiles(currentID, "")
		if err != nil {
			return "", fmt.Errorf("list failed")
		}

		found := false
		for _, item := range resp.Data.Items {
			isFolder := item.Type == "folder"

			// 1. 精确匹配（旧版兼容）
			if item.Name == part {
				currentID = item.FileId
				found = true
				break
			}

			// 2. 解密匹配（新版加密名）
			if fs.enc != nil {
				origName := fs.origName(item.Name, isFolder)
				if origName == part {
					currentID = item.FileId
					found = true
					break
				}
			}
		}

		if !found {
			return "", os.ErrNotExist
		}
	}

	return currentID, nil
}

// ========== 辅助函数 ==========

func splitPath(name string) (string, string) {
	name = strings.Trim(name, "/")
	if name == "" {
		return "/", ""
	}
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 1 {
		return "/", parts[0]
	}
	return "/" + parts[0], parts[1]
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// ========== FileInfo ==========

type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (fi *fileInfo) Name() string      { return fi.name }
func (fi *fileInfo) Size() int64       { return fi.size }
func (fi *fileInfo) Mode() os.FileMode { return 0644 }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool       { return false }
func (fi *fileInfo) Sys() interface{}  { return nil }

type dirInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (di *dirInfo) Name() string      { return di.name }
func (di *dirInfo) Size() int64       { return di.size }
func (di *dirInfo) Mode() os.FileMode { return os.ModeDir | 0755 }
func (di *dirInfo) ModTime() time.Time { return di.modTime }
func (di *dirInfo) IsDir() bool       { return true }
func (di *dirInfo) Sys() interface{}  { return nil }

// ========== File 接口 ==========

type mopanFile struct {
	*os.File
	info os.FileInfo
	name string
}

func (f *mopanFile) Readdir(count int) ([]os.FileInfo, error) { return []os.FileInfo{}, nil }
func (f *mopanFile) Stat() (os.FileInfo, error)               { return f.info, nil }

type mopanWriteFile struct {
	*os.File
	fs   *MopanFS
	name string
}

type mopanDir struct {
	info  os.FileInfo
	name  string
	files []os.FileInfo
	fs    *MopanFS
	pos   int
}

func (d *mopanDir) Close() error                               { return nil }
func (d *mopanDir) Read(p []byte) (int, error)                 { return 0, nil }
func (d *mopanDir) Write(p []byte) (int, error)                { return 0, nil }
func (d *mopanDir) Seek(offset int64, whence int) (int64, error) { return 0, nil }

func (d *mopanDir) Readdir(count int) ([]os.FileInfo, error) {
	if len(d.files) == 0 {
		parentID, err := d.fs.resolvePath(d.name)
		if err != nil {
			return nil, err
		}
		resp, err := d.fs.client.ListFiles(parentID, "")
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Data.Items {
			isFolder := item.Type == "folder"
			name := d.fs.origName(item.Name, isFolder)
			size := item.Size
			if d.fs.store != nil {
				info, _ := d.fs.store.GetFileByID(item.FileId)
				if info != nil {
					name = info.OrigName
					if isFolder {
						size = 0
					} else {
						size = info.OrigSize
					}
				}
			}
			if isFolder {
				d.files = append(d.files, &dirInfo{name: name, size: 0, modTime: parseTime(item.UpdatedAt)})
			} else {
				d.files = append(d.files, &fileInfo{name: name, size: size, modTime: parseTime(item.UpdatedAt)})
			}
		}
	}

	if count <= 0 {
		files := d.files[d.pos:]
		d.pos = len(d.files)
		return files, nil
	}
	end := d.pos + count
	if end > len(d.files) {
		end = len(d.files)
	}
	files := d.files[d.pos:end]
	d.pos = end
	return files, nil
}

func (d *mopanDir) Stat() (os.FileInfo, error) { return d.info, nil }

func (f *mopanWriteFile) Close() error {
	f.File.Seek(0, 0)
	info, _ := f.File.Stat()
	f.File.Close()

	if info.Size() == 0 {
		os.Remove(f.File.Name())
		return nil
	}

	file, err := os.Open(f.File.Name())
	if err != nil {
		os.Remove(f.File.Name())
		return err
	}
	plaintext, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		os.Remove(f.File.Name())
		return err
	}

	parent, base := splitPath(f.name)
	parentID, err := f.fs.resolvePath(parent)
	if err != nil {
		os.Remove(f.File.Name())
		return fmt.Errorf("resolve parent failed")
	}

	if f.fs.enc != nil {
		encrypted, err := f.fs.enc.Encrypt(plaintext)
		if err != nil {
			os.Remove(f.File.Name())
			return err
		}
		encName := f.fs.enc.EncNameForFile(base)
		plainHash := crypto.PlainTextHash(plaintext)

		err = f.fs.client.UploadFileEncrypted(parentID, encName, bytes.NewReader(encrypted), int64(len(encrypted)), plainHash)
		if err != nil {
			os.Remove(f.File.Name())
			return err
		}

		var nonce []byte
		if len(encrypted) >= crypto.NonceSize {
			nonce = make([]byte, crypto.NonceSize)
			copy(nonce, encrypted[:crypto.NonceSize])
		}

		if f.fs.store != nil {
			// M-025: 修复重试循环 break 逻辑
			for retry := 0; retry < 10; retry++ {
				time.Sleep(time.Duration(300+retry*200) * time.Millisecond)
				resp, err := f.fs.client.ListFiles(parentID, "")
				if err != nil {
					continue
				}
				found := false
				for _, item := range resp.Data.Items {
					if item.Name == encName {
						f.fs.store.PutFile(&metadata.FileInfo{
							CloudFileID:   item.FileId,
							ParentCloudID: parentID,
							CloudName:     encName,
							OrigName:      base,
							Type:          "file",
							OrigSize:      info.Size(),
							EncSize:       int64(len(encrypted)),
							Nonce:         nonce,
							ContentHash:   plainHash,
							IsEncrypted:   true,
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
						})
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
	} else {
		err = f.fs.client.UploadFile(parentID, base, bytes.NewReader(plaintext), info.Size())
		if err != nil {
			os.Remove(f.File.Name())
			return err
		}
	}

	os.Remove(f.File.Name())
	return nil
}

func (f *mopanWriteFile) Readdir(count int) ([]os.FileInfo, error) { return []os.FileInfo{}, nil }
func (f *mopanWriteFile) Stat() (os.FileInfo, error) {
	return &fileInfo{name: f.name, size: 0, modTime: time.Now()}, nil
}
