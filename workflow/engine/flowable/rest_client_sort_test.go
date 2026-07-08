package flowable

import (
	"testing"

	"github.com/goodbye-jack/go-common/workflow/types"
)

func TestSortTaskInfosTodoUsesDescendingCreatedAt(t *testing.T) {
	items := []types.TaskInfo{
		{TaskID: "task-1", CreatedAt: "2026-06-14T10:00:00Z"},
		{TaskID: "task-2", CreatedAt: "2026-06-14T12:00:00Z"},
		{TaskID: "task-3", CreatedAt: "2026-06-14T11:00:00Z"},
	}

	sortTaskInfos(items, false)

	got := []string{items[0].TaskID, items[1].TaskID, items[2].TaskID}
	want := []string{"task-2", "task-3", "task-1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("todo order=%v, want=%v", got, want)
		}
	}
}

func TestSortTaskInfosDoneUsesDescendingCompletedAt(t *testing.T) {
	items := []types.TaskInfo{
		{TaskID: "task-1", CompletedAt: "2026-06-14T10:00:00Z"},
		{TaskID: "task-2", CompletedAt: "2026-06-14T12:00:00Z"},
		{TaskID: "task-3", CompletedAt: "2026-06-14T11:00:00Z"},
	}

	sortTaskInfos(items, true)

	got := []string{items[0].TaskID, items[1].TaskID, items[2].TaskID}
	want := []string{"task-2", "task-3", "task-1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("done order=%v, want=%v", got, want)
		}
	}
}
