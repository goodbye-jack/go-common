package http

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
)

type StartupLogOptions struct {
	ServiceName       string
	Addr              string
	RouteCount        int
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// LogStartupSuccess 打印统一的启动成功摘要，收口到最后几行，便于开发时快速确认服务、环境和端口。
func LogStartupSuccess(opts StartupLogOptions) {
	appName := strings.TrimSpace(config.GetAppName())
	if appName == "" {
		appName = "-"
	}
	appEnv := strings.TrimSpace(config.GetAppEnv())
	if appEnv == "" {
		appEnv = "-"
	}
	serviceName := strings.TrimSpace(opts.ServiceName)
	if serviceName == "" {
		serviceName = "-"
	}
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "-"
	}

	log.Info("============================================================")
	log.Infof("= 启动成功 | app=%s | env=%s | service=%s", appName, appEnv, serviceName)
	log.Infof("= 监听地址 | listen=%s | local=%s", addr, resolveLocalAccessURL(addr))
	log.Infof("= 运行信息 | gin=%s | routes=%d", gin.Mode(), opts.RouteCount)
	if hasHTTPTimeouts(opts) {
		log.Infof(
			"= HTTP超时 | read_header=%s | read=%s | write=%s | idle=%s",
			opts.ReadHeaderTimeout,
			opts.ReadTimeout,
			opts.WriteTimeout,
			opts.IdleTimeout,
		)
	}
	log.Info("============================================================")
}

func hasHTTPTimeouts(opts StartupLogOptions) bool {
	return opts.ReadHeaderTimeout > 0 || opts.ReadTimeout > 0 || opts.WriteTimeout > 0 || opts.IdleTimeout > 0
}

func resolveLocalAccessURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		trimmed := strings.TrimSpace(addr)
		if trimmed == "" {
			return "-"
		}
		if strings.HasPrefix(trimmed, ":") {
			return fmt.Sprintf("http://127.0.0.1%s", trimmed)
		}
		if _, portErr := strconv.Atoi(trimmed); portErr == nil {
			return fmt.Sprintf("http://127.0.0.1:%s", trimmed)
		}
		return trimmed
	}

	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}
