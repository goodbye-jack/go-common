package assignment

import (
	"context"
	"testing"

	"github.com/goodbye-jack/go-common/workflow/types"
)

type stubDirectoryService struct {
	profile  *types.DirectoryUserProfile
	profiles map[string]*types.DirectoryUserProfile
	err      error
}

func (s *stubDirectoryService) ValidateUser(ctx context.Context, userID, password string) (*types.DirectoryUserProfile, error) {
	return s.profile, s.err
}

func (s *stubDirectoryService) GetCurrentUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error) {
	return s.profile, s.err
}

func (s *stubDirectoryService) GetUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error) {
	if s.profiles != nil {
		if profile, ok := s.profiles[userID]; ok {
			return profile, s.err
		}
	}
	return s.profile, s.err
}

func (s *stubDirectoryService) GetManager(ctx context.Context, userID string) (*types.DirectoryUserSummary, error) {
	if s.profile == nil {
		return nil, s.err
	}
	return s.profile.Manager, s.err
}

func (s *stubDirectoryService) GetDepartment(ctx context.Context, userID string) (*types.DirectoryDepartment, error) {
	if s.profile == nil {
		return nil, s.err
	}
	return s.profile.Department, s.err
}

func TestDirectoryBackedServiceResolveStart(t *testing.T) {
	service := NewDirectoryBackedService(&stubDirectoryService{
		profiles: map[string]*types.DirectoryUserProfile{
			"alice": {
				UserID:      "alice",
				DisplayName: "Alice",
				Department: &types.DirectoryDepartment{
					DepartmentID:   "dept-1",
					DepartmentName: "文物处",
				},
				Manager: &types.DirectoryUserSummary{
					UserID:      "leader01",
					DisplayName: "Leader",
				},
			},
			"leader01": {
				UserID:      "leader01",
				DisplayName: "Leader",
				Position: &types.DirectoryPosition{
					PositionID: "CITY_ADMIN",
				},
			},
		},
	})

	response, err := service.ResolveStart(context.Background(), &types.AssignmentResolveRequest{
		User: types.AssignmentUserContext{
			UserID:     "alice",
			UserName:   "Alice",
			TenantID:   "tenant-a",
			SystemCode: "relics",
		},
	})
	if err != nil {
		t.Fatalf("ResolveStart returned error: %v", err)
	}
	if response == nil {
		t.Fatal("ResolveStart returned nil response")
	}
	if response.Assignee != "leader01" {
		t.Fatalf("expected assignee leader01, got %q", response.Assignee)
	}
	if len(response.CandidateUsers) != 1 || response.CandidateUsers[0] != "leader01" {
		t.Fatalf("expected candidate user leader01, got %#v", response.CandidateUsers)
	}
	if got := response.Variables["managerId"]; got != "leader01" {
		t.Fatalf("expected managerId leader01, got %#v", got)
	}
	if got := response.Variables["departmentId"]; got != "dept-1" {
		t.Fatalf("expected departmentId dept-1, got %#v", got)
	}
	if len(response.CandidateGroups) != 1 || response.CandidateGroups[0] != "CITY_ADMIN" {
		t.Fatalf("expected candidate group CITY_ADMIN, got %#v", response.CandidateGroups)
	}
}

func TestDirectoryBackedServiceResolveCompleteRejectsToStarter(t *testing.T) {
	service := NewDirectoryBackedService(&stubDirectoryService{
		profile: &types.DirectoryUserProfile{
			UserID: "alice",
			Manager: &types.DirectoryUserSummary{
				UserID: "leader01",
			},
		},
	})

	response, err := service.ResolveComplete(context.Background(), &types.AssignmentResolveRequest{
		Result: "REJECTED",
		User: types.AssignmentUserContext{
			UserID: "alice",
		},
		CurrentVariables: map[string]interface{}{
			"starterId": "starter99",
		},
	})
	if err != nil {
		t.Fatalf("ResolveComplete returned error: %v", err)
	}
	if response == nil {
		t.Fatal("ResolveComplete returned nil response")
	}
	if response.Assignee != "starter99" {
		t.Fatalf("expected assignee starter99, got %q", response.Assignee)
	}
	if len(response.CandidateUsers) != 1 || response.CandidateUsers[0] != "starter99" {
		t.Fatalf("expected candidate user starter99, got %#v", response.CandidateUsers)
	}
}
