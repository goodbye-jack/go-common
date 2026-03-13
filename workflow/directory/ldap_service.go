package directory

import (
	"context"
	"strings"

	commonldap "github.com/goodbye-jack/go-common/ldap"
	"github.com/goodbye-jack/go-common/workflow/types"
)

type LDAPService struct {
	client commonldap.Ldap
}

func NewLDAPService(client commonldap.Ldap) *LDAPService {
	return &LDAPService{client: client}
}

func NewLDAPServiceFromConfig() (*LDAPService, error) {
	client, err := commonldap.New()
	if err != nil {
		return nil, err
	}
	return NewLDAPService(client), nil
}

func (s *LDAPService) ValidateUser(ctx context.Context, userID, password string) (*types.DirectoryUserProfile, error) {
	user, err := s.client.ValidateUserByUID(ctx, userID, password)
	if err != nil {
		return nil, err
	}
	return s.toProfile(ctx, user)
}

func (s *LDAPService) GetCurrentUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error) {
	return s.GetUser(ctx, userID)
}

func (s *LDAPService) GetUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error) {
	user, err := s.client.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.toProfile(ctx, user)
}

func (s *LDAPService) GetManager(ctx context.Context, userID string) (*types.DirectoryUserSummary, error) {
	profile, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return profile.Manager, nil
}

func (s *LDAPService) GetDepartment(ctx context.Context, userID string) (*types.DirectoryDepartment, error) {
	profile, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return profile.Department, nil
}

func (s *LDAPService) toProfile(ctx context.Context, user *commonldap.OrgUser) (*types.DirectoryUserProfile, error) {
	if user == nil {
		return nil, nil
	}

	profile := &types.DirectoryUserProfile{
		UserID:      user.UID,
		Username:    user.UID,
		DisplayName: user.DisplayName,
		GivenName:   user.FirstName,
		FamilyName:  user.LastName,
		Email:       user.Email,
		Mobile:      user.Phone,
		DN:          user.DN,
		Title:       firstNonBlank(firstItem(user.PositionCodes), user.DisplayName),
	}

	if firstDept := firstItem(user.DeptCodes); firstDept != "" {
		department, err := s.client.GetDepartment(ctx, firstDept)
		if err == nil && department != nil {
			profile.Department = &types.DirectoryDepartment{
				DepartmentID:         department.Code,
				DepartmentName:       firstNonBlank(department.Name, department.Code),
				DN:                   department.DN,
				ParentDepartmentID:   extractRDNValue(department.ParentDN, "ou"),
				ParentDepartmentName: extractRDNValue(department.ParentDN, "ou"),
			}
			if department.ManagerDN != "" {
				manager, err := s.client.GetUserByDN(ctx, department.ManagerDN)
				if err == nil && manager != nil {
					profile.Manager = &types.DirectoryUserSummary{
						UserID:      manager.UID,
						DisplayName: firstNonBlank(manager.DisplayName, manager.UID),
						Email:       manager.Email,
						DN:          manager.DN,
					}
				} else {
					profile.Manager = fallbackManager(department.ManagerDN)
				}
			}
		}
	}

	if firstPosition := firstItem(user.PositionCodes); firstPosition != "" {
		position, err := s.client.GetPosition(ctx, firstPosition)
		if err == nil && position != nil {
			profile.Position = &types.DirectoryPosition{
				PositionID:   position.Code,
				PositionName: firstNonBlank(position.Name, position.Code),
				DN:           position.DN,
			}
			profile.Title = firstNonBlank(position.Name, position.Code, profile.Title)
		}
	}

	return profile, nil
}

func fallbackManager(managerDN string) *types.DirectoryUserSummary {
	uid := extractRDNValue(managerDN, "uid")
	if uid == "" {
		return nil
	}
	return &types.DirectoryUserSummary{
		UserID:      uid,
		DisplayName: uid,
		DN:          managerDN,
	}
}

func extractRDNValue(dn, key string) string {
	if dn == "" || key == "" {
		return ""
	}
	prefix := strings.ToLower(key) + "="
	parts := strings.Split(dn, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			return trimmed[len(prefix):]
		}
	}
	return ""
}

func firstItem(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0])
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
