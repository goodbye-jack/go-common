package http

import "testing"

func TestNewHTTPServerDoesNotInitializeRbacClient(t *testing.T) {
	original := RbacClient
	RbacClient = nil
	t.Cleanup(func() {
		RbacClient = original
	})

	server := NewHTTPServer("workflow-test")
	if server == nil {
		t.Fatalf("NewHTTPServer() returned nil")
	}
	if RbacClient != nil {
		t.Fatalf("RbacClient initialized eagerly, want nil before Prepare or middleware use")
	}
}
