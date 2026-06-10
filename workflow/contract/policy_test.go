package contract

import "testing"

func TestDefaultPolicyUsesCompatibleModeAndStandardVars(t *testing.T) {
	policy := DefaultPolicy()
	if policy == nil {
		t.Fatalf("DefaultPolicy() returned nil")
	}
	if policy.EffectiveMode() != ModeCompatible {
		t.Fatalf("EffectiveMode()=%s, want %s", policy.EffectiveMode(), ModeCompatible)
	}
	if !policy.EnforceStandardAssignmentKeys {
		t.Fatalf("EnforceStandardAssignmentKeys=false, want true")
	}
	if !AreStandardAssignmentKeys(StandardAssigneeKey, StandardCandidateUsersKey, StandardCandidateGroupsKey) {
		t.Fatalf("AreStandardAssignmentKeys()=false, want true")
	}
}

func TestStrictPolicyFailsOnNonstandardAssignmentKeys(t *testing.T) {
	policy := &Policy{Mode: ModeStrict}
	if !policy.ShouldFailOnNonstandardAssignmentKeys() {
		t.Fatalf("ShouldFailOnNonstandardAssignmentKeys()=false, want true")
	}
	if !policy.ShouldWarnOnNonstandardAssignmentKeys() {
		t.Fatalf("ShouldWarnOnNonstandardAssignmentKeys()=false, want true")
	}
}
