package approval

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func ApprovalMiddleware(config Config) gin.HandlerFunc {
	if !config.BusinessApproval || config.Handler == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		// 检查是否需要审批
		if !config.Handler.ShouldApprove(c) {
			c.Next()
			return
		}
		// 复制请求体
		var buf bytes.Buffer
		tee := io.TeeReader(c.Request.Body, &buf)
		requestData, _ := io.ReadAll(tee)
		c.Request.Body = io.NopCloser(&buf)
		// 创建审批流程
		processID, err := config.Handler.CreateApprovalProcess(c, requestData)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "创建审批流程失败",
			})
			//good_http.JsonResponse(c,nil,errors.New("创建审批流程失败"))
			return
		}
		// 返回审批响应
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "请求已提交审批",
			"data": gin.H{
				"approval_id": processID,
				"status":      "pending",
			},
		})
		//good_http.JsonResponse(c,processID,nil)
	}
}
