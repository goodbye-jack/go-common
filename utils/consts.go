package utils

const (
	RoleIdle          = ""
	RoleManager       = "manager"
	RoleAdministrator = "administrator"

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
