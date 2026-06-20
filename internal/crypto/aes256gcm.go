package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// NonceSize GCM nonce 大小
	NonceSize = 12
	// KeySize AES-256 密钥大小
	KeySize = 32
	// SaltSize salt 大小
	SaltSize = 16
)

// Encryptor AES-256-GCM 加密器
type Encryptor struct {
	key []byte
}

// NewEncryptor 创建加密器
// saltHex 为空时自动生成，返回值为生成的 salt hex
func NewEncryptor(password, saltHex string, iterations int) (*Encryptor, string, error) {
	if password == "" {
		return nil, "", fmt.Errorf("password is required")
	}
	if iterations <= 0 {
		iterations = 100000
	}

	var salt []byte
	generatedSalt := ""
	if saltHex == "" {
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, "", fmt.Errorf("generate salt: %w", err)
		}
		generatedSalt = hex.EncodeToString(salt)
	} else {
		var err error
		salt, err = hex.DecodeString(saltHex)
		if err != nil {
			return nil, "", fmt.Errorf("decode salt: %w", err)
		}
	}

	key := pbkdf2.Key([]byte(password), salt, iterations, KeySize, sha256.New)

	return &Encryptor{key: key}, generatedSalt, nil
}

// Encrypt 加密数据，返回 [nonce][ciphertext][tag]
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	result := make([]byte, 0, len(nonce)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

// Decrypt 解密 [nonce][ciphertext][tag] 数据
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	minSize := gcm.NonceSize() + gcm.Overhead()
	if len(data) < minSize {
		return nil, fmt.Errorf("ciphertext too short: %d < %d", len(data), minSize)
	}

	nonce := data[:gcm.NonceSize()]
	ciphertext := data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed (wrong key or corrupted data): %w", err)
	}

	return plaintext, nil
}

// ========== 名称加密 ==========

// nameDeterministicNonce 从名称派生确定性 nonce（相同名称总是产生相同 nonce）
func (e *Encryptor) nameDeterministicNonce(name string) []byte {
	h := sha256.Sum256([]byte(name + "_name_nonce"))
	return h[:NonceSize]
}

// EncryptName 加密名称，返回 nonce + ciphertext + tag 的 hex
// 同一名称总是产生相同结果（确定性 nonce）
func (e *Encryptor) EncryptName(name string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := e.nameDeterministicNonce(name)
	ciphertext := gcm.Seal(nil, nonce, []byte(name), nil)
	// 存储 nonce + ciphertext（含 tag），便于解密
	result := make([]byte, 0, len(nonce)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return hex.EncodeToString(result), nil
}

// DecryptName 解密名称（从 nonce + ciphertext + tag 的 hex）
func (e *Encryptor) DecryptName(encryptedHex string) (string, error) {
	data, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	minSize := gcm.NonceSize() + gcm.Overhead()
	if len(data) < minSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce := data[:gcm.NonceSize()]
	ciphertext := data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// EncNameForFile 加密文件名，加 .bin 后缀
// "report.pdf" → "E_1da74a6a...83a9cae42bd49.bin"
func (e *Encryptor) EncNameForFile(name string) string {
	enc, err := e.EncryptName(name)
	if err != nil {
		return name
	}
	return "E_" + enc + ".bin"
}

// EncNameForFolder 加密文件夹名
// "Documents" → "E_a7f3e2b1...f0a9b8"
func (e *Encryptor) EncNameForFolder(name string) string {
	enc, err := e.EncryptName(name)
	if err != nil {
		return name
	}
	return "E_" + enc
}

// DecNameForFile 解密文件名（去掉 E_ 前缀和 .bin 后缀）
func (e *Encryptor) DecNameForFile(cloudName string) (string, bool) {
	if !strings.HasPrefix(cloudName, "E_") || !strings.HasSuffix(cloudName, ".bin") {
		return "", false
	}
	hexPart := strings.TrimPrefix(cloudName, "E_")
	hexPart = strings.TrimSuffix(hexPart, ".bin")
	orig, err := e.DecryptName(hexPart)
	if err != nil {
		return "", false
	}
	return orig, true
}

// DecNameForFolder 解密文件夹名（去掉 E_ 前缀）
func (e *Encryptor) DecNameForFolder(cloudName string) (string, bool) {
	if !strings.HasPrefix(cloudName, "E_") {
		return "", false
	}
	hexPart := strings.TrimPrefix(cloudName, "E_")
	orig, err := e.DecryptName(hexPart)
	if err != nil {
		return "", false
	}
	return orig, true
}

// IsEncName 检查是否为新版加密名称（E_ 前缀）
func IsEncName(name string) bool {
	return strings.HasPrefix(name, "E_")
}

// IsLegacyEncName 检查是否为旧版加密名称（_enc / .enc 后缀）
func IsLegacyEncName(name string, isFolder bool) bool {
	if isFolder {
		return strings.HasSuffix(name, "_enc")
	}
	return strings.HasSuffix(name, ".enc")
}

// ========== 文件加密 ==========

// EncryptReader 从 reader 读取明文，加密后写入 writer
func (e *Encryptor) EncryptReader(r io.Reader, w io.Writer) (int64, error) {
	plaintext, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read plaintext: %w", err)
	}
	encrypted, err := e.Encrypt(plaintext)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(encrypted)
	return int64(n), err
}

// DecryptReader 从 reader 读取密文，解密后写入 writer
func (e *Encryptor) DecryptReader(r io.Reader, w io.Writer) (int64, error) {
	ciphertext, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read ciphertext: %w", err)
	}
	plaintext, err := e.Decrypt(ciphertext)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(plaintext)
	return int64(n), err
}

// PlainTextHash 计算明文的 SHA256
func PlainTextHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// EncryptedSize 计算加密后的大小（含 nonce + tag 开销）
func EncryptedSize(plainSize int64) int64 {
	return plainSize + NonceSize + 16 // 12 nonce + 16 GCM tag
}
