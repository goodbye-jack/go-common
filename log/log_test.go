package log

import "testing"

func TestNormalizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args []interface{}
		want string
	}{
		{
			name: "empty",
			args: nil,
			want: "",
		},
		{
			name: "plain string",
			args: []interface{}{"hello"},
			want: "hello",
		},
		{
			name: "format string",
			args: []interface{}{"user=%s role=%s", "u1", "ADMIN"},
			want: "user=u1 role=ADMIN",
		},
		{
			name: "plain variadic concat",
			args: []interface{}{"token name = ", "good_token"},
			want: "token name = good_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeArgs(tt.args...)
			if got != tt.want {
				t.Fatalf("normalizeArgs mismatch: want=%q got=%q", tt.want, got)
			}
		})
	}
}
