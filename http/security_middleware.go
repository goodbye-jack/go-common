package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
)

// SecurityConfig 用于配置全局 XSS / SQL 注入拦截与放行白名单
type SecurityConfig struct {
	EnableXSS    bool
	EnableSQLi   bool
	MaxBodyBytes int64 // 请求体大小限制，<=0 表示不限制

	// 按路径放行，例如："/api/v1/upload_file": true
	BypassPaths map[string]bool

	// 按路径+字段放行，例如：{"/api/v1/search":{"q":true}}
	BypassFields map[string]map[string]bool

	// 自定义危险模式（追加）
	SQLiExtraPatterns []string
	// 额外的 XSS 模式（追加）
	XSSExtraPatterns []string
	// 按路径+方法放行: {"/api/v1/upload_file": {"POST":true}}
	BypassPathMethods map[string]map[string]bool
	// 关闭安全响应头（默认开启）
	DisableSecurityHeaders bool
}

var defaultSQLiPatterns = []string{
	`(?i)\bunion\b.*\bselect\b`,
	`(?i)\bdrop\b\s+\b(table|database|index|view|procedure|function|trigger|schema)\b`,
	`(?i)\bdelete\b\s+from\b`,
	`(?i)\bupdate\b\s+\w+\s+set\b`,
	`(?i)\binsert\b\s+into\b`,
	`(?i)\btruncate\b\s+table\b`,
	`(?i)\balter\b\s+table\b`,
	`(?i)\bcreate\b\s+\b(table|database|index|view|procedure|function|trigger)\b`,
	`(?i)\bexec\b|\bexecute\b`,
	`(?i)\bgrant\b|\brevoke\b`,
	`(?i)\binformation_schema\b`,
	`(?i)\bor\b\s+\b1\s*=\s*1\b`,
	`(?i)\bor\b\s+\b1\s*=\s*'1'\b`,
	`(?i)\bor\b\s+'.*'\s*=\s*'.*'\b`,
	`--\s`, // SQL 注释：-- 后跟空格或换行（避免误报中文破折号）
	`--$`,  // SQL 注释：-- 在行尾
	`/\*`,  // SQL 注释开始
}

var defaultXSSPatterns = []string{
	`(?i)<\s*script[\s\S]*?>`,
	`(?i)<\s*/\s*script\s*>`,
	`(?i)javascript\s*:`,
	`(?i)onerror\s*=`,
	`(?i)onload\s*=`,
	`(?i)onclick\s*=`,
	`(?i)<\s*img[\s\S]*?>`,
	`(?i)<\s*iframe[\s\S]*?>`,
	`(?i)<\s*svg[\s\S]*?>`,
	`(?i)<\s*object[\s\S]*?>`,
	`(?i)<\s*embed[\s\S]*?>`,
}

// SecurityMiddleware 全局安全中间件：
// 1) 设置安全响应头 2) 限制请求体大小 3) 对 Query/Form/JSON 文本做 SQLi/XSS 基础校验(可按路径/字段放行)
func SecurityMiddleware(cfg SecurityConfig) gin.HandlerFunc {
	// 预编译 SQLi 正则
	var sqliRegs []*regexp.Regexp
	patterns := append([]string{}, defaultSQLiPatterns...)
	if len(cfg.SQLiExtraPatterns) > 0 {
		patterns = append(patterns, cfg.SQLiExtraPatterns...)
	}
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			sqliRegs = append(sqliRegs, re)
		}
	}

	// 预编译 XSS 正则
	var xssRegs []*regexp.Regexp
	xssPatterns := append([]string{}, defaultXSSPatterns...)
	if len(cfg.XSSExtraPatterns) > 0 {
		xssPatterns = append(xssPatterns, cfg.XSSExtraPatterns...)
	}
	for _, p := range xssPatterns {
		if re, err := regexp.Compile(p); err == nil {
			xssRegs = append(xssRegs, re)
		}
	}

	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" { // 某些情况下 FullPath 为空，退回 URL.Path
			path = c.Request.URL.Path
		}

		// 1) 设置基础安全响应头（可关闭）
		if !cfg.DisableSecurityHeaders {
			c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
			c.Writer.Header().Set("X-Frame-Options", "SAMEORIGIN")
			c.Writer.Header().Set("Referrer-Policy", "no-referrer")
		}
		// 若仅 JSON API，可考虑： Content-Security-Policy: default-src 'none'

		// 判断是否按路径/方法整体放行
		bypassPath := cfg.BypassPaths != nil && cfg.BypassPaths[path]
		bypassMethod := false
		if cfg.BypassPathMethods != nil {
			if m, ok := cfg.BypassPathMethods[path]; ok && m[strings.ToUpper(c.Request.Method)] {
				bypassMethod = true
			}
		}
		bypassAll := bypassPath || bypassMethod

		// 2) 限制请求体大小（仅对未放行路径生效）
		if !bypassAll && cfg.MaxBodyBytes > 0 && c.Request.Body != nil {
			if cl := c.Request.ContentLength; cl > cfg.MaxBodyBytes && cl > 0 {
				c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
					"success": false,
					"message": fmt.Sprintf("请求体过大，最大允许 %.2f MB", float64(cfg.MaxBodyBytes)/(1024*1024)),
				})
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cfg.MaxBodyBytes)
		}

		// 若此路径整体放行，直接进入下一步（仍保留安全响应头）
		if bypassAll {
			c.Next()
			return
		}

		// 3) 进行基础拦截（轻量，避免破坏业务绑定）：只校验可见的 Query 与 Form 字段；
		// JSON 体不解析为结构体，只做原始字符串浅检，随后恢复 Body 供后续绑定使用。
		if cfg.EnableSQLi || cfg.EnableXSS {
			// 3.1 Query
			for k, vals := range c.Request.URL.Query() {
				if shouldBypassField(cfg, path, k) {
					continue
				}
				for _, v := range vals {
					if (cfg.EnableSQLi && looksLikeSQLi(v, sqliRegs)) || (cfg.EnableXSS && looksLikeXSS(v, xssRegs)) {
						c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数含有sql注入或xss风险内容，请修改再试"})
						return
					}
				}
			}
			// 3.2 Form（包括 multipart 以外的 application/x-www-form-urlencoded）
			if c.Request != nil && c.Request.Method != http.MethodGet {
				_ = c.Request.ParseForm()
				for k, vals := range c.Request.PostForm {
					if shouldBypassField(cfg, path, k) {
						continue
					}
					for _, v := range vals {
						if (cfg.EnableSQLi && looksLikeSQLi(v, sqliRegs)) || (cfg.EnableXSS && looksLikeXSS(v, xssRegs)) {
							c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数含有sql注入或xss风险内容，请修改再试"})
							return
						}
					}
				}
			}
			// 3.3 JSON 原文浅检（不强改内容），仅在 content-type 为 json 且 body 非空时
			ct := c.GetHeader("Content-Type")
			if strings.Contains(ct, "application/json") && c.Request.Body != nil {
				var buf bytes.Buffer
				tee := io.TeeReader(c.Request.Body, &buf)
				raw, _ := io.ReadAll(tee)
				c.Request.Body.Close()
				c.Request.Body = io.NopCloser(&buf)
				rawStr := string(raw)
				if rawStr != "" {
					if (cfg.EnableSQLi && looksLikeSQLi(rawStr, sqliRegs)) || (cfg.EnableXSS && looksLikeXSS(rawStr, xssRegs)) {
						c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数含有sql注入或xss风险内容，请修改再试"})
						return
					}
				}
			}
		}

		c.Next()
	}
}

func shouldBypassField(cfg SecurityConfig, path, field string) bool {
	if cfg.BypassFields == nil {
		return false
	}
	if m, ok := cfg.BypassFields[path]; ok {
		return m[field]
	}
	return false
}

func looksLikeSQLi(s string, regs []*regexp.Regexp) bool {
	// 快速跳过纯字母数字和常见分隔
	if s == "" {
		return false
	}
	// 常见危险字符直拦（尽量减少误伤，具体放行走字段白名单）
	for _, re := range regs {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func looksLikeXSS(s string, regs []*regexp.Regexp) bool {
	if s == "" {
		return false
	}
	for _, re := range regs {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// BuildSecurityConfigFromAppConfig: 可选的外部配置加载（不调用则不生效，保证向后兼容）
// 支持：
//
//	security.enable_xss: true|false
//	security.enable_sqli: true|false
//	security.max_body_bytes: 10485760
//	security.bypass_paths: "/api/v1/upload_file,/health"
//	security.sqli_patterns: "(?i)update\s+.*\s+set"
//	security.xss_patterns: "(?i)<meta[\s\S]*?>"
func BuildSecurityConfigFromAppConfig(base SecurityConfig) SecurityConfig {
	cfg := base
	if v := config.GetConfigString("security.enable_xss"); v != "" {
		cfg.EnableXSS = v == "true" || v == "1"
	}
	if v := config.GetConfigString("security.enable_sqli"); v != "" {
		cfg.EnableSQLi = v == "true" || v == "1"
	}
	if v := config.GetConfigString("security.max_body_bytes"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxBodyBytes = n
		}
	}
	if v := config.GetConfigString("security.bypass_paths"); v != "" {
		if cfg.BypassPaths == nil {
			cfg.BypassPaths = map[string]bool{}
		}
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.BypassPaths[p] = true
			}
		}
	}
	if v := config.GetConfigString("security.sqli_patterns"); v != "" {
		cfg.SQLiExtraPatterns = append(cfg.SQLiExtraPatterns, splitCSV(v)...)
	}
	if v := config.GetConfigString("security.xss_patterns"); v != "" {
		cfg.XSSExtraPatterns = append(cfg.XSSExtraPatterns, splitCSV(v)...)
	}
	return cfg
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
