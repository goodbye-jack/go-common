package context

import "github.com/gin-gonic/gin"

const userContextKey = "WorkflowUserContext"

type UserContext struct {
	UserID     string
	UserName   string
	TenantID   string
	SystemCode string
	Groups     []string
	Roles      []string
}

type Resolver interface {
	Resolve(c *gin.Context) (*UserContext, error)
}

func SetUserContext(c *gin.Context, user *UserContext) {
	if c == nil || user == nil {
		return
	}
	c.Set(userContextKey, user)
}

func GetUserContext(c *gin.Context) (*UserContext, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(userContextKey)
	if !ok {
		return nil, false
	}
	user, ok := value.(*UserContext)
	return user, ok
}
