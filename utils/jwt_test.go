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
