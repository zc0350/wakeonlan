// Package auth 负责 JWT 令牌的签发与解析。
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 JWT 中携带的自定义声明。
type Claims struct {
	AdminID  int64  `json:"admin_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// TokenTTL 是登录令牌的有效期。
const TokenTTL = 24 * time.Hour

// GenerateToken 使用 HS256 算法签发一个管理员令牌。
func GenerateToken(secret string, adminID int64, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		AdminID:  adminID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
			Issuer:    "wol-server",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken 校验并解析令牌，返回其中的声明。
func ParseToken(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
