package http

import "github.com/gin-gonic/gin"

type TokenValidator interface {
	Name() string
	Validate(*gin.Context, *Credential, *Principal) error
}
