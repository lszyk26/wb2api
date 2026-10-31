package main

// crypto.go — 上游凭据与敏感字段的 AES-256-GCM 加解密
// 格式：enc:v1:<base64(nonce+ciphertext)>

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"strings"
)

var encKey []byte

func initEncryption(src string) {
	if b, err := hex.DecodeString(strings.TrimSpace(src)); err == nil && len(b) == 32 {
		encKey = b
		return
	}
	sum := sha256.Sum256([]byte(src))
	encKey = sum[:]
}

func isEncrypted(s string) bool { return strings.HasPrefix(s, "enc:v1:") }

func encryptor() (cipher.AEAD, error) {
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func encrypt(plain string) string {
	if plain == "" || isEncrypted(plain) {
		return plain
	}
	aead, err := encryptor()
	if err != nil {
		log.Printf("[crypto] init: %v", err)
		return plain
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plain
	}
	sealed := aead.Seal(nonce, nonce, []byte(plain), nil)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(sealed)
}

func decrypt(s string) (string, error) {
	if !isEncrypted(s) {
		return s, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "enc:v1:"))
	if err != nil {
		return "", err
	}
	aead, err := encryptor()
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// 只展示头尾，用于前端展示
func maskSecret(s string) string {
	if len(s) <= 14 {
		return "••••"
	}
	return s[:8] + "••••" + s[len(s)-4:]
}
