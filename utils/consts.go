package utils

const (
	RoleGuest         = "guest"
	RoleEditor        = "editor"
	RoleManager       = "manager"
	RoleAdministrator = "administrator"

	//not login
	UserAnonymous     = "anonymous"
	UserAdministrator = "administrator"

	TenantAnonymous = ""

	LLDapLoginURL        = "/auth/simple/login"
	LLDapRefreshTokenURL = "/auth/refresh"
	LLDapGraphURL        = "/api/graphql"
)
