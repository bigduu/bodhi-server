package auth

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

type Claims struct {
	jwt.RegisteredClaims
	Username string    `json:"username"`
	Type     TokenType `json:"type"`
}

func GenerateAccessToken(userID, username, secret string, ttl time.Duration) (string, error) {
	return generateToken(userID, username, secret, AccessToken, ttl)
}

func GenerateRefreshToken(userID, username, secret string, ttl time.Duration) (string, error) {
	return generateToken(userID, username, secret, RefreshToken, ttl)
}

func generateToken(userID, username, secret string, tokenType TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        generateJTI(),
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "bodhi-server",
		},
		Username: username,
		Type:     tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func generateJTI() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func ValidateToken(tokenStr, secret string, expectedType TokenType) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	if claims.Type != expectedType {
		return nil, fmt.Errorf("invalid token type: expected %s, got %s", expectedType, claims.Type)
	}

	return claims, nil
}
