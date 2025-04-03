package utils

const (
	RoleIdle    = ""
	RoleManager = "manager"
	//RoleAdministrator = "administrator"
	RoleAdministrator    = "ADMINISTRATOR_ROLE"     //管理员角色
	RoleDefault          = "DEFAULT_ROLE"           //默认角色
	RoleAppraisalStation = "APPRAISAL_STATION_ROLE" // 鉴定站角色
	RoleMuseum           = "MUSEUM_ROLE"            // 博物馆角色
	RoleMuseumOffice     = "MUSEUM_OFFICE_ROLE"     // 博物馆处角色

	//not login
	UserAnonymous     = "anonymous"
	UserAdministrator = "administrator"

	TenantAnonymous = ""

	LLDapLoginURL        = "/auth/simple/login"
	LLDapRefreshTokenURL = "/auth/refresh"
	LLDapGraphURL        = "/api/graphql"

	TenantContextName = "ContextTenant"
	TenantHeaderName  = "X-Tenant"

	CasbinRedisAddrName = "redis_addr"

	JWTSecret = "goodbye-jack,comeon"

	ConfigNameToken = "cookie_token"
	// 统一登录校验是否开启
	SsoEnabledVerify       = "sso_enable_verify"
	SsoVerifyHandlerName   = "sso_verify_handler_name"
	ConfigNameTokenExpired = "cookie_token_expired_seconds"
)
