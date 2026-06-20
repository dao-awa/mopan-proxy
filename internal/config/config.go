package config

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// 服务器
	Port   int    `yaml:"port"`
	Host   string `yaml:"host"`
	Debug  bool   `yaml:"debug"`

	// 和彩云认证
	Account      string `yaml:"account"`       // 手机号
	Authorization string `yaml:"authorization"` // Base64("pc:phone:token")

	// WebDAV
	WebDAV struct {
		Enabled  bool   `yaml:"enabled"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"webdav"`

	// 加密
	Crypto struct {
		Enabled    bool   `yaml:"enabled"`
		Password   string `yaml:"password"`
		SaltHex    string `yaml:"salt_hex"`
		Iterations int    `yaml:"iterations"`
	} `yaml:"crypto"`

	// 存储
	Storage struct {
		CacheDir   string `yaml:"cache_dir"`
		MetaPath   string `yaml:"meta_path"`
		MaxUpload  int64  `yaml:"max_upload"`
		RootFolder string `yaml:"root_folder"` // 和彩云根目录（如 /MopanProxy）
	} `yaml:"storage"`

	// Token 刷新
	Refresh struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"refresh"`
}

func Default() *Config {
	cfg := &Config{
		Port:  9090,
		Host:  "0.0.0.0",
		Debug: false,
	}
	cfg.WebDAV.Enabled = true
	cfg.WebDAV.Port = 9091
	cfg.WebDAV.Username = "admin"
	cfg.WebDAV.Password = "admin"
	cfg.Crypto.Enabled = true
	cfg.Crypto.Password = "mopan-default-password"
	cfg.Crypto.Iterations = 100000
	cfg.Storage.CacheDir = "./data/cache"
	cfg.Storage.MetaPath = "./data/metadata.db"
	cfg.Storage.MaxUpload = 10 * 1024 * 1024 * 1024 // 10GB
	cfg.Storage.RootFolder = "/MopanProxy"
	cfg.Refresh.Enabled = true
	return cfg
}

func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 首次启动，生成 salt
			if cfg.Crypto.SaltHex == "" {
				salt := make([]byte, 16)
				if _, err := crypto_rand.Read(salt); err != nil {
					return nil, fmt.Errorf("generate salt: %w", err)
				}
				cfg.Crypto.SaltHex = hex.EncodeToString(salt)
			}
			return cfg, cfg.Save(path)
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// 自动生成 salt（首次启用加密时）
	if cfg.Crypto.Enabled && cfg.Crypto.SaltHex == "" {
		salt := make([]byte, 16)
		if _, err := crypto_rand.Read(salt); err != nil {
			return nil, fmt.Errorf("generate salt: %w", err)
		}
		cfg.Crypto.SaltHex = hex.EncodeToString(salt)
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
