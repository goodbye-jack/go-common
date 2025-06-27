package role

import (
	"fmt"
	"github.com/goodbye-jack/go-common/utils"
	"sync"
)

// RoleMappingConfig 定义角色映射配置结构
type RoleMappingConfig struct {
	RoleMapping map[string]string
	//RoleMappingPrecise map[string]string
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
	roleMappingConfig = defaultRoleMappingConfig
	once              sync.Once
)

// InitRoleMapping 初始化角色映射配置
func InitRoleMapping(config RoleMappingConfig) {
	once.Do(func() {
		// 验证配置
		if err := validateRoleConfig(config); err != nil {
			fmt.Printf("角色配置验证失败，使用默认配置: %v\n", err)
			return
		}
		// 设置配置
		roleMappingConfig = config
		fmt.Println("角色配置已成功初始化")
	})
}

// validateRoleConfig 验证角色配置
func validateRoleConfig(config RoleMappingConfig) error {
	// 验证 RoleMapping
	//for role, mappedRoles := range config.RoleMapping {
	//	if role == "" {
	//		return fmt.Errorf("RoleMapping 中发现空角色键")
	//	}
	//	for _, mappedRole := range mappedRoles {
	//		if mappedRole == "" {
	//			return fmt.Errorf("RoleMapping 中角色 %s 的映射值为空", role)
	//		}
	//	}
	//}
	// 验证 RoleMappingPrecise
	for role, preciseRole := range config.RoleMapping {
		if role == "" || preciseRole == "" {
			return fmt.Errorf("RoleMappingPrecise 中发现空键或空值")
		}
	}

	return nil
}

//// GetRoleMapping 获取角色映射
//func GetRoleMapping() map[string][]string {
//	return roleMappingConfig.RoleMapping
//}

// GetRoleMappingPrecise 获取精确角色映射
func GetRoleMapping() map[string]string {
	return roleMappingConfig.RoleMapping
}

// HasRole 检查用户是否拥有特定角色
func HasRole(userRole, requiredRole string) bool {
	// 先检查精确映射
	if preciseRole, exists := GetRoleMapping()[userRole]; exists {
		if preciseRole == requiredRole {
			return true
		}
	}
	//// 再检查普通映射
	//for role, roles := range GetRoleMapping() {
	//	if role == requiredRole {
	//		for _, r := range roles {
	//			if r == userRole {
	//				return true
	//			}
	//		}
	//	}
	//}
	return false
}
