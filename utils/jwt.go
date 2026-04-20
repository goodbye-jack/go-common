package utils

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goodbye-jack/go-common/log"
	"strings"
	"time"
)

type payload struct {
	Data string `json:"data"`
	jwt.RegisteredClaims
}

func GenJWT(data string, expiredSeconds int) (string, error) {
	claims := payload{
		data,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiredSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(JWTSecret))
}

func ParseJWT(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("jwt token is empty")
	}
	log.Debug("ParseJWT invoked")
	t, err := jwt.ParseWithClaims(token, &payload{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(JWTSecret), nil
	})
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", errors.New("jwt token parse result is nil")
	}
	claims, ok := t.Claims.(*payload)
	if !ok {
		return "", errors.New("jwt claims type mismatch")
	}
	if !t.Valid {
		return "", errors.New("jwt token is invalid")
	}
	return claims.Data, nil
}
