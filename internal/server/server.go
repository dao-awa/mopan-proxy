package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"mopan-proxy/internal/config"
	"mopan-proxy/internal/crypto"
	"mopan-proxy/internal/metadata"
	"mopan-proxy/internal/mopan"
	mopanWebdav "mopan-proxy/internal/webdav"

	"golang.org/x/net/webdav"
)

type Server struct {
	cfg        *config.Config
	configPath string
	client     *mopan.Client
	enc        *crypto.Encryptor
	store      *metadata.Store
	webDir     string
	rootID     string

	saveMu     sync.Mutex
	httpSrv    *http.Server
	webdavSrv  *http.Server
	storeClose func()

	// Session 管理
	sessions   map[string]time.Time // sessionID → 过期时间
	sessionMu  sync.RWMutex
	sessionKey []byte // HMAC 签名密钥
}

func New(cfg *config.Config, configPath string) (*Server, error) {
	client := mopan.NewClient(cfg.Account, cfg.Authorization)
	os.MkdirAll(cfg.Storage.CacheDir, 0700)

	// C-002: WebDAV 启用时强制要求非空凭据
	if cfg.WebDAV.Enabled && (cfg.WebDAV.Username == "" || cfg.WebDAV.Password == "") {
		return nil, fmt.Errorf("WebDAV 已启用但用户名或密码为空，请在 config.yaml 中配置 webdav.username 和 webdav.password")
	}

	// M-001: 检测默认密码并警告
	if cfg.Crypto.Password == "mopan-default-password" {
		log.Println("⚠️  警告: 正在使用默认加密密码，请在 config.yaml 中修改 crypto.password")
	}
	if cfg.WebDAV.Password == "admin" && cfg.WebDAV.Username == "admin" {
		log.Println("⚠️  警告: 正在使用默认 WebDAV 凭据 admin/admin，请修改")
	}
	if cfg.Auth.Enabled && cfg.Auth.Password == "admin123" {
		log.Println("⚠️  警告: 正在使用默认登录密码 admin123，请在 config.yaml 中修改 auth.password")
	}

	var enc *crypto.Encryptor
	if cfg.Crypto.Enabled {
		e, salt, err := crypto.NewEncryptor(cfg.Crypto.Password, cfg.Crypto.SaltHex, cfg.Crypto.Iterations)
		if err != nil {
			return nil, fmt.Errorf("init crypto: %w", err)
		}
		enc = e
		if salt != "" {
			cfg.Crypto.SaltHex = salt
			cfg.Save(configPath)
		}
		log.Printf("加密已启用 (AES-256-GCM, PBKDF2 %d iterations)", cfg.Crypto.Iterations)
	} else {
		log.Printf("加密未启用")
	}

	store, err := metadata.NewStore(cfg.Storage.MetaPath)
	if err != nil {
		return nil, fmt.Errorf("init metadata store: %w", err)
	}

	s := &Server{
		cfg:        cfg,
		configPath: configPath,
		client:     client,
		enc:        enc,
		store:      store,
		webDir:     cfg.Storage.CacheDir,
		storeClose: func() { store.Close() },
		sessions:   make(map[string]time.Time),
	}

	if err := s.initRootFolder(); err != nil {
		log.Printf("⚠️  根目录初始化失败: %v (可通过 Web UI 设置 Token 后重试)", err)
	}

	return s, nil
}

func (s *Server) Shutdown(ctx context.Context) {
	if s.webdavSrv != nil {
		s.webdavSrv.Shutdown(ctx)
	}
	if s.httpSrv != nil {
		s.httpSrv.Shutdown(ctx)
	}
	if s.storeClose != nil {
		s.storeClose()
	}
}

func (s *Server) initRootFolder() error {
	rootFolder := s.cfg.Storage.RootFolder
	if rootFolder == "" || rootFolder == "/" {
		s.rootID = "/"
		return nil
	}

	cachedID, _ := s.store.GetConfig("root_cloud_id")
	if cachedID != "" {
		resp, err := s.client.ListFiles("/", "")
		if err == nil {
			for _, item := range resp.Data.Items {
				if item.FileId == cachedID {
					s.rootID = cachedID
					log.Printf("根目录: %s (cached)", rootFolder)
					return nil
				}
			}
		}
	}

	rootName := strings.TrimPrefix(rootFolder, "/")
	resp, err := s.client.ListFiles("/", "")
	if err != nil {
		return fmt.Errorf("list root: %w", err)
	}

	if s.store != nil && s.enc != nil {
		encName := s.enc.EncNameForFolder(rootName)
		for _, item := range resp.Data.Items {
			if item.Name == encName || item.Name == rootName || item.Name == rootName+"_enc" {
				s.rootID = item.FileId
				s.store.SetConfig("root_cloud_id", s.rootID)
				log.Printf("根目录: %s (found)", rootFolder)
				return nil
			}
		}
	} else {
		for _, item := range resp.Data.Items {
			if item.Name == rootName {
				s.rootID = item.FileId
				s.store.SetConfig("root_cloud_id", s.rootID)
				log.Printf("根目录: %s (found)", rootFolder)
				return nil
			}
		}
	}

	log.Printf("根目录 %s 不存在，正在创建...", rootFolder)
	encRootName := rootName
	if s.enc != nil {
		encRootName = s.enc.EncNameForFolder(rootName)
	}
	if err := s.client.Mkdir("/", encRootName); err != nil {
		return fmt.Errorf("create root folder: %w", err)
	}

	for retry := 0; retry < 10; retry++ {
		time.Sleep(time.Duration(500+retry*200) * time.Millisecond)
		resp, err = s.client.ListFiles("/", "")
		if err != nil {
			continue
		}
		for _, item := range resp.Data.Items {
			if item.Name == encRootName || item.Name == rootName {
				s.rootID = item.FileId
				s.store.SetConfig("root_cloud_id", s.rootID)
				if s.store != nil && s.enc != nil {
					s.store.PutFile(&metadata.FileInfo{
						CloudFileID:   item.FileId,
						ParentCloudID: "/",
						CloudName:     encRootName,
						OrigName:      rootName,
						Type:          "folder",
						IsEncrypted:   true,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					})
				}
				log.Printf("根目录: %s (created)", rootFolder)
				return nil
			}
		}
	}

	return fmt.Errorf("root folder created but not found")
}

// ========== 安全中间件 ==========

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.client.IsAuthed() {
			jsonError(w, "未登录", 401)
			return
		}
		next(w, r)
	}
}

// H-003: CSRF 保护 — 检查 X-Requested-With + Origin/Referer
func (s *Server) csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if r.Header.Get("X-Requested-With") == "" {
				jsonError(w, "CSRF 验证失败", 403)
				return
			}
			// 额外检查 Origin 或 Referer（如果提供了的话）
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")
			allowed := func(url string) bool {
				return strings.HasPrefix(url, "http://localhost") ||
					strings.HasPrefix(url, "https://localhost") ||
					strings.HasPrefix(url, "http://"+s.cfg.Host) ||
					strings.HasPrefix(url, "https://"+s.cfg.Host) ||
					strings.Contains(url, "webdav.96110.li") ||
					strings.Contains(url, "yun.139.com")
			}
			if origin != "" {
				if !allowed(origin) {
					jsonError(w, "CSRF 验证失败", 403)
					return
				}
			} else if referer != "" {
				if !allowed(referer) {
					jsonError(w, "CSRF 验证失败", 403)
					return
				}
				}
			}
			next(w, r)
	}
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) maxBytesMiddleware(maxBytes int64, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next(w, r)
	}
}

// sanitizeName 清理文件/文件夹名称：移除控制字符、空字节、限制长度
func sanitizeName(name string) string {
	// L-010: 移除空字节和控制字符
	var b strings.Builder
	for _, r := range name {
		if r == 0 || unicode.IsControl(r) && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	name = b.String()
	name = strings.TrimSpace(name)
	// 限制长度
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

// ========== Session 管理 ==========

func (s *Server) createSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	id := hex.EncodeToString(b)

	s.sessionMu.Lock()
	s.sessions[id] = time.Now().Add(7 * 24 * time.Hour)
	s.sessionMu.Unlock()
	return id
}

func (s *Server) validateSession(id string) bool {
	if id == "" {
		return false
	}
	s.sessionMu.RLock()
	expiry, ok := s.sessions[id]
	s.sessionMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		s.sessionMu.Lock()
		delete(s.sessions, id)
		s.sessionMu.Unlock()
		return false
	}
	return true
}

func (s *Server) deleteSession(id string) {
	s.sessionMu.Lock()
	delete(s.sessions, id)
	s.sessionMu.Unlock()
}

// 清理过期 session（每小时执行一次）
func (s *Server) cleanupSessions() {
	for {
		time.Sleep(time.Hour)
		now := time.Now()
		s.sessionMu.Lock()
		for id, expiry := range s.sessions {
			if now.After(expiry) {
				delete(s.sessions, id)
			}
		}
		s.sessionMu.Unlock()
	}
}

func (s *Server) getSessionCookie(r *http.Request) string {
	c, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}
	return c.Value
}

func (s *Server) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 天
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// ========== 登录/登出 Handlers ==========

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.cfg.Auth.Username)) != 1 ||
		subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.Auth.Password)) != 1 {
		jsonError(w, "用户名或密码错误", 401)
		return
	}

	sessionID := s.createSession()
	s.setSessionCookie(w, sessionID)
	log.Printf("用户 %s 登录成功", req.Username)
	jsonOK(w, map[string]string{"message": "登录成功"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	sessionID := s.getSessionCookie(r)
	if sessionID != "" {
		s.deleteSession(sessionID)
	}
	s.clearSessionCookie(w)
	jsonOK(w, map[string]string{"message": "已退出"})
}

// ========== 路由保护 ==========

func (s *Server) authGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth 未启用时直接放行
		if !s.cfg.Auth.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// 登录/登出接口放行
		if r.URL.Path == "/api/login" || r.URL.Path == "/api/logout" {
			next.ServeHTTP(w, r)
			return
		}

		// 检查 session
		if s.validateSession(s.getSessionCookie(r)) {
			next.ServeHTTP(w, r)
			return
		}

		// 未认证
		if strings.HasPrefix(r.URL.Path, "/api/") {
			jsonError(w, "请先登录", 401)
			return
		}
		// 页面请求返回登录页
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, loginHTML)
	})
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// 登录/登出（不需要认证）
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)

	mux.HandleFunc("/api/status", s.handleStatus)
	// C-001: /api/set-token 也加认证保护
	mux.HandleFunc("/api/set-token", s.authMiddleware(s.csrfMiddleware(s.handleSetToken)))
	mux.HandleFunc("/api/files", s.authMiddleware(s.handleListFiles))
	mux.HandleFunc("/api/mkdir", s.authMiddleware(s.maxBytesMiddleware(1<<20, s.handleMkdir)))
	mux.HandleFunc("/api/delete", s.authMiddleware(s.csrfMiddleware(s.maxBytesMiddleware(1<<20, s.handleDelete))))
	mux.HandleFunc("/api/download/", s.authMiddleware(s.handleDownload))
	mux.HandleFunc("/api/upload", s.authMiddleware(s.handleUpload))
	mux.HandleFunc("/api/settings", s.authMiddleware(s.handleSettings))
	mux.HandleFunc("/api/settings/update", s.authMiddleware(s.csrfMiddleware(s.maxBytesMiddleware(1<<20, s.handleSettingsUpdate))))
	mux.HandleFunc("/api/stats", s.authMiddleware(s.handleStats))
	mux.HandleFunc("/", s.handleIndex)

	// 用 authGuard 包裹所有路由
	return s.securityHeaders(s.authGuard(mux))
}

func (s *Server) Start() error {
	handler := s.SetupRoutes()
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	if s.cfg.Auth.Enabled {
		log.Printf("MopanProxy 启动在 http://%s (登录保护: 开启)", addr)
	} else {
		log.Printf("MopanProxy 启动在 http://%s (登录保护: 关闭)", addr)
	}

	// 启动 session 清理
	go s.cleanupSessions()

	// M-017: 添加 HTTP 超时配置
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      30 * time.Minute, // 上传需要长时间
		IdleTimeout:       120 * time.Second,
	}

	if s.cfg.WebDAV.Enabled {
		go s.startWebDAV()
	}

	return s.httpSrv.ListenAndServe()
}

func (s *Server) startWebDAV() {
	// 本地目录模式
	localDir := "/mnt/yidong"
	os.MkdirAll(localDir, 0755)
	fs := webdav.Dir(localDir)
	log.Printf("WebDAV 本地目录: %s", localDir)
	davHandler := &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("WebDAV: %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	// H-002: 使用 ConstantTimeCompare 防时序攻击
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(s.cfg.WebDAV.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.WebDAV.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="MopanProxy WebDAV"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		davHandler.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.WebDAV.Port)
	log.Printf("WebDAV 服务器启动在 http://%s/dav/", addr)

	s.webdavSrv = &http.Server{
		Addr:              addr,
		Handler:           authHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	if err := s.webdavSrv.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
		log.Printf("WebDAV 服务器错误: %v", err)
	}
}

// ========== 名称加密/解密 ==========

func (s *Server) encFolderName(name string) string {
	if s.enc == nil {
		return name
	}
	return s.enc.EncNameForFolder(name)
}

func (s *Server) encFileName(name string) string {
	if s.enc == nil {
		return name
	}
	return s.enc.EncNameForFile(name)
}

func (s *Server) origName(cloudName string, isFolder bool) string {
	if s.enc == nil {
		return cloudName
	}
	if isFolder {
		if orig, ok := s.enc.DecNameForFolder(cloudName); ok {
			return orig
		}
	} else {
		if orig, ok := s.enc.DecNameForFile(cloudName); ok {
			return orig
		}
	}
	if isFolder && strings.HasSuffix(cloudName, "_enc") {
		return strings.TrimSuffix(cloudName, "_enc")
	}
	if !isFolder && strings.HasSuffix(cloudName, ".enc") {
		return strings.TrimSuffix(cloudName, ".enc")
	}
	return cloudName
}

// ========== 路径解析 ==========

func (s *Server) resolvePath(path string) (string, error) {
	// L-010: 路径规范化
	path = filepath.Clean(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return s.rootID, nil
	}

	parts := strings.Split(path, "/")
	currentID := s.rootID

	for _, part := range parts {
		if part == "" {
			continue
		}

		resp, err := s.client.ListFiles(currentID, "")
		if err != nil {
			return "", fmt.Errorf("list files failed")
		}

		found := false
		for _, item := range resp.Data.Items {
			isFolder := item.Type == "folder"
			if item.Name == part {
				currentID = item.FileId
				found = true
				break
			}
			if s.enc != nil {
				if s.origName(item.Name, isFolder) == part {
					currentID = item.FileId
					found = true
					break
				}
			}
		}

		if !found {
			return "", fmt.Errorf("not found")
		}
	}

	return currentID, nil
}

func (s *Server) resolvePathForUpload(path string) (string, error) {
	path = filepath.Clean(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return s.rootID, nil
	}

	parts := strings.Split(path, "/")
	currentID := s.rootID

	for _, part := range parts {
		if part == "" {
			continue
		}

		resp, err := s.client.ListFiles(currentID, "")
		if err != nil {
			return "", fmt.Errorf("list files failed")
		}

		found := false
		for _, item := range resp.Data.Items {
			if item.Type != "folder" {
				continue
			}
			if item.Name == part {
				currentID = item.FileId
				found = true
				break
			}
			if s.enc != nil && s.origName(item.Name, true) == part {
				currentID = item.FileId
				found = true
				break
			}
		}

		if !found {
			return "", fmt.Errorf("folder not found")
		}
	}

	return currentID, nil
}

func (s *Server) findItemByName(parentID, encName string, maxRetries int) (string, bool) {
	for retry := 0; retry < maxRetries; retry++ {
		time.Sleep(time.Duration(300+retry*200) * time.Millisecond)
		resp, err := s.client.ListFiles(parentID, "")
		if err != nil {
			continue
		}
		for _, item := range resp.Data.Items {
			if item.Name == encName {
				return item.FileId, true
			}
		}
	}
	return "", false
}

// ========== API Handlers ==========

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// M-002: 隐藏手机号，只显示部分
	account := s.client.Account
	if len(account) > 3 {
		account = account[:3] + "****"
	}
	jsonOK(w, map[string]interface{}{
		"auth":      s.client.IsAuthed(),
		"logged_in": true,
		"account":   account,
		"encrypted": s.enc != nil,
	})
}

func (s *Server) handleSetToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Authorization string `json:"authorization"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	if req.Authorization == "" {
		jsonError(w, "authorization is required", 400)
		return
	}
	authStr := req.Authorization
	if !strings.HasPrefix(authStr, "Basic ") {
		authStr = "Basic " + authStr
	}
	s.client.SetAuthorization(authStr)
	if !s.client.IsAuthed() {
		jsonError(w, "invalid token format", 400)
		return
	}
	s.cfg.Authorization = authStr
	s.cfg.Account = s.client.Account
	s.saveConfig()

	// 如果根目录未初始化，尝试重新初始化
	if s.rootID == "" {
		if err := s.initRootFolder(); err != nil {
			log.Printf("⚠️  根目录初始化失败: %v", err)
		}
	}

	log.Printf("Token 设置成功")
	jsonOK(w, map[string]interface{}{
		"account": s.client.Account,
		"message": "Token 设置成功",
	})
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	cursor := r.URL.Query().Get("cursor")

	parentID, err := s.resolvePath(path)
	if err != nil {
		jsonError(w, "路径不存在", 404)
		return
	}

	resp, err := s.client.ListFiles(parentID, cursor)
	if err != nil {
		jsonError(w, "获取文件列表失败", 500)
		return
	}

	var files []map[string]interface{}
	for _, f := range resp.Data.Items {
		isFolder := f.Type == "folder"
		name := s.origName(f.Name, isFolder)
		size := f.Size
		isEncrypted := s.enc != nil

		if s.store != nil {
			info, err := s.store.GetFileByID(f.FileId)
			if err == nil && info != nil {
				name = info.OrigName
				if isFolder {
					size = 0
				} else {
					size = info.OrigSize
				}
				isEncrypted = info.IsEncrypted
			}
		}

		files = append(files, map[string]interface{}{
			"id":           f.FileId,
			"name":         name,
			"size":         size,
			"type":         f.Type,
			"created_at":   f.CreatedAt,
			"updated_at":   f.UpdatedAt,
			"is_encrypted": isEncrypted,
		})
	}

	var nextCursor string
	if resp.Data.NextPageCursor != nil {
		nextCursor = *resp.Data.NextPageCursor
	}
	jsonOK(w, map[string]interface{}{
		"files":       files,
		"next_cursor": nextCursor,
		"total":       len(files),
	})
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}

	// M-008: 输入验证
	req.Name = sanitizeName(req.Name)
	if req.Name == "" {
		jsonError(w, "文件夹名称不能为空", 400)
		return
	}

	parentID, err := s.resolvePathForUpload(req.ParentID)
	if err != nil {
		jsonError(w, "路径不存在", 404)
		return
	}

	cloudName := s.encFolderName(req.Name)

	if err := s.client.Mkdir(parentID, cloudName); err != nil {
		jsonError(w, "创建文件夹失败", 500)
		return
	}

	if s.store != nil && s.enc != nil {
		if itemID, ok := s.findItemByName(parentID, cloudName, 10); ok {
			s.store.PutFile(&metadata.FileInfo{
				CloudFileID:   itemID,
				ParentCloudID: parentID,
				CloudName:     cloudName,
				OrigName:      req.Name,
				Type:          "folder",
				IsEncrypted:   true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			})
		}
	}

	jsonOK(w, map[string]string{"message": "创建成功"})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileIDs []string `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	// L-011: 数组长度限制
	if len(req.FileIDs) == 0 {
		jsonError(w, "file_ids 不能为空", 400)
		return
	}
	if len(req.FileIDs) > 100 {
		jsonError(w, "单次最多删除 100 个文件", 400)
		return
	}
	if err := s.client.DeleteFiles(req.FileIDs); err != nil {
		jsonError(w, "删除失败", 500)
		return
	}
	if s.store != nil {
		s.store.DeleteFiles(req.FileIDs)
	}
	jsonOK(w, map[string]string{"message": "删除成功"})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	fileID := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if fileID == "" || strings.Contains(fileID, "/") {
		http.Error(w, "invalid file id", 400)
		return
	}

	if s.enc != nil && s.store != nil {
		info, err := s.store.GetFileByID(fileID)
		if err == nil && info != nil && info.IsEncrypted {
			s.downloadDecrypted(w, r, fileID, info)
			return
		}
	}

	downloadUrl, err := s.client.GetDownloadUrl(fileID)
	if err != nil {
		http.Error(w, "download failed", 500)
		return
	}
	http.Redirect(w, r, downloadUrl, http.StatusTemporaryRedirect)
}

func (s *Server) downloadDecrypted(w http.ResponseWriter, r *http.Request, fileID string, info *metadata.FileInfo) {
	downloadUrl, err := s.client.GetDownloadUrl(fileID)
	if err != nil {
		http.Error(w, "download failed", 500)
		return
	}
	httpClient := &http.Client{Timeout: 30 * time.Minute}
	resp, err := httpClient.Get(downloadUrl)
	if err != nil {
		http.Error(w, "download failed", 500)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		http.Error(w, "cloud download failed", 500)
		return
	}
	ciphertext, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read failed", 500)
		return
	}
	plaintext, err := s.enc.Decrypt(ciphertext)
	if err != nil {
		http.Error(w, "解密失败", 500)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.OrigSize))
	// M-009: Content-Disposition 文件名转义
	escapedName := strings.ReplaceAll(info.OrigName, `\`, `\\`)
	escapedName = strings.ReplaceAll(escapedName, `"`, `\"`)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, escapedName))
	w.Write(plaintext)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{
		"account":            s.client.Account,
		"webdav_port":        s.cfg.WebDAV.Port,
		"webdav_user":        s.cfg.WebDAV.Username,
		"webdav_pass":        "****",
		"webdav_enabled":     s.cfg.WebDAV.Enabled,
		"port":               s.cfg.Port,
		"encryption_enabled": s.enc != nil,
		"root_folder":        s.cfg.Storage.RootFolder,
	})
}

func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		WebDAVPort    *int    `json:"webdav_port"`
		WebDAVUser    *string `json:"webdav_user"`
		WebDAVPass    *string `json:"webdav_pass"`
		WebDAVEnabled *bool   `json:"webdav_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	if req.WebDAVPort != nil {
		s.cfg.WebDAV.Port = *req.WebDAVPort
	}
	if req.WebDAVUser != nil {
		s.cfg.WebDAV.Username = *req.WebDAVUser
	}
	if req.WebDAVPass != nil && *req.WebDAVPass != "****" {
		s.cfg.WebDAV.Password = *req.WebDAVPass
	}
	if req.WebDAVEnabled != nil {
		s.cfg.WebDAV.Enabled = *req.WebDAVEnabled
	}
	// C-002: 保存前验证 WebDAV 凭据非空
	if s.cfg.WebDAV.Enabled && (s.cfg.WebDAV.Username == "" || s.cfg.WebDAV.Password == "") {
		jsonError(w, "WebDAV 用户名和密码不能为空", 400)
		return
	}
	s.saveConfig()
	jsonOK(w, map[string]string{"message": "设置已保存"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		jsonOK(w, map[string]interface{}{"encrypted_files": 0})
		return
	}
	count, origSize, encSize, err := s.store.Stats()
	if err != nil {
		jsonError(w, "stats failed", 500)
		return
	}
	jsonOK(w, map[string]interface{}{
		"encrypted_files": count,
		"total_orig_size": origSize,
		"total_enc_size":  encSize,
		"encryption":      s.enc != nil,
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, indexHTML)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	maxUpload := s.cfg.Storage.MaxUpload
	if maxUpload <= 0 {
		maxUpload = 32 << 20
	}
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		jsonError(w, "文件过大或请求格式错误", 400)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "no file provided", 400)
		return
	}
	defer file.Close()

	// M-007: 文件名验证
	filename := sanitizeName(header.Filename)
	if filename == "" {
		jsonError(w, "文件名无效", 400)
		return
	}

	parentPath := r.FormValue("path")
	if parentPath == "" {
		parentPath = "/"
	}

	parentID, err := s.resolvePathForUpload(parentPath)
	if err != nil {
		jsonError(w, "路径不存在", 404)
		return
	}

	if s.enc != nil {
		s.uploadEncrypted(w, parentID, filename, file, header.Size)
		return
	}

	if err := s.client.UploadFile(parentID, filename, file, header.Size); err != nil {
		jsonError(w, "上传失败", 500)
		return
	}
	jsonOK(w, map[string]interface{}{
		"message":   "上传成功",
		"encrypted": false,
	})
}

func (s *Server) uploadEncrypted(w http.ResponseWriter, parentID, filename string, file io.Reader, size int64) {
	plaintext, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, "读取文件失败", 500)
		return
	}
	plainHash := crypto.PlainTextHash(plaintext)

	encrypted, err := s.enc.Encrypt(plaintext)
	if err != nil {
		jsonError(w, "加密失败", 500)
		return
	}

	encName := s.encFileName(filename)

	if err := s.client.UploadFileEncrypted(parentID, encName, bytes.NewReader(encrypted), int64(len(encrypted)), plainHash); err != nil {
		jsonError(w, "上传失败", 500)
		return
	}

	var nonce []byte
	if len(encrypted) >= crypto.NonceSize {
		nonce = make([]byte, crypto.NonceSize)
		copy(nonce, encrypted[:crypto.NonceSize])
	}

	if s.store != nil {
		if itemID, ok := s.findItemByName(parentID, encName, 10); ok {
			s.store.PutFile(&metadata.FileInfo{
				CloudFileID:   itemID,
				ParentCloudID: parentID,
				CloudName:     encName,
				OrigName:      filename,
				Type:          "file",
				OrigSize:      size,
				EncSize:       int64(len(encrypted)),
				Nonce:         nonce,
				ContentHash:   plainHash,
				IsEncrypted:   true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			})
		}
	}

	jsonOK(w, map[string]interface{}{
		"message":   "上传成功",
		"encrypted": true,
		"orig_size": size,
		"enc_size":  len(encrypted),
	})
}

func (s *Server) saveConfig() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.cfg.Save(s.configPath)
}

// ========== 工具函数 ==========

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}
