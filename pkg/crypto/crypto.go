package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// EncryptedPrefix 加密密文前缀标识，用于区分明文与密文
	EncryptedPrefix = "ENC:"
	// SecretKeyFileName 密钥文件名
	SecretKeyFileName = ".secret_key"
)

var (
	globalCipher *AESCipher
	cipherOnce   sync.Once
)

// AESCipher AES-256-GCM 加密器
type AESCipher struct {
	key []byte
}

// InitGlobalCipher 初始化全局加密器，存储在 dataDir/configs/.secret_key
func InitGlobalCipher(dataDir string) error {
	var initErr error
	cipherOnce.Do(func() {
		keyPath := filepath.Join(dataDir, "configs", SecretKeyFileName)
		key, err := getOrCreateKey(keyPath)
		if err != nil {
			initErr = fmt.Errorf("初始化密钥失败: %w", err)
			return
		}
		c, err := NewAESCipher(key)
		if err != nil {
			initErr = fmt.Errorf("创建 AES 加密器失败: %w", err)
			return
		}
		globalCipher = c
	})
	return initErr
}

// GetGlobalCipher 获取全局加密器实例
func GetGlobalCipher() *AESCipher {
	return globalCipher
}

// NewAESCipher 创建指定密钥的 AESCipher 实例（密钥长度必须为 32 字节对应 AES-256）
func NewAESCipher(key []byte) (*AESCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("AES-256 密钥长度必须严格为 32 字节")
	}
	return &AESCipher{key: key}, nil
}

// Encrypt 将明文字符串使用 AES-GCM 加密并返回 ENC:base64 格式
func (c *AESCipher) Encrypt(plainText string) (string, error) {
	if plainText == "" {
		return "", nil
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", fmt.Errorf("创建 AES 密码块失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 模式失败: %w", err)
	}

	// 生成随机 Nonce (12 字节)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机 Nonce 失败: %w", err)
	}

	// 执行加密并拼接 Nonce + CipherText
	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	encoded := base64.StdEncoding.EncodeToString(cipherText)
	return EncryptedPrefix + encoded, nil
}

// Decrypt 将 ENC:base64 格式密文解密还原为明文；若未包含 ENC: 前缀则视为明文原样返回
func (c *AESCipher) Decrypt(cipherText string) (string, error) {
	if cipherText == "" {
		return "", nil
	}

	// 如果没有加密前缀，兼容直接按明文处理
	if !strings.HasPrefix(cipherText, EncryptedPrefix) {
		return cipherText, nil
	}

	rawBase64 := strings.TrimPrefix(cipherText, EncryptedPrefix)
	data, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", fmt.Errorf("创建 AES 密码块失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 模式失败: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("密文数据长度小于 Nonce 长度，数据损坏")
	}

	nonce, actualCipher := data[:nonceSize], data[nonceSize:]
	plainBytes, err := gcm.Open(nil, nonce, actualCipher, nil)
	if err != nil {
		return "", fmt.Errorf("AES-GCM 解密认证失败: %w", err)
	}

	return string(plainBytes), nil
}

// IsEncrypted 判断字符串是否已经是加密密文
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, EncryptedPrefix)
}

// getOrCreateKey 读取已存在的持久化密钥文件；若不存在则自动生成 32 字节高熵随机密钥
func getOrCreateKey(keyPath string) ([]byte, error) {
	if _, err := os.Stat(keyPath); err == nil {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		if len(key) == 32 {
			return key, nil
		}
		// 若已有密钥长度异常，重新生成保护安全
	}

	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, err
	}

	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, fmt.Errorf("读取系统安全随机源失败: %w", err)
	}

	if err := os.WriteFile(keyPath, newKey, 0600); err != nil {
		return nil, fmt.Errorf("写入密钥文件失败: %w", err)
	}

	return newKey, nil
}
