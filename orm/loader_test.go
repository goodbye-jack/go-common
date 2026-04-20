package orm

import "testing"

func TestGetInstanceNamesDefaultFirst(t *testing.T) {
	instances := map[string]interface{}{
		"flowable_meta": struct{}{},
		"default":       struct{}{},
		"archive":       struct{}{},
	}

	got := getInstanceNames(instances)
	want := []string{"default", "archive", "flowable_meta"}

	if len(got) != len(want) {
		t.Fatalf("len(getInstanceNames())=%d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("getInstanceNames()[%d]=%s, want %s", index, got[index], want[index])
		}
	}
}
