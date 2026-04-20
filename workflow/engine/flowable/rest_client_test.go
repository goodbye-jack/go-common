package flowable

import (
	"testing"

	"github.com/goodbye-jack/go-common/workflow/types"
)

func TestMergeOptionalNeedExpert(t *testing.T) {
	tests := []struct {
		name     string
		req      *types.CompleteTaskRequest
		target   map[string]interface{}
		expected interface{}
		exists   bool
	}{
		{
			name:     "skip when request omitted",
			req:      &types.CompleteTaskRequest{},
			target:   map[string]interface{}{},
			expected: nil,
			exists:   false,
		},
		{
			name: "keep explicit false from variables",
			req: &types.CompleteTaskRequest{
				Variables: map[string]interface{}{
					"needExpert": false,
				},
			},
			target: map[string]interface{}{
				"needExpert": false,
			},
			expected: false,
			exists:   true,
		},
		{
			name: "variables win over top level true",
			req: &types.CompleteTaskRequest{
				NeedExpert: true,
				Variables: map[string]interface{}{
					"needExpert": false,
				},
			},
			target: map[string]interface{}{
				"needExpert": false,
			},
			expected: false,
			exists:   true,
		},
		{
			name: "top level true fills missing variable",
			req: &types.CompleteTaskRequest{
				NeedExpert: true,
			},
			target:   map[string]interface{}{},
			expected: true,
			exists:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeOptionalNeedExpert(tt.target, tt.req)
			value, ok := tt.target["needExpert"]
			if ok != tt.exists {
				t.Fatalf("needExpert exists=%v, want %v", ok, tt.exists)
			}
			if !tt.exists {
				return
			}
			if value != tt.expected {
				t.Fatalf("needExpert=%v, want %v", value, tt.expected)
			}
		})
	}
}

func TestShouldRetryFlowableDeadlock(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "deadlock message",
			err:  assertError("POST /runtime/tasks/1 failed: status=500 body={\"exception\":\"Deadlock found when trying to get lock; try restarting transaction\"}"),
			want: true,
		},
		{
			name: "other error",
			err:  assertError("POST /runtime/tasks/1 failed: status=500 body={\"exception\":\"Unknown property used in expression\"}"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryFlowableDeadlock(tt.err); got != tt.want {
				t.Fatalf("shouldRetryFlowableDeadlock()=%v, want %v", got, tt.want)
			}
		})
	}
}

type assertError string

func (e assertError) Error() string {
	return string(e)
}
