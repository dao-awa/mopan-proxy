package mopan

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	// 和彩云 API 基础 URL
	BaseURL     = "https://personal-kd-njs.yun.139.com/hcy"
	UserBaseURL = "https://user-njs.yun.139.com/user"

	// 客户端版本（与浏览器一致）
	AppVersion = "7.17.4"
	ClientID   = "10701"

	// 设备标识
	DeviceInfo     = "||9|7.17.4|edge||6bcccd2a27658b8342c5ffe03e5d91be||windows 10||zh-CN|||"
	ClientInfoFull = "||9|7.17.4|edge||6bcccd2a27658b8342c5ffe03e5d91be||windows 10||zh-CN|||ZWRnZQ==||"
)

// Client 和彩云 API 客户端
type Client struct {
	Account       string // 手机号
	Authorization string // 完整的 Authorization header 值
	Token         string // token 部分
	httpClient    *http.Client
	personalHost  string // 个人云 API 主机
}

// NewClient 创建客户端
func NewClient(account, authorization string) *Client {
	token := ""
	if authorization != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authorization, "Basic "))
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 3)
			if len(parts) >= 3 {
				token = parts[2]
				if account == "" {
					account = parts[1]
				}
			}
		}
	}
	return &Client{
		Account:       account,
		Authorization: authorization,
		Token:         token,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}
}

// SetAuthorization 设置 Authorization
func (c *Client) SetAuthorization(auth string) {
	c.Authorization = auth
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return
	}
	parts := strings.SplitN(string(decoded), ":", 3)
	if len(parts) >= 3 {
		c.Account = parts[1]
		c.Token = parts[2]
	}
}

// IsAuthed 是否已认证
func (c *Client) IsAuthed() bool {
	return c.Authorization != "" && c.Token != ""
}

// sendRequest 发送 API 请求（新 API 格式，与浏览器一致）
func (c *Client) sendRequest(url string, data interface{}) (map[string]interface{}, error) {
	bodyBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	randStr := randomString(16)
	sign := calSign(string(bodyBytes), ts, randStr)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// 所有必需的 headers（与浏览器抓包完全一致）
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Authorization", c.Authorization)
	req.Header.Set("CMS-DEVICE", "default")
	req.Header.Set("caller", "web")

	// mcloud 签名头
	req.Header.Set("mcloud-channel", "1000101")
	req.Header.Set("mcloud-client", ClientID)
	req.Header.Set("mcloud-route", "001")
	req.Header.Set("mcloud-sign", fmt.Sprintf("%s,%s,%s", ts, randStr, sign))
	req.Header.Set("mcloud-version", AppVersion)

	// x-yun 系列头（之前缺失的关键 headers）
	req.Header.Set("x-yun-api-version", "v1")
	req.Header.Set("x-yun-app-channel", "10000034")
	req.Header.Set("x-yun-channel-source", "10000034")
	req.Header.Set("x-yun-client-info", ClientInfoFull)
	req.Header.Set("x-yun-module-type", "100")
	req.Header.Set("x-yun-svc-type", "1")

	// 其他必需头
	req.Header.Set("x-DeviceInfo", DeviceInfo)
	req.Header.Set("x-huawei-channelSrc", "10000034")
	req.Header.Set("x-inner-ntwk", "2")
	req.Header.Set("x-m4c-caller", "PC")
	req.Header.Set("x-m4c-src", "10002")
	req.Header.Set("x-SvcType", "1")
	req.Header.Set("Inner-Hcy-Router-Https", "1")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response failed")
	}

	return result, nil
}

// ========== 文件操作 ==========

// FileItem 文件/目录项
type FileItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Type      string `json:"type"` // "file" or "folder"
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// FileListResponse 文件列表响应
type FileListResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Items []struct {
			FileId       string `json:"fileId"`
			ParentFileId string `json:"parentFileId"`
			Name         string `json:"name"`
			Type         string `json:"type"` // "folder" or "file"
			Category     string `json:"category"`
			Size         int64  `json:"size"`
			CreatedAt    string `json:"createdAt"`
			UpdatedAt    string `json:"updatedAt"`
			DownloadUrl  string `json:"downloadUrl"`
			ThumbnailUrl string `json:"thumbnailUrl"`
			FileExtension string `json:"fileExtension"`
			SystemDir    bool   `json:"systemDir"`
		} `json:"items"`
		NextPageCursor *string `json:"nextPageCursor"`
	} `json:"data"`
}

// ListFiles 列出文件
func (c *Client) ListFiles(parentFileID string, pageCursor string) (*FileListResponse, error) {
	if parentFileID == "" {
		parentFileID = "/"
	}

	data := map[string]interface{}{
		"parentFileId": parentFileID,
		"pageInfo": map[string]interface{}{
			"pageSize":   100,
			"pageCursor": pageCursor,
		},
		"orderBy":                  "updated_at",
		"orderDirection":           "DESC",
		"imageThumbnailStyleList": []string{"Small", "Middle", "Big", "Large"},
	}

	resp, err := c.sendRequest(BaseURL+"/file/list", data)
	if err != nil {
		return nil, err
	}

	// 检查响应
	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		code, _ := resp["code"].(string)
		return nil, fmt.Errorf("API error %s: %s", code, msg)
	}

	// 转换为 FileListResponse
	respBytes, _ := json.Marshal(resp)
	var result FileListResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

// Mkdir 创建文件夹
func (c *Client) Mkdir(parentFileID, name string) error {
	data := map[string]interface{}{
		"parentFileId":   parentFileID,
		"name":           name,
		"description":    "",
		"type":           "folder",
		"fileRenameMode": "force_rename",
	}

	resp, err := c.sendRequest(BaseURL+"/file/create", data)
	if err != nil {
		return err
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("mkdir failed: %s", msg)
	}

	return nil
}

// DeleteFiles 删除文件
func (c *Client) DeleteFiles(fileIDs []string) error {
	data := map[string]interface{}{
		"fileIds": fileIDs,
	}

	resp, err := c.sendRequest(BaseURL+"/file/batchDelete", data)
	if err != nil {
		return err
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("delete failed: %s", msg)
	}

	return nil
}

// GetDownloadUrl 获取下载链接
func (c *Client) GetDownloadUrl(fileID string) (string, error) {
	data := map[string]interface{}{
		"fileId": fileID,
	}

	resp, err := c.sendRequest(BaseURL+"/file/getDownloadUrl", data)
	if err != nil {
		return "", err
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return "", fmt.Errorf("get download url failed: %s", msg)
	}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		// 尝试多个可能的字段名
		if url, ok := data["cdnUrl"].(string); ok && url != "" {
			return url, nil
		}
		if url, ok := data["url"].(string); ok && url != "" {
			return url, nil
		}
		if url, ok := data["downloadUrl"].(string); ok && url != "" {
			return url, nil
		}
		if url, ok := data["downloadURL"].(string); ok && url != "" {
			return url, nil
		}
	}

	return "", fmt.Errorf("no download url in response: %v", resp)
}

// RenameFile 重命名文件
func (c *Client) RenameFile(fileID, newName string) error {
	data := map[string]interface{}{
		"fileId":   fileID,
		"fileName": newName,
	}

	resp, err := c.sendRequest(BaseURL+"/file/update", data)
	if err != nil {
		return err
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("rename failed: %s", msg)
	}

	return nil
}

// MoveFiles 移动文件
func (c *Client) MoveFiles(fileIDs []string, toParentFileID string) error {
	data := map[string]interface{}{
		"fileIds":        fileIDs,
		"toParentFileId": toParentFileID,
	}

	resp, err := c.sendRequest(BaseURL+"/file/batchMove", data)
	if err != nil {
		return err
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("move failed: %s", msg)
	}

	return nil
}

// UploadFile 上传文件
func (c *Client) UploadFile(parentFileID, name string, reader io.Reader, size int64) error {
	// 读取文件内容
	fileData, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// 计算 SHA256
	hash := sha256.Sum256(fileData)
	fullHash := hex.EncodeToString(hash[:])

	// 创建文件记录
	createData := map[string]interface{}{
		"parentFileId":         parentFileID,
		"name":                 name,
		"type":                 "file",
		"size":                 size,
		"contentHash":          fullHash,
		"contentHashAlgorithm": "SHA256",
		"contentType":          "application/octet-stream",
		"parallelUpload":       false,
		"fileRenameMode":       "auto_rename",
		"partInfos": []map[string]interface{}{
			{
				"partNumber":  1,
				"partSize":    size,
				"contentHash": fullHash,
			},
		},
	}

	resp, err := c.sendRequest(BaseURL+"/file/create", createData)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("create failed: %s", msg)
	}

	// 检查是否需要上传
	if respData, ok := resp["data"].(map[string]interface{}); ok {
		// 文件已存在（秒传成功）
		if exist, ok := respData["exist"].(bool); ok && exist {
			return nil
		}

		// 快传成功
		if rapidUpload, ok := respData["rapidUpload"].(bool); ok && rapidUpload {
			return nil
		}

		fileId, _ := respData["fileId"].(string)
		uploadId, _ := respData["uploadId"].(string)

		// 获取上传URL并上传分片
		if partInfos, ok := respData["partInfos"].([]interface{}); ok && len(partInfos) > 0 {
			for _, pi := range partInfos {
				partInfo, ok := pi.(map[string]interface{})
				if !ok {
					continue
				}
				uploadUrl, _ := partInfo["uploadUrl"].(string)
				if uploadUrl == "" {
					continue
				}

				// 从 URL 中提取 uploadId（如果响应中没有）
				if uploadId == "" {
					uploadId = extractUploadId(uploadUrl)
				}

				// 上传分片
				req, err := http.NewRequest("PUT", uploadUrl, bytes.NewReader(fileData))
				if err != nil {
					return fmt.Errorf("create upload request: %w", err)
				}
				req.Header.Set("Content-Type", "application/octet-stream")

				httpClient := &http.Client{Timeout: 30 * time.Minute}
				uploadResp, err := httpClient.Do(req)
				if err != nil {
					return fmt.Errorf("upload part: %w", err)
				}
				uploadResp.Body.Close()

				if uploadResp.StatusCode >= 400 {
					return fmt.Errorf("upload failed: status %d", uploadResp.StatusCode)
				}
			}
		}

		// 调用 /file/complete 完成上传
		if fileId != "" && uploadId != "" {
			completeData := map[string]interface{}{
				"contentHash":          fullHash,
				"contentHashAlgorithm": "SHA256",
				"fileId":               fileId,
				"uploadId":             uploadId,
			}
			_, err := c.sendRequest(BaseURL+"/file/complete", completeData)
			if err != nil {
				return fmt.Errorf("complete upload: %w", err)
			}
		}
	}

	return nil
}

// extractUploadId 从 COS 上传 URL 中提取 uploadId
func extractUploadId(uploadUrl string) string {
	u, err := url.Parse(uploadUrl)
	if err != nil {
		return ""
	}
	return u.Query().Get("uploadId")
}

// UploadResult 上传结果
type UploadResult struct {
	FileId   string
	OrigName string // 上传时使用的原始名
	Exist    bool   // 秒传/快传
}

// UploadFileEncrypted 上传加密文件（调用方已完成加密），返回上传结果
func (c *Client) UploadFileEncrypted(parentFileID, name string, encReader io.Reader, encSize int64, plainHash string) (*UploadResult, error) {
	// 读取加密数据
	encData, err := io.ReadAll(encReader)
	if err != nil {
		return nil, fmt.Errorf("read encrypted data: %w", err)
	}

	// 计算密文 hash（用于 API 的 contentHash 验证）
	encHash := sha256.Sum256(encData)
	encHashStr := hex.EncodeToString(encHash[:])

	// 创建文件记录
	createData := map[string]interface{}{
		"parentFileId":         parentFileID,
		"name":                 name,
		"type":                 "file",
		"size":                 encSize,
		"contentHash":          encHashStr,
		"contentHashAlgorithm": "SHA256",
		"contentType":          "application/octet-stream",
		"parallelUpload":       false,
		"fileRenameMode":       "auto_rename",
		"partInfos": []map[string]interface{}{
			{
				"partNumber":  1,
				"partSize":    encSize,
				"contentHash": encHashStr,
			},
		},
	}

	resp, err := c.sendRequest(BaseURL+"/file/create", createData)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return nil, fmt.Errorf("create failed: %s", msg)
	}

	result := &UploadResult{OrigName: name}

	if respData, ok := resp["data"].(map[string]interface{}); ok {
		if exist, ok := respData["exist"].(bool); ok && exist {
			result.Exist = true
			return result, nil
		}
		if rapidUpload, ok := respData["rapidUpload"].(bool); ok && rapidUpload {
			result.Exist = true
			return result, nil
		}

		fileId, _ := respData["fileId"].(string)
		uploadId, _ := respData["uploadId"].(string)
		result.FileId = fileId

		if partInfos, ok := respData["partInfos"].([]interface{}); ok && len(partInfos) > 0 {
			for _, pi := range partInfos {
				partInfo, ok := pi.(map[string]interface{})
				if !ok {
					continue
				}
				uploadUrl, _ := partInfo["uploadUrl"].(string)
				if uploadUrl == "" {
					continue
				}
				if uploadId == "" {
					uploadId = extractUploadId(uploadUrl)
				}

				// 上传加密数据到 COS
				req, err := http.NewRequest("PUT", uploadUrl, bytes.NewReader(encData))
				if err != nil {
					return nil, fmt.Errorf("create upload request: %w", err)
				}
				req.Header.Set("Content-Type", "application/octet-stream")

				httpClient := &http.Client{Timeout: 30 * time.Minute}
				uploadResp, err := httpClient.Do(req)
				if err != nil {
					return nil, fmt.Errorf("upload part: %w", err)
				}
				uploadResp.Body.Close()

				if uploadResp.StatusCode >= 400 {
					return nil, fmt.Errorf("upload failed: status %d", uploadResp.StatusCode)
				}
			}
		}

		// 完成上传（使用密文的 hash，因为 COS 上是密文）
		if fileId != "" && uploadId != "" {
			log.Printf("UploadFileEncrypted: completing fileId=%s uploadId=%s", fileId, uploadId)
			completeData := map[string]interface{}{
				"contentHash":          encHashStr,
				"contentHashAlgorithm": "SHA256",
				"fileId":               fileId,
				"uploadId":             uploadId,
			}
			_, err := c.sendRequest(BaseURL+"/file/complete", completeData)
			if err != nil {
				return nil, fmt.Errorf("complete upload: %w", err)
			}
		}
	}

	return result, nil
}

// DownloadFile 下载文件到 writer（流式）
func (c *Client) DownloadFile(fileID string, w io.Writer) (int64, error) {
	downloadUrl, err := c.GetDownloadUrl(fileID)
	if err != nil {
		return 0, err
	}

	httpClient := &http.Client{Timeout: 30 * time.Minute}
	resp, err := httpClient.Get(downloadUrl)
	if err != nil {
		return 0, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	return io.Copy(w, resp.Body)
}

// CreateFolder 创建文件夹
func (c *Client) CreateFolder(parentFileID, name string) error {
	data := map[string]interface{}{
		"parentFileId":   parentFileID,
		"name":           name,
		"description":    "",
		"type":           "folder",
		"fileRenameMode": "force_rename",
	}

	resp, err := c.sendRequest(BaseURL+"/file/create", data)
	if err != nil {
		return err
	}

	if success, ok := resp["success"].(bool); ok && !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("create folder failed: %s", msg)
	}

	return nil
}

// ListAllFiles 列出所有文件（分页遍历）
func (c *Client) ListAllFiles(parentFileID string) ([]FileItem, error) {
	if parentFileID == "" {
		parentFileID = "/"
	}

	var allFiles []FileItem
	cursor := ""

	for {
		resp, err := c.ListFiles(parentFileID, cursor)
		if err != nil {
			return nil, err
		}

		for _, item := range resp.Data.Items {
			allFiles = append(allFiles, FileItem{
				ID:        item.FileId,
				Name:      item.Name,
				Size:      item.Size,
				Type:      item.Type,
				CreatedAt: item.CreatedAt,
				UpdatedAt: item.UpdatedAt,
			})
		}

		if resp.Data.NextPageCursor == nil || *resp.Data.NextPageCursor == "" {
			break
		}
		cursor = *resp.Data.NextPageCursor
	}

	return allFiles, nil
}

// ========== 路由查询 ==========

// QueryRoute 查询路由策略
func (c *Client) QueryRoute() (string, error) {
	data := map[string]interface{}{
		"userInfo": map[string]interface{}{
			"userType":    1,
			"accountType": 1,
			"accountName": c.Account,
		},
		"modAddrType": 1,
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	randStr := randomString(16)
	bodyBytes, _ := json.Marshal(data)
	sign := calSign(string(bodyBytes), ts, randStr)

	req, err := http.NewRequest("POST", UserBaseURL+"/route/qryRoutePolicy", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.Authorization)
	req.Header.Set("mcloud-sign", fmt.Sprintf("%s,%s,%s", ts, randStr, sign))
	req.Header.Set("mcloud-channel", "1000101")
	req.Header.Set("mcloud-client", ClientID)
	req.Header.Set("mcloud-version", AppVersion)
	req.Header.Set("CMS-DEVICE", "default")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if data, ok := result["data"].(map[string]interface{}); ok {
		if list, ok := data["routePolicyList"].([]interface{}); ok {
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					if m["modName"] == "personal" {
						if url, ok := m["httpsUrl"].(string); ok {
							return strings.TrimRight(url, "/"), nil
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("personal route not found")
}

// ========== Token 刷新 ==========

// RefreshToken 刷新 Token
func (c *Client) RefreshToken() (string, error) {
	if c.Token == "" {
		return "", fmt.Errorf("no token to refresh")
	}

	xmlBody := fmt.Sprintf(`<root><token>%s</token><account>%s</account><clienttype>656</clienttype></root>`, c.Token, c.Account)

	req, err := http.NewRequest("POST", "https://aas.caiyun.feixin.10086.cn:443/tellin/authTokenRefresh.do",
		strings.NewReader(xmlBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 简单解析 XML
	bodyStr := string(body)
	if strings.Contains(bodyStr, "<return>0</return>") {
		// 提取新 token
		start := strings.Index(bodyStr, "<token>") + 7
		end := strings.Index(bodyStr, "</token>")
		if start > 6 && end > start {
			newToken := bodyStr[start:end]
			// 更新 Authorization
			newAuth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("pc:%s:%s", c.Account, newToken)))
			c.Token = newToken
			c.Authorization = "Basic " + newAuth
			return c.Authorization, nil
		}
	}

	return "", fmt.Errorf("refresh token failed: %s", bodyStr[:min(len(bodyStr), 200)])
}

// ========== 工具函数 ==========

// calSign 计算 mcloud-sign（与浏览器和 AList 一致）
func calSign(body, ts, randStr string) string {
	body = encodeURIComponent(body)
	strs := strings.Split(body, "")
	sort.Strings(strs)
	body = strings.Join(strs, "")
	body = base64.StdEncoding.EncodeToString([]byte(body))

	h1 := md5.Sum([]byte(body))
	h2 := md5.Sum([]byte(ts + ":" + randStr))
	res := md5.Sum([]byte(hex.EncodeToString(h1[:]) + hex.EncodeToString(h2[:])))
	return strings.ToUpper(hex.EncodeToString(res[:]))
}

func encodeURIComponent(str string) string {
	r := url.QueryEscape(str)
	r = strings.ReplaceAll(r, "+", "%20")
	r = strings.ReplaceAll(r, "%21", "!")
	r = strings.ReplaceAll(r, "%27", "'")
	r = strings.ReplaceAll(r, "%28", "(")
	r = strings.ReplaceAll(r, "%29", ")")
	r = strings.ReplaceAll(r, "%2A", "*")
	return r
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
