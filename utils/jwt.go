package utils

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goodbye-jack/go-common/log"
	"github.com/google/uuid"
	"strings"
	"time"
)

type JWTClaims struct {
	Data           string `json:"data"`
	SessionVersion int64  `json:"session_version,omitempty"`
	jwt.RegisteredClaims
}

type JWTOptions struct {
	TokenID        string
	Subject        string
	Issuer         string
	Audience       []string
	SessionVersion int64
	IssuedAt       time.Time
	NotBefore      time.Time
	ExpiresAt      time.Time
}

func GenJWT(data string, expiredSeconds int) (string, error) {
	return GenJWTWithOptions(data, expiredSeconds, JWTOptions{})
}

func GenJWTWithOptions(data string, expiredSeconds int, opts JWTOptions) (string, error) {
	now := time.Now()
	issuedAt := opts.IssuedAt
	if issuedAt.IsZero() {
		issuedAt = now
	}
	notBefore := opts.NotBefore
	if notBefore.IsZero() {
		notBefore = issuedAt
	}
	expiresAt := opts.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now.Add(time.Duration(expiredSeconds) * time.Second)
	}
	tokenID := strings.TrimSpace(opts.TokenID)
	if tokenID == "" {
		tokenID = uuid.NewString()
	}

	claims := JWTClaims{
		Data:           data,
		SessionVersion: opts.SessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Subject:   strings.TrimSpace(opts.Subject),
			Issuer:    strings.TrimSpace(opts.Issuer),
			Audience:  jwt.ClaimStrings(opts.Audience),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(notBefore),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(JWTSecret))
}

func ParseJWT(token string) (string, error) {
	claims, err := ParseJWTClaims(token)
	if err != nil {
		return "", err
	}
	return claims.Data, nil
}

func ParseJWTClaims(token string) (*JWTClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("jwt token is empty")
	}
	log.Debug("ParseJWT invoked")
	t, err := jwt.ParseWithClaims(token, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errors.New("jwt token parse result is nil")
	}
	claims, ok := t.Claims.(*JWTClaims)
	if !ok {
		return nil, errors.New("jwt claims type mismatch")
	}
	if !t.Valid {
		return nil, errors.New("jwt token is invalid")
	}
	return claims, nil
}
