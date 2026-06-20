package metadata

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// FileInfo 文件加密元数据
type FileInfo struct {
	CloudFileID   string    `db:"cloud_file_id"`
	ParentCloudID string    `db:"parent_cloud_id"`
	CloudName     string    `db:"cloud_name"`
	OrigName      string    `db:"orig_name"`
	OrigPath      string    `db:"orig_path"`
	Type          string    `db:"type"`           // "file" or "folder"
	OrigSize      int64     `db:"orig_size"`
	EncSize       int64     `db:"enc_size"`
	Nonce         []byte    `db:"nonce"`
	ContentHash   string    `db:"content_hash"`
	IsEncrypted   bool      `db:"is_encrypted"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// Store SQLite 元数据存储
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore 创建或打开 SQLite 存储
func NewStore(dbPath string) (*Store, error) {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// 设置 PRAGMA
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	log.Printf("Metadata store initialized: %s", dbPath)
	return s, nil
}

func (s *Store) migrate() error {
	// 逐条执行 schema（部分驱动不支持多语句 Exec）
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS files (
			cloud_file_id   TEXT PRIMARY KEY,
			parent_cloud_id TEXT NOT NULL DEFAULT '/',
			cloud_name      TEXT NOT NULL,
			orig_name       TEXT NOT NULL,
			orig_path       TEXT NOT NULL DEFAULT '',
			type            TEXT NOT NULL DEFAULT 'file',
			orig_size       INTEGER NOT NULL DEFAULT 0,
			enc_size        INTEGER NOT NULL DEFAULT 0,
			nonce           BLOB,
			content_hash    TEXT NOT NULL DEFAULT '',
			is_encrypted    INTEGER NOT NULL DEFAULT 1,
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_files_parent ON files(parent_cloud_id)`,
		`CREATE INDEX IF NOT EXISTS idx_files_orig_path ON files(orig_path)`,
		`CREATE TABLE IF NOT EXISTS kv_store (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:50], err)
		}
	}
	return nil
}

// PutFile 添加或更新文件元数据
func (s *Store) PutFile(info *FileInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO files
		(cloud_file_id, parent_cloud_id, cloud_name, orig_name, orig_path, type,
		 orig_size, enc_size, nonce, content_hash, is_encrypted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		info.CloudFileID, info.ParentCloudID, info.CloudName, info.OrigName, info.OrigPath, info.Type,
		info.OrigSize, info.EncSize, info.Nonce, info.ContentHash, info.IsEncrypted,
		info.CreatedAt, info.UpdatedAt,
	)
	return err
}

// GetFileByID 按 cloud_file_id 查询
func (s *Store) GetFileByID(cloudFileID string) (*FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := &FileInfo{}
	err := s.db.QueryRow(`
		SELECT cloud_file_id, parent_cloud_id, cloud_name, orig_name, orig_path, type,
		       orig_size, enc_size, nonce, content_hash, is_encrypted, created_at, updated_at
		FROM files WHERE cloud_file_id = ?
	`, cloudFileID).Scan(
		&info.CloudFileID, &info.ParentCloudID, &info.CloudName, &info.OrigName, &info.OrigPath, &info.Type,
		&info.OrigSize, &info.EncSize, &info.Nonce, &info.ContentHash, &info.IsEncrypted,
		&info.CreatedAt, &info.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return info, nil
}

// GetFilesByParent 按父目录查询所有文件
func (s *Store) GetFilesByParent(parentCloudID string) ([]*FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT cloud_file_id, parent_cloud_id, cloud_name, orig_name, orig_path, type,
		       orig_size, enc_size, nonce, content_hash, is_encrypted, created_at, updated_at
		FROM files WHERE parent_cloud_id = ?
	`, parentCloudID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*FileInfo
	for rows.Next() {
		info := &FileInfo{}
		if err := rows.Scan(
			&info.CloudFileID, &info.ParentCloudID, &info.CloudName, &info.OrigName, &info.OrigPath, &info.Type,
			&info.OrigSize, &info.EncSize, &info.Nonce, &info.ContentHash, &info.IsEncrypted,
			&info.CreatedAt, &info.UpdatedAt,
		); err != nil {
			return nil, err
		}
		files = append(files, info)
	}
	return files, nil
}

// DeleteFile 删除文件元数据
func (s *Store) DeleteFile(cloudFileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM files WHERE cloud_file_id = ?", cloudFileID)
	return err
}

// DeleteFiles 批量删除
func (s *Store) DeleteFiles(cloudFileIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range cloudFileIDs {
		if _, err := s.db.Exec("DELETE FROM files WHERE cloud_file_id = ?", id); err != nil {
			return err
		}
	}
	return nil
}

// Stats 统计信息
func (s *Store) Stats() (fileCount int, totalOrigSize, totalEncSize int64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(orig_size), 0), COALESCE(SUM(enc_size), 0)
		FROM files WHERE is_encrypted = 1
	`).Scan(&fileCount, &totalOrigSize, &totalEncSize)
	return
}

// GetConfig 获取配置值
func (s *Store) GetConfig(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value string
	err := s.db.QueryRow("SELECT value FROM kv_store WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetConfig 设置配置值
func (s *Store) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO kv_store (key, value) VALUES (?, ?)", key, value)
	return err
}

// Close 关闭数据库
func (s *Store) Close() error {
	return s.db.Close()
}

// Count 文件总数
func (s *Store) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	return count, err
}
