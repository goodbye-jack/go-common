package rbac

import (
	"fmt"
	"github.com/goodbye-jack/go-common/utils"
	"sync"
)

// RoleMappingConfig 定义角色映射配置结构
type RoleMappingConfig struct {
	RoleMapping map[string]string
}

// 默认配置
var defaultRoleMappingConfig = RoleMappingConfig{
	RoleMapping: map[string]string{
		utils.RoleDefault:          utils.RoleDefault,
		utils.RoleMuseum:           utils.RoleMuseum,
		utils.RoleMuseumOffice:     utils.RoleMuseum,
		utils.RoleAppraisalStation: utils.RoleAppraisalStation,
		utils.RoleAdministrator:    utils.RoleAdministrator,
		utils.UserAnonymous:        utils.UserAnonymous,
	},
}

// 全局配置实例
var (
	roleConfig     RoleMappingConfig
	roleConfigOnce sync.Once
)

// InitRoleMapping 初始化角色映射配置
// 支持从角色列表初始化，也支持从完整配置初始化
func InitRoleMapping(config interface{}) {
	roleConfigOnce.Do(func() {
		switch cfg := config.(type) {
		case []string:
			// 从角色列表初始化
			roleConfig = initFromRoleList(cfg)
			fmt.Println("从角色列表初始化角色配置成功")
		case RoleMappingConfig:
			// 从完整配置初始化
			roleConfig = cfg
			validateAndMergeConfig(&roleConfig)
			fmt.Println("从完整配置初始化角色配置成功")
		default:
			// 使用默认配置
			roleConfig = defaultRoleMappingConfig
			fmt.Println("使用默认角色配置")
		}
	})
}

// validateAndMergeConfig 验证并合并配置
func validateAndMergeConfig(config *RoleMappingConfig) {
	// 如果没有提供任何映射，使用默认映射
	if config.RoleMapping == nil || len(config.RoleMapping) == 0 {
		config.RoleMapping = defaultRoleMappingConfig.RoleMapping
		return
	}
	// 合并默认映射（保留用户提供的映射，添加默认映射中没有的）
	for role, mappedRole := range defaultRoleMappingConfig.RoleMapping {
		if _, exists := config.RoleMapping[role]; !exists {
			config.RoleMapping[role] = mappedRole
		}
	}
}

// initFromRoleList 从角色列表初始化角色配置
func initFromRoleList(roleList []string) RoleMappingConfig {
	config := defaultRoleMappingConfig
	// 如果角色列表为空，返回默认配置
	if len(roleList) == 0 {
		return config
	}
	// 创建新的映射，保留默认映射并添加/覆盖角色列表中的角色
	preciseMapping := make(map[string]string)
	// 复制默认映射
	for role, mappedRole := range config.RoleMapping {
		preciseMapping[role] = mappedRole
	}
	// 添加/覆盖角色列表中的角色
	for _, role := range roleList {
		preciseMapping[role] = role
	}
	config.RoleMapping = preciseMapping
	return config
}

// GetPreciseRoleMapping 获取精确角色映射
func GetRoleMapping(role string) (string, bool) {
	if roleConfig.RoleMapping == nil {
		return "", false
	}
	mappedRole, exists := roleConfig.RoleMapping[role]
	return mappedRole, exists
}

// HasRole 检查用户是否拥有特定角色
func HasRole(userRole, requiredRole string) bool {
	// 检查精确映射
	if preciseRole, exists := GetRoleMapping(userRole); exists {
		if preciseRole == requiredRole {
			return true
		}
	}
	// 如果用户角色与所需角色直接匹配，也返回 true
	return userRole == requiredRole
}
