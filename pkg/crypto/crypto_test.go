package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAESCipher(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	cipher, err := NewAESCipher(key)
	if err != nil {
		t.Fatalf("创建加密器失败: %v", err)
	}

	rawPass := "MyComplex#Pass123_@!"

	encrypted, err := cipher.Encrypt(rawPass)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	if !IsEncrypted(encrypted) {
		t.Errorf("加密后未包含 ENC: 前缀: %s", encrypted)
	}

	if encrypted == rawPass {
		t.Errorf("密文与明文相同")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	if decrypted != rawPass {
		t.Errorf("解密结果不匹配: got %s, want %s", decrypted, rawPass)
	}

	// 测试未加密的明文原样返回
	plain, err := cipher.Decrypt("PlainOldPassword")
	if err != nil {
		t.Fatalf("明文解密失败: %v", err)
	}
	if plain != "PlainOldPassword" {
		t.Errorf("明文应当原样返回，实际为: %s", plain)
	}
}

func TestInitGlobalCipher(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mysql-crypto-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if err := InitGlobalCipher(tempDir); err != nil {
		t.Fatalf("初始化全局加密器失败: %v", err)
	}

	c := GetGlobalCipher()
	if c == nil {
		t.Fatalf("GetGlobalCipher 返回 nil")
	}

	// 验证密钥文件生成
	keyPath := filepath.Join(tempDir, "configs", SecretKeyFileName)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatalf("未生成密钥文件 %s", keyPath)
	}
}
