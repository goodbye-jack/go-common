package utils

import (
	"testing"
	"time"
)

func TestJwt(t *testing.T) {
	token, err := GenJWT("data", 10)
	t.Log(token)
	t.Log(err)

	time.Sleep(time.Duration(5) * time.Second)

	data, err := ParseJWT(token)
	t.Log(data)
	t.Log(err)
}

func TestParseJWTEmptyToken(t *testing.T) {
	data, err := ParseJWT("   ")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if data != "" {
		t.Fatalf("expected empty data, got %q", data)
	}
}

func TestGenJWTWithOptionsAndParseJWTClaims(t *testing.T) {
	token, err := GenJWTWithOptions("customer|general_tenant|1|1", 3600, JWTOptions{
		TokenID:        "token-123",
		Subject:        "customer_1",
		Issuer:         "pano-backend",
		SessionVersion: 7,
	})
	if err != nil {
		t.Fatalf("GenJWTWithOptions() error = %v", err)
	}

	claims, err := ParseJWTClaims(token)
	if err != nil {
		t.Fatalf("ParseJWTClaims() error = %v", err)
	}
	if claims == nil {
		t.Fatal("ParseJWTClaims() returned nil claims")
	}
	if claims.Data != "customer|general_tenant|1|1" {
		t.Fatalf("claims.Data = %s", claims.Data)
	}
	if claims.ID != "token-123" {
		t.Fatalf("claims.ID = %s, want token-123", claims.ID)
	}
	if claims.Subject != "customer_1" {
		t.Fatalf("claims.Subject = %s, want customer_1", claims.Subject)
	}
	if claims.Issuer != "pano-backend" {
		t.Fatalf("claims.Issuer = %s, want pano-backend", claims.Issuer)
	}
	if claims.SessionVersion != 7 {
		t.Fatalf("claims.SessionVersion = %d, want 7", claims.SessionVersion)
	}
}
