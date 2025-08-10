package approval

import (
	"bytes"
	"encoding/base64"
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
		// 构建包含所有请求信息的结构体
		requestInfo := map[string]interface{}{
			"method": c.Request.Method,
			"path":   c.Request.URL.Path,
			"query":  c.Request.URL.Query(),
			"header": c.Request.Header,
		}
		// 处理请求体
		contentType := c.ContentType()
		switch contentType {
		case "application/json": // 处理 JSON 请求
			var jsonData map[string]interface{}
			if err := c.ShouldBindJSON(&jsonData); err == nil {
				requestInfo["body"] = jsonData
			}
		case "multipart/form-data": // 处理表单请求
			form, err := c.MultipartForm()
			if err == nil { // 处理普通字段
				fields := make(map[string]interface{})
				for key, values := range form.Value {
					if len(values) > 0 {
						fields[key] = values[0]
					}
				}
				requestInfo["fields"] = fields
				// 处理文件
				files := make(map[string][]map[string]interface{})
				for key, fileHeaders := range form.File {
					fileList := make([]map[string]interface{}, 0, len(fileHeaders))
					for _, fileHeader := range fileHeaders {
						// 打开文件
						file, err := fileHeader.Open()
						if err != nil {
							c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
							return
						}
						defer file.Close()
						// 读取文件内容
						fileContent, err := io.ReadAll(file)
						if err != nil {
							c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
							return
						}
						// Base64编码（适合小文件）
						encodedContent := base64.StdEncoding.EncodeToString(fileContent)
						fileList = append(fileList, map[string]interface{}{
							"name":        fileHeader.Filename,
							"size":        fileHeader.Size,
							"contentType": fileHeader.Header.Get("Content-Type"),
							"content":     encodedContent, // 存储Base64编码的内容
							"encoded":     true,           // 标记为已编码
						})
					}
					files[key] = fileList
				}
				requestInfo["files"] = files

			}
		case "application/x-www-form-urlencoded":
			// 处理表单编码请求
			var formData map[string]interface{}
			if err := c.ShouldBind(&formData); err == nil {
				requestInfo["body"] = formData
			}
		default: // 其他类型的请求（如纯文本）
			body, _ := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
			requestInfo["rawBody"] = string(body)
		}
		// 创建审批流程
		processID, err := config.Handler.CreateApprovalProcess(c, requestInfo)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "创建审批流程失败"})
			return
		}
		// 返回审批响应
		c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 200, "message": "数据已提交审批,请前往‘审批认证->审批管理->数据审批’中查看进度", "data": gin.H{"approval_id": processID, "status": "pending"}})
	}
}
