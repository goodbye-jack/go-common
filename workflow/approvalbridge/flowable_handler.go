package approvalbridge

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/approval"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/engine/flowable"
	"github.com/goodbye-jack/go-common/workflow/types"
)

const (
	StatusPending  = 1
	StatusApproved = 2
)

type ShouldApproveFunc func(c *gin.Context) bool
type StartRequestBuilder func(c *gin.Context, requestData map[string]interface{}) (*types.StartProcessRequest, error)
type StatusResolver func(view *types.ProcessProgressViewResponse) (int, error)
type ApprovedExecutor func(c *gin.Context, processID string, requestData []byte) error

type FlowableHandler struct {
	client          flowable.Client
	resolver        workflowcontext.Resolver
	shouldApprove   ShouldApproveFunc
	buildRequest    StartRequestBuilder
	resolveStatus   StatusResolver
	executeApproved ApprovedExecutor
}

var _ approval.ApprovalHandler = (*FlowableHandler)(nil)

func NewFlowableHandler(client flowable.Client, resolver workflowcontext.Resolver, builder StartRequestBuilder) (*FlowableHandler, error) {
	if client == nil {
		return nil, errors.New("workflow flowable client is required")
	}
	if builder == nil {
		return nil, errors.New("workflow approval request builder is required")
	}
	return &FlowableHandler{
		client:        client,
		resolver:      resolver,
		buildRequest:  builder,
		resolveStatus: defaultStatusResolver,
	}, nil
}

func (h *FlowableHandler) WithShouldApprove(fn ShouldApproveFunc) *FlowableHandler {
	if h == nil {
		return nil
	}
	h.shouldApprove = fn
	return h
}

func (h *FlowableHandler) WithStatusResolver(fn StatusResolver) *FlowableHandler {
	if h == nil || fn == nil {
		return h
	}
	h.resolveStatus = fn
	return h
}

func (h *FlowableHandler) WithApprovedExecutor(fn ApprovedExecutor) *FlowableHandler {
	if h == nil || fn == nil {
		return h
	}
	h.executeApproved = fn
	return h
}

func (h *FlowableHandler) ShouldApprove(c *gin.Context) bool {
	if h != nil && h.shouldApprove != nil {
		return h.shouldApprove(c)
	}
	return true
}

func (h *FlowableHandler) CreateApprovalProcess(c *gin.Context, requestData map[string]interface{}) (string, error) {
	if h == nil || h.client == nil {
		return "", errors.New("workflow approval handler is not initialized")
	}
	req, err := h.buildRequest(c, requestData)
	if err != nil {
		return "", err
	}
	user, err := h.resolveUser(c)
	if err != nil {
		return "", err
	}
	if req == nil {
		return "", errors.New("workflow approval request is nil")
	}
	if req.Variables == nil {
		req.Variables = map[string]interface{}{}
	}
	req.Variables["approvalRequest"] = requestData
	if user != nil {
		if req.Variables["startUserId"] == nil && strings.TrimSpace(user.UserID) != "" {
			req.Variables["startUserId"] = user.UserID
		}
		if req.Variables["tenantId"] == nil && strings.TrimSpace(user.TenantID) != "" {
			req.Variables["tenantId"] = user.TenantID
		}
		if req.Variables["systemCode"] == nil && strings.TrimSpace(user.SystemCode) != "" {
			req.Variables["systemCode"] = user.SystemCode
		}
	}
	response, err := h.client.StartProcess(c.Request.Context(), req)
	if err != nil {
		return "", err
	}
	return response.ProcessInstanceID, nil
}

func (h *FlowableHandler) GetApprovalProcess(c *gin.Context, processID string) (int, error) {
	if h == nil || h.client == nil {
		return 0, errors.New("workflow approval handler is not initialized")
	}
	user, _ := h.resolveUser(c)
	view, err := h.client.GetProgressView(contextOrBackground(c), strings.TrimSpace(processID), user)
	if err != nil {
		return 0, err
	}
	if h.resolveStatus == nil {
		h.resolveStatus = defaultStatusResolver
	}
	return h.resolveStatus(view)
}

func (h *FlowableHandler) ExecuteApprovedRequest(c *gin.Context, processID string, requestData []byte) error {
	if h != nil && h.executeApproved != nil {
		return h.executeApproved(c, processID, requestData)
	}
	return nil
}

func (h *FlowableHandler) resolveUser(c *gin.Context) (*workflowcontext.UserContext, error) {
	if h == nil || h.resolver == nil || c == nil {
		return nil, nil
	}
	return h.resolver.Resolve(c)
}

func defaultStatusResolver(view *types.ProcessProgressViewResponse) (int, error) {
	if view == nil {
		return StatusPending, nil
	}
	if strings.EqualFold(strings.TrimSpace(view.Summary.Status), "completed") {
		return StatusApproved, nil
	}
	return StatusPending, nil
}

func contextOrBackground(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
