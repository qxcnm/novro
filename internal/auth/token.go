package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const sessionTokenBytes = 32

/**
 * newSessionToken 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func newSessionToken() (string, error) {
	random := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return "nvs_" + base64.RawURLEncoding.EncodeToString(random), nil
}

/**
 * hashSessionToken 封装该名称对应的业务处理逻辑。
 * @param secret 本次操作需要使用的输入参数。
 * @param token 用于认证或继续操作的令牌。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func hashSessionToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
