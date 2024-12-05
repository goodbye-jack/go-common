package http

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
)

type Route struct {
	ServiceName  string
	Url          string
	Methods      []string
	Sso          bool
	DefaultRoles []string
	handlerFunc  gin.HandlerFunc
}

var RoleMapping = map[string][]string{}

func init() {
	RoleMapping[utils.RoleIdle] = []string{
		utils.UserAnonymous,
		utils.RoleAdministrator,
	}
	RoleMapping[utils.RoleAdministrator] = []string{
		utils.RoleAdministrator,
	}
}

func NewRoute(service_name string, url string, methods []string, role string, sso bool, handlerFunc gin.HandlerFunc) *Route {
	if len(methods) == 0 {
		log.Fatal("NewRoute methods is empty")
	}

	if _, ok := RoleMapping[role]; !ok {
		log.Fatalf("the role %v is invalid", role)
	}

	return &Route{
		ServiceName:  service_name,
		Sso:          sso,
		Url:          url,
		Methods:      methods,
		DefaultRoles: RoleMapping[role],
		handlerFunc:  handlerFunc,
	}
}

func (r *Route) ToSso() ([]string, []string) {
	sso := []string{}
	nonsso := []string{}
	for _, method := range r.Methods {
		uniq := fmt.Sprintf("%s_%s", r.Url, method)
		if r.Sso {
			sso = append(sso, uniq)
		} else {
			nonsso = append(nonsso, uniq)
		}
	}
	return sso, nonsso
}

func (r *Route) ToRbacPolicy() []rbac.Policy {
	ans := []rbac.Policy{}
	for _, method := range r.Methods {
		if len(r.DefaultRoles) == 0 {
			ans = append(
				ans,
				rbac.NewActionPolicy(r.ServiceName, utils.UserAnonymous, r.Url, method),
			)
		}
		for _, role := range r.DefaultRoles {
			ans = append(
				ans,
				rbac.NewActionPolicy(r.ServiceName, role, r.Url, method),
			)
		}
	}

	return ans
}

func (r *Route) ToGinRoute() []*gin.RouteInfo {
	ans := []*gin.RouteInfo{}
	for _, method := range r.Methods {
		ans = append(
			ans,
			&gin.RouteInfo{
				Method:      method,
				Path:        r.Url,
				HandlerFunc: r.handlerFunc,
			},
		)
	}
	return ans
}
