package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"open-call/internal/errs"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword 使用 argon2id 生成 PHC 格式哈希。
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// ValidatePassword 校验所有新建或重置凭证使用的最低密码强度。
func ValidatePassword(password string) error {
	if len(password) < 12 {
		return errs.InvalidRequest("密码至少需要 12 个字符")
	}
	if len(password) > 256 {
		return errs.InvalidRequest("密码不能超过 256 个字符")
	}
	var upper, lower, digit bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	if !upper || !lower || !digit {
		return errs.InvalidRequest("密码必须同时包含大写字母、小写字母和数字")
	}
	return nil
}

// VerifyPassword 校验明文与哈希。
func VerifyPassword(password, encoded string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errs.Unauthorized("账号或密码错误")
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return errs.Unauthorized("账号或密码错误")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return errs.Unauthorized("账号或密码错误")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return errs.Unauthorized("账号或密码错误")
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return errs.Unauthorized("账号或密码错误")
	}
	return nil
}

// RandomToken 生成 URL 安全的随机 token。
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// TokenDigest 返回只适合数据库查询的令牌摘要，避免明文访客令牌落库。
func TokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
