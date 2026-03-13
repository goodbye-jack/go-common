package formref

import (
	"context"

	"github.com/goodbye-jack/go-common/workflow/types"
)

type TaskFormLocator struct {
	ProcessDefinitionID string
	ProcessInstanceID   string
	TaskID              string
	TaskDefinitionKey   string
	FormKey             string
	TenantID            string
}

type Service interface {
	Resolve(ctx context.Context, locator *TaskFormLocator) (*types.TaskFormReference, error)
}
