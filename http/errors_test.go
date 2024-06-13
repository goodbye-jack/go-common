package http

import (
	"github.com/goodbye-jack/go-common/log"
	"testing"
)

func TestServerError(t *testing.T) {
	err := ServerError("error occurring")
	log.Info(err)
	msg := whichError(err)
	log.Info(msg)

	ServerErrorf("error occurring, %d", 2)


}
