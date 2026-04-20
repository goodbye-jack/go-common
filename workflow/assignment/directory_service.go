package assignment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/goodbye-jack/go-common/workflow/directory"
	"github.com/goodbye-jack/go-common/workflow/identity"
	"github.com/goodbye-jack/go-common/workflow/types"
)

type DirectoryBackedService struct {
	directory directory.Service
}

func NewDirectoryBackedService(service directory.Service) *DirectoryBackedService {
	return &DirectoryBackedService{directory: service}
}

func NewDirectoryBackedServiceFromConfig() (*DirectoryBackedService, string, error) {
	service, provider, err := directory.NewServiceFromConfig()
	if err != nil {
		return nil, provider, err
	}
	return NewDirectoryBackedService(service), provider, nil
}

func NewLDAPServiceFromConfig() (*DirectoryBackedService, error) {
	service, err := directory.NewLDAPServiceFromConfig()
	if err != nil {
		return nil, err
	}
	return NewDirectoryBackedService(service), nil
}

func (s *DirectoryBackedService) ResolveStart(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error) {
	return s.resolve(ctx, req)
}

func (s *DirectoryBackedService) ResolveComplete(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error) {
	return s.resolve(ctx, req)
}

func (s *DirectoryBackedService) resolve(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error) {
	if s == nil || s.directory == nil {
		return nil, ErrNotConfigured
	}
	if req == nil {
		req = &types.AssignmentResolveRequest{}
	}
	currentUserID := firstNonBlank(req.User.UserID, req.User.UserName)
	if currentUserID == "" {
		return nil, errors.New("workflow assignment user is required")
	}
	profile, err := s.directory.GetUser(ctx, currentUserID)
	if err != nil {
		return nil, err
	}

	starterID := firstNonBlank(
		stringValueFromMap(req.Variables, "starterId"),
		stringValueFromMap(req.Variables, "startUserId"),
		stringValueFromMap(req.CurrentVariables, "starterId"),
		stringValueFromMap(req.CurrentVariables, "startUserId"),
		currentUserID,
	)
	starterName := firstNonBlank(
		stringValueFromMap(req.Variables, "starterName"),
		stringValueFromMap(req.CurrentVariables, "starterName"),
		req.User.UserName,
		currentUserID,
	)

	variables := map[string]interface{}{
		"starterId":   starterID,
		"starterName": starterName,
		"startUserId": starterID,
	}
	putIfNotBlank(variables, "tenantId", req.User.TenantID)
	putIfNotBlank(variables, "systemCode", req.User.SystemCode)

	var managerID string
	if profile != nil {
		putIfNotBlank(variables, "departmentId", valueDepartmentID(profile.Department))
		putIfNotBlank(variables, "departmentName", valueDepartmentName(profile.Department))
		if profile.Manager != nil {
			managerID = strings.TrimSpace(profile.Manager.UserID)
			putIfNotBlank(variables, "managerId", managerID)
			putIfNotBlank(variables, "managerName", strings.TrimSpace(profile.Manager.DisplayName))
		}
	}

	nextAssignee := resolveDirectoryNextAssignee(req, currentUserID, starterID, managerID)
	response := &types.AssignmentResolveResponse{
		Variables: variables,
		Assignee:  nextAssignee,
	}
	if nextAssignee != "" {
		response.CandidateUsers = []string{nextAssignee}
		response.CandidateGroups = s.resolveCandidateGroupsForAssignee(ctx, profile, nextAssignee)
	}
	return response, nil
}

func (s *DirectoryBackedService) resolveCandidateGroupsForAssignee(ctx context.Context, currentProfile *types.DirectoryUserProfile, nextAssignee string) []string {
	normalizer := identity.NewNormalizerFromConfig()
	if currentProfile != nil && strings.TrimSpace(currentProfile.UserID) == strings.TrimSpace(nextAssignee) {
		return normalizer.CandidateGroupIDsForProfile(currentProfile)
	}
	targetProfile, err := s.resolveTargetProfile(ctx, currentProfile, nextAssignee)
	if err != nil || targetProfile == nil {
		return nil
	}
	return normalizer.CandidateGroupIDsForProfile(targetProfile)
}

func (s *DirectoryBackedService) resolveTargetProfile(ctx context.Context, currentProfile *types.DirectoryUserProfile, nextAssignee string) (*types.DirectoryUserProfile, error) {
	targetUserID := strings.TrimSpace(nextAssignee)
	if targetUserID == "" {
		return nil, nil
	}
	if currentProfile != nil && strings.TrimSpace(currentProfile.UserID) == targetUserID {
		return currentProfile, nil
	}
	return s.directory.GetUser(ctx, targetUserID)
}

func resolveDirectoryNextAssignee(req *types.AssignmentResolveRequest, currentUserID, starterID, managerID string) string {
	if shouldRouteToStarter(req) {
		return firstNonBlank(starterID, currentUserID)
	}
	return firstNonBlank(managerID, starterID, currentUserID)
}

func shouldRouteToStarter(req *types.AssignmentResolveRequest) bool {
	if req == nil {
		return false
	}
	return isNegativeResult(req.Result) ||
		isNegativeResult(stringValueFromMap(req.Variables, "result")) ||
		isNegativeResult(stringValueFromMap(req.CurrentVariables, "result"))
}

func isNegativeResult(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "REJECT", "REJECTED", "BACK", "RETURN", "RETURNED", "REWORK", "ROLLBACK":
		return true
	default:
		return false
	}
}

func stringValueFromMap(values map[string]interface{}, key string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(toString(values[key]))
}

func toString(value interface{}) string {
	switch current := value.(type) {
	case nil:
		return ""
	case string:
		return current
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func putIfNotBlank(target map[string]interface{}, key, value string) {
	if target == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}
	target[strings.TrimSpace(key)] = strings.TrimSpace(value)
}

func valueDepartmentID(department *types.DirectoryDepartment) string {
	if department == nil {
		return ""
	}
	return strings.TrimSpace(department.DepartmentID)
}

func valueDepartmentName(department *types.DirectoryDepartment) string {
	if department == nil {
		return ""
	}
	return strings.TrimSpace(department.DepartmentName)
}
