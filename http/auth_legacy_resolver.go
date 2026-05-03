package http

import (
	"strings"

	"github.com/gin-gonic/gin"
	goodutils "github.com/goodbye-jack/go-common/utils"
)

type legacyJWTResolver struct{}

func (legacyJWTResolver) Name() string { return "legacy-jwt" }

func (legacyJWTResolver) Supports(cred *Credential) bool {
	if cred == nil || strings.TrimSpace(cred.Token) == "" {
		return false
	}
	_, err := goodutils.ParseJWTClaims(cred.Token)
	return err == nil
}

func (legacyJWTResolver) Resolve(_ *gin.Context, cred *Credential) (*Principal, error) {
	claims, err := goodutils.ParseJWTClaims(cred.Token)
	if err != nil {
		return nil, err
	}
	payload := strings.TrimSpace(claims.Data)
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, nil
	}
	return &Principal{
		Type:        PrincipalService,
		Subject:     payload,
		TokenSource: cred.Source,
		TokenID:     claims.ID,
		Issuer:      claims.Issuer,
		DisplayName: payload,
		RawClaims: map[string]any{
			"legacy_payload":  payload,
			"session_version": claims.SessionVersion,
		},
	}, nil
}
