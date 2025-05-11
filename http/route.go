package http

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
)

type Route struct {
	ServiceName      string            // 服务名称
	Tips             string            // 路由说明
	Sso              bool              // 是否需要SSO登录
	Url              string            // 路由路径
	Methods          []string          // HTTP方法(GET,POST等)
	DefaultRoles     []string          // 所需角色映射
	handlerFunc      gin.HandlerFunc   // 主处理函数
	BusinessApproval bool              // 是否需要业务审批
	middlewares      []gin.HandlerFunc // 中间件链(新增)
}

var RoleMapping = map[string][]string{}
var RoleMappingPrecise = map[string]string{}

func init() {
	RoleMapping[utils.RoleIdle] = []string{
		utils.UserAnonymous,
		utils.RoleAdministrator,
		utils.RoleDefault,
		utils.RoleMuseum,
		utils.RoleMuseumOffice,
		utils.RoleAppraisalStation,
	}
	RoleMapping[utils.RoleAdministrator] = []string{
		utils.RoleAdministrator,
	}
	RoleMappingPrecise[utils.RoleDefault] = utils.RoleDefault
	RoleMappingPrecise[utils.RoleMuseum] = utils.RoleMuseum
	RoleMappingPrecise[utils.RoleMuseumOffice] = utils.RoleMuseum
	RoleMappingPrecise[utils.RoleAppraisalStation] = utils.RoleAppraisalStation
	RoleMappingPrecise[utils.RoleAdministrator] = utils.RoleAdministrator
	RoleMappingPrecise[utils.UserAnonymous] = utils.UserAnonymous
}

func NewRoute(service_name string, url string, tips string, methods []string, role string, sso bool, business_approval bool, handlerFunc gin.HandlerFunc) *Route {
	if len(methods) == 0 {
		log.Fatal("NewRoute methods is empty")
	}
	if _, ok := RoleMapping[role]; !ok {
		log.Fatalf("the role %v is invalid", role)
	}
	return &Route{
		ServiceName:      service_name,
		Tips:             tips,
		Sso:              sso,
		Url:              url,
		Methods:          methods,
		DefaultRoles:     RoleMapping[role],
		handlerFunc:      handlerFunc,
		BusinessApproval: business_approval,
		middlewares:      []gin.HandlerFunc{}, // 初始化空中间件链
	}
}

func NewRouteForRA(serviceName string, url string, tips string, methods []string, roles []string, sso bool, businessApproval bool, handlerFunc gin.HandlerFunc) *Route {
	if len(methods) == 0 {
		log.Fatal("NewRoute methods is empty")
	}
	newRoles := []string{}
	for _, role := range roles {
		if _, ok := RoleMappingPrecise[role]; ok { // 代表权限在初始化的权限角色中,
			// 可以进行访问,这块应该是脱离common 的 但是还是等后面重新设计吧 OK
			newRoles = append(newRoles, role)
		}
	}
	return &Route{
		ServiceName:      serviceName,
		Tips:             tips,
		Sso:              sso,
		Url:              url,
		Methods:          methods,
		DefaultRoles:     newRoles,
		handlerFunc:      handlerFunc,
		BusinessApproval: businessApproval,
		middlewares:      []gin.HandlerFunc{}, // 初始化空中间件链
	}
}

// AddMiddleware 添加中间件到路由
func (r *Route) AddMiddleware(middleware gin.HandlerFunc) {
	r.middlewares = append(r.middlewares, middleware)
}

// GetHandlersChain 获取完整处理链(中间件+主处理函数)
func (r *Route) GetHandlersChain() []gin.HandlerFunc {
	handlers := make([]gin.HandlerFunc, 0, len(r.middlewares)+1)
	// 先添加所有中间件
	handlers = append(handlers, r.middlewares...)
	// 最后添加主处理函数
	handlers = append(handlers, r.handlerFunc)
	return handlers
}

func (r *Route) ToSso() ([]string, []string) {
	var sso []string
	var nonsso []string
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
	var ans []rbac.Policy
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

// ToGinRoute 转换为Gin路由信息(修改后版本)
//func (r *Route) ToGinRoute() []*gin.RouteInfo {
//	var routeInfos []*gin.RouteInfo
//	for _, method := range r.Methods {
//		routeInfos = append(routeInfos, &gin.RouteInfo{
//			Method:      method,
//			Path:        r.Url,
//			HandlerFunc: r.GetHandlersChain()..., // 展开处理链
//		})
//	}
//	return routeInfos
//}

//func (r *Route) ToGinRoute() []*gin.RouteInfo {
//	var ans []*gin.RouteInfo
//	for _, method := range r.Methods {
//		ans = append(
//			ans,
//			&gin.RouteInfo{
//				Method:      method,
//				Path:        r.Url,
//				HandlerFunc: r.handlerFunc,
//			},
//		)
//	}
//	return ans
//}
