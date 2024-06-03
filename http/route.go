package http

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
)

type Route struct {
	service_name string
	url         string
	methods     []string
	sso         bool
	roles       []string
	handlerFunc gin.HandlerFunc
}

var RoleMapping = map[string][]string{}

func init() {
	RoleMapping[""] = []string{
		utils.RoleAdministrator,
		utils.RoleManager,
		utils.RoleEditor,
		utils.RoleGuest,
		utils.UserAnonymous,
	}
	RoleMapping[utils.RoleGuest] = []string{
		utils.RoleAdministrator,
		utils.RoleManager,
		utils.RoleEditor,
		utils.RoleGuest,
	}
	RoleMapping[utils.RoleEditor] = []string{
		utils.RoleAdministrator,
		utils.RoleManager,
		utils.RoleEditor,
	}
	RoleMapping[utils.RoleManager] = []string{
		utils.RoleAdministrator,
		utils.RoleManager,
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
		service_name: service_name,
		sso:        sso,
		url:        url,
		methods:    methods,
		roles:      RoleMapping[role],
		handlerFunc: handlerFunc,
	}
}

func (r *Route) ToSso() ([]string, []string) {
	sso := []string{}
	nonsso := []string{}
	for _, method := range r.methods {
		uniq := fmt.Sprintf("%s_%s", r.url, method)
		if r.sso {
			sso = append(sso, uniq)
		} else {
			nonsso = append(nonsso, uniq)
		}
	}
	return sso, nonsso
}

func (r *Route) ToRbacPolicy() []rbac.Policy {
	ans := []rbac.Policy{}
	for _, method := range r.methods {
		if len(r.roles) == 0 {
			ans = append(
				ans,
				rbac.NewActionPolicy(r.service_name, utils.UserAnonymous, r.url, method),
			)
		}
		for _, role := range r.roles {
			ans = append(
				ans,
				rbac.NewActionPolicy(r.service_name, role, r.url, method),
			)
		}
	}

	return ans
}

func (r *Route) ToGinRoute() []*gin.RouteInfo {
	ans := []*gin.RouteInfo{}
	for _, method := range r.methods {
		ans = append(
			ans, 
			&gin.RouteInfo{
				Method:      method,
				Path:        r.url,
				HandlerFunc: r.handlerFunc,
			},
		)
	}
	return ans
}
