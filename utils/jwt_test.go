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
