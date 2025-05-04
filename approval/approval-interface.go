package approval

import "github.com/gin-gonic/gin"

// ApprovalHandler 审批处理器接口
type ApprovalHandler interface {
	// ShouldApprove 判断当前请求是否需要审批
	ShouldApprove(c *gin.Context) bool

	// CreateApprovalProcess 创建审批流程
	CreateApprovalProcess(c *gin.Context, requestData []byte) (processID string, err error)

	// GetApprovalProcess 获取审批流程状态
	GetApprovalProcess(c *gin.Context, processID string) (status int, err error)

	// ExecuteApprovedRequest 审批通过后执行原始请求
	ExecuteApprovedRequest(c *gin.Context, processID string, requestData []byte) error
}

// Config 审批配置
type Config struct {
	BusinessApproval bool            // 是否启用业务审批
	Handler          ApprovalHandler // 审批处理器
}

// DefaultHandler 默认审批处理器(空实现)
type DefaultHandler struct{}

func (h *DefaultHandler) ShouldApprove(c *gin.Context) bool {
	return false
}

func (h *DefaultHandler) CreateApprovalProcess(c *gin.Context, requestData []byte) (string, error) {
	return "", nil
}

func (h *DefaultHandler) GetApprovalProcess(c *gin.Context, processID string) (int, error) {
	return 0, nil
}

func (h *DefaultHandler) ExecuteApprovedRequest(c *gin.Context, processID string, requestData []byte) error {
	return nil
}
