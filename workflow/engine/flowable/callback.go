package flowable

import (
	stdcontext "context"
	"errors"
	"strings"

	"github.com/goodbye-jack/go-common/workflow/types"
)

func (c *RESTClient) ProcessCallback(ctx stdcontext.Context, payload *types.FlowableCallbackPayload) error {
	if c == nil {
		return errors.New("flowable client is required")
	}
	if payload == nil {
		return errors.New("callback payload is required")
	}
	eventType := strings.ToUpper(strings.TrimSpace(payload.EventType))
	if eventType == "" {
		return errors.New("callback event type is required")
	}
	switch eventType {
	case "PROCESS_STARTED":
		c.refreshProcessSummaryProjection(ctx, payload)
	case "NODE_STARTED":
		c.syncRuntimeTaskProjection(ctx, payload.TaskID, payload.ProcessInstanceID)
		c.refreshProcessSummaryProjection(ctx, payload)
	case "NODE_ENDED":
		c.syncDoneProjectionByTaskID(ctx, payload.TaskID)
		c.removeRuntimeTaskProjection(ctx, payload.TaskID)
		c.refreshProcessSummaryProjection(ctx, payload)
	case "PROCESS_ENDED":
		c.clearProcessRuntimeTaskProjections(ctx, payload.ProcessInstanceID)
		c.refreshProcessSummaryProjection(ctx, payload)
	default:
		c.refreshProcessSummaryProjection(ctx, payload)
	}
	return nil
}
