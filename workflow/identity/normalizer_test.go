package identity

import (
	"testing"

	"github.com/goodbye-jack/go-common/workflow/types"
	"github.com/spf13/viper"
)

func TestNormalizerNormalizeRolesAndGroups(t *testing.T) {
	t.Cleanup(func() {
		viper.Set(configRoleAliases, nil)
		viper.Set(configGroupAliases, nil)
		viper.Set(configRolePrefix, "")
		viper.Set(configGroupPrefix, "")
	})

	viper.Set(configRoleAliases, map[string]string{
		"SOURCE_ROLE_CITY_REVIEWER": "APP_ROLE_CITY_REVIEW",
		"SOURCE_ROLE_CITY_LEADER":   "APP_ROLE_CITY_REVIEW",
		"SOURCE_ROLE_COUNTY_OWNER":  "APP_ROLE_COUNTY_REVIEW",
	})
	viper.Set(configGroupAliases, map[string]string{
		"SOURCE_GROUP_CITY_REVIEWERS": "city_reviewers",
	})
	viper.Set(configRolePrefix, "role_")
	viper.Set(configGroupPrefix, "group_")

	normalizer := NewNormalizerFromConfig()
	if got := normalizer.NormalizeRole("source_role_city_reviewer"); got != "APP_ROLE_CITY_REVIEW" {
		t.Fatalf("expected normalized role APP_ROLE_CITY_REVIEW, got %q", got)
	}
	if got := normalizer.NormalizeGroup("SOURCE_GROUP_CITY_REVIEWERS"); got != "city_reviewers" {
		t.Fatalf("expected normalized group city_reviewers, got %q", got)
	}

	candidateGroups := normalizer.CandidateGroupIDs(
		[]string{"SOURCE_GROUP_CITY_REVIEWERS", "SOURCE_GROUP_CITY_REVIEWERS"},
		[]string{"SOURCE_ROLE_CITY_REVIEWER", "SOURCE_ROLE_CITY_LEADER", "APP_ROLE_COUNTY_REVIEW"},
	)
	expected := []string{"group_city_reviewers", "role_APP_ROLE_CITY_REVIEW", "role_APP_ROLE_COUNTY_REVIEW"}
	if len(candidateGroups) != len(expected) {
		t.Fatalf("expected %d candidate groups, got %#v", len(expected), candidateGroups)
	}
	for index, value := range expected {
		if candidateGroups[index] != value {
			t.Fatalf("expected candidateGroups[%d]=%q, got %q", index, value, candidateGroups[index])
		}
	}
}

func TestCandidateGroupIDsForProfile(t *testing.T) {
	t.Cleanup(func() {
		viper.Set(configRoleAliases, nil)
		viper.Set(configRolePrefix, "")
	})

	viper.Set(configRoleAliases, map[string]string{
		"SOURCE_ROLE_COUNTY_OWNER": "APP_ROLE_COUNTY_REVIEW",
	})
	viper.Set(configRolePrefix, "role_")

	normalizer := NewNormalizerFromConfig()
	groups := normalizer.CandidateGroupIDsForProfile(&types.DirectoryUserProfile{
		Position: &types.DirectoryPosition{
			PositionID: "SOURCE_ROLE_COUNTY_OWNER",
		},
	})
	if len(groups) != 1 || groups[0] != "role_APP_ROLE_COUNTY_REVIEW" {
		t.Fatalf("expected role_APP_ROLE_COUNTY_REVIEW, got %#v", groups)
	}
}
