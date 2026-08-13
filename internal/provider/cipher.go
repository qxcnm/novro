package provider

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

const encryptedValueVersion = "v1."

type Cipher struct {
	aead cipher.AEAD
}

/**
 * NewCipher 用于创建并返回所需的对象或记录。
 * @param secret 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewCipher(secret string) (*Cipher, error) {
	key := sha256.Sum256([]byte("novro/provider/v1\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create provider cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create provider AEAD: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

/**
 * Encrypt 用于对敏感数据执行安全转换。
 * @param plainText 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c *Cipher) Encrypt(plainText string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate provider nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plainText), []byte(encryptedValueVersion))
	payload := append(nonce, sealed...)
	return encryptedValueVersion + base64.RawURLEncoding.EncodeToString(payload), nil
}

/**
 * Decrypt 用于解密并返回受保护的数据。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c *Cipher) Decrypt(value string) (string, error) {
	if len(value) <= len(encryptedValueVersion) || value[:len(encryptedValueVersion)] != encryptedValueVersion {
		return "", ErrInvalidInput
	}
	payload, err := base64.RawURLEncoding.DecodeString(value[len(encryptedValueVersion):])
	if err != nil || len(payload) < c.aead.NonceSize() {
		return "", ErrInvalidInput
	}
	nonce, sealed := payload[:c.aead.NonceSize()], payload[c.aead.NonceSize():]
	plainText, err := c.aead.Open(nil, nonce, sealed, []byte(encryptedValueVersion))
	if err != nil {
		return "", fmt.Errorf("decrypt provider credential: %w", err)
	}
	return string(plainText), nil
}
