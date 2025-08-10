package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
	"io/ioutil"
	"time"
)

type Operation struct {
	User       string                 `json:"user"`
	Time       time.Time              `json:"time"`
	Path       string                 `json:"path"`
	Method     string                 `json:"method"`
	ClientIP   string                 `json:"client_ip"`
	StatusCode int                    `json:"status_code"`
	Duration   int                    `json:"duration"`
	Body       map[string]interface{} `json:"body"`
	Tips       string                 `json:"tips"`
}

type OpRecordFn func(ctx context.Context, op Operation) error

// 新增：定义一个函数类型，用于从请求中获取 tips（由调用方实现）
type TipsGetter func(path, method string) string

func RecordOperationMiddleware(tipsGetter TipsGetter, fn OpRecordFn) gin.HandlerFunc {
	return func(c *gin.Context) {
		if fn == nil {
			log.Warn("RecorOperationMiddleware callback not set")
			c.Next()
			return
		}
		start := time.Now()
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = ioutil.ReadAll(c.Request.Body)
			c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		// 从外部传入的 tipsGetter 中获取当前请求的 tips
		tips := tipsGetter(c.Request.URL.Path, c.Request.Method)
		c.Next()
		var bodyMap map[string]interface{}
		contentType := c.ContentType()
		if len(bodyBytes) > 0 {
			switch {
			case contentType == "application/json":
				json.Unmarshal(bodyBytes, &bodyMap)
			case contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded":
				c.Request.ParseForm()
				bodyMap = make(map[string]interface{})
				for k, v := range c.Request.PostForm {
					if len(v) == 1 {
						bodyMap[k] = v[0]
					} else {
						bodyMap[k] = v
					}
				}
			}
		}
		clientIP := c.ClientIP()
		user := utils.GetUser(c)
		if u, ok := bodyMap["phone"]; ok {
			if ustr, ok := u.(string); ok {
				user = ustr
			}
		}
		op := Operation{
			User:       user,
			Time:       start,
			Path:       c.Request.URL.Path,
			Method:     c.Request.Method,
			StatusCode: c.Writer.Status(),
			Duration:   int(time.Since(start).Milliseconds()),
			Body:       bodyMap,
			Tips:       tips,
			ClientIP:   clientIP,
		}
		log.Info("RecordOperaionMiddlware callback starting")
		go func() { // 异步记录日志
			_ = fn(context.Background(), op)
		}()
	}
}
