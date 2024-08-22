package utils

import (
	"time"
	"github.com/golang-jwt/jwt/v5"
)

type payload struct {
	Data string `json:"data"`
	jwt.RegisteredClaims
}

func GenJWT(data string, expiredSeconds int) (string, error) {
	claims := payload {
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
	t, err := jwt.ParseWithClaims(token, &payload{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(JWTSecret), nil
	})

	if claims, ok := t.Claims.(*payload); ok && t.Valid {
		return claims.Data, nil
	} else {
		return "", err
	}
} 
