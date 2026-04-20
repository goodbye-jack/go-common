package templates

import "embed"

// FS 内置版本化配置模板与差异文件，供业务项目同步器直接读取。
//
//go:embed releases/*/*.yaml diff/*.yaml
var FS embed.FS
