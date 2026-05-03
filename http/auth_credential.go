package http

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
)

type Credential struct {
	Token  string
	Source string
	Scheme string
	Raw    string
}

const (
	TokenSourceBearer = "bearer"
	TokenSourceCookie = "cookie"
)

type CredentialExtractor interface {
	Name() string
	Extract(c *gin.Context) (*Credential, error)
}

type PrincipalResolver interface {
	Name() string
	Supports(*Credential) bool
	Resolve(c *gin.Context, cred *Credential) (*Principal, error)
}

type bearerCredentialExtractor struct{}

func (bearerCredentialExtractor) Name() string { return "bearer" }

func (bearerCredentialExtractor) Extract(c *gin.Context) (*Credential, error) {
	if c == nil {
		return nil, errors.New("request context is nil")
	}
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return nil, nil
	}
	token := strings.TrimSpace(authHeader[7:])
	if token == "" {
		return nil, errors.New("empty bearer token")
	}
	return &Credential{
		Token:  token,
		Source: TokenSourceBearer,
		Scheme: "Bearer",
		Raw:    authHeader,
	}, nil
}

type cookieCredentialExtractor struct{}

func (cookieCredentialExtractor) Name() string { return "cookie" }

func (cookieCredentialExtractor) Extract(c *gin.Context) (*Credential, error) {
	if c == nil {
		return nil, errors.New("request context is nil")
	}
	token, err := c.Cookie(ResolveCookieTokenName())
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, nil
	}
	token = strings.TrimSpace(token)
	return &Credential{
		Token:  token,
		Source: TokenSourceCookie,
		Scheme: "Cookie",
		Raw:    ResolveCookieTokenName(),
	}, nil
}
