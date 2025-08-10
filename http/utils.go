package http

import (
	"github.com/gin-gonic/gin"
)

func JsonResponse(c *gin.Context, data interface{}, err error) {
	statusCode := 200
	message := "success"

	if err != nil {
		data = nil
		message = whichError(err)
		statusCode = 500
	}

	c.JSON(statusCode, gin.H{
		"data":    data,
		"message": message,
	})
}

// 业务异常结构体
type BusinessError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *BusinessError) Error() string {
	return e.Message
}

// 参数异常结构体
type ParameterError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// 系统异常
type SystemError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ParameterError) Error() string {
	return e.Message
}

func JsonResponseNew(c *gin.Context, data interface{}, err error) {
	statusCode := 200
	var responseMessage string
	if err != nil {
		data = nil
		switch err := err.(type) {
		case *BusinessError:
			statusCode = err.Code
			responseMessage = err.Message
		case *ParameterError:
			statusCode = err.Code
			responseMessage = err.Message
		default: //
			statusCode = 500
			responseMessage = whichError(err)
		}
	} else {
		responseMessage = "success"
	}
	c.JSON(statusCode, gin.H{
		"data":    data,
		"message": responseMessage,
	})
}

func JsonResponsePage(c *gin.Context, pageNo int, pageSize int, totalCount int64, data interface{}, err error) {
	statusCode := 200
	message := "success"
	if err != nil {
		data = nil
		message = whichError(err)
		statusCode = 500
	}
	c.JSON(statusCode, gin.H{
		"data":      data,
		"page_no":   pageNo,
		"page_size": pageSize,
		"total":     totalCount,
		"message":   message,
	})
}
