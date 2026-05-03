package configsync

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type rulesDocumentInput struct {
	TargetVersion      string
	PreviousVersion    string
	ConfigPath         string
	MetaPath           string
	IncludeLatest      bool
	IncludeDiffSummary bool
	LatestTemplate     []byte
	UpgradeDiff        *diffFile
	MissingKeys        []string
	MissingContent     *yaml.Node
	DeprecatedKeys     []string
	DeprecatedContent  *yaml.Node
}

type yamlBlock struct {
	Name    string
	Content string
}

func renderRulesDocument(input rulesDocumentInput) ([]byte, error) {
	var builder strings.Builder
	currentVersion := strings.TrimSpace(input.TargetVersion)
	if currentVersion == "" {
		currentVersion = CurrentVersion
	}
	previousVersion := blankAsUnknown(input.PreviousVersion)
	configPath := strings.TrimSpace(input.ConfigPath)
	if configPath == "" {
		configPath = "config.yaml"
	}
	metaPath := strings.TrimSpace(input.MetaPath)
	if metaPath == "" {
		metaPath = metaFileName
	}

	builder.WriteString("# 这份文件是干什么的\n\n")
	builder.WriteString(fmt.Sprintf("这份文件由 `go-common %s` 自动生成，是当前项目配置升级和日常配置的唯一主说明。业务侧优先阅读这份文件，不再额外生成 `config.latest.yaml` 和 `config.todo.yaml`。\n\n", currentVersion))
	builder.WriteString(fmt.Sprintf("当前真实配置文件：`%s`。同步元数据文件：`%s`。\n\n", configPath, metaPath))

	builder.WriteString("# 先看这几段\n\n")
	builder.WriteString("1. 先看“本版本升级内容”，判断这次升级会不会影响你当前项目。\n")
	builder.WriteString("2. 再看“这次需要处理的配置项”，这里只列当前项目真正需要补或迁移的内容。\n")
	builder.WriteString("3. 如果不确定配置该写到哪里，再看“配置加载规则”和“基础配置和覆盖配置怎么分工”。\n")
	builder.WriteString("4. 最后按“常用 yaml 写法示例”里的模块片段落到真实配置文件。\n\n")

	builder.WriteString("# 本版本升级内容\n\n")
	builder.WriteString(fmt.Sprintf("- 当前模板版本：`%s`\n", currentVersion))
	builder.WriteString(fmt.Sprintf("- 升级参考起点：`%s`\n", previousVersion))
	if !input.IncludeDiffSummary {
		builder.WriteString("- 当前调用未展开版本差异摘要。\n\n")
	} else if input.UpgradeDiff == nil {
		builder.WriteString("- 当前没有可展开的版本差异摘要；如果这是首次接入，可以直接按后文示例整理真实配置。\n\n")
	} else {
		builder.WriteString(fmt.Sprintf("- 新增配置项：`%d` 个\n", len(input.UpgradeDiff.Added)))
		builder.WriteString(fmt.Sprintf("- 结构变更项：`%d` 个\n", len(input.UpgradeDiff.Changed)))
		builder.WriteString(fmt.Sprintf("- 废弃配置项：`%d` 个\n\n", len(input.UpgradeDiff.Deprecated)))
		builder.WriteString(renderUpgradeItems(input.UpgradeDiff))
	}

	builder.WriteString("# 从上个版本升级到本版本要做什么\n\n")
	builder.WriteString(renderUpgradeActions(input))
	builder.WriteString("\n\n")

	builder.WriteString("# 这次需要处理的配置项\n\n")
	builder.WriteString(renderProjectConfigChanges(input))
	builder.WriteString("\n\n")

	builder.WriteString("# 配置加载规则\n\n")
	builder.WriteString("- 永远先读取真实基础配置文件，默认是 `config.yaml`。\n")
	builder.WriteString("- 如果设置了 `CONFIG_FILE`，会按 `config.yaml + CONFIG_FILE` 的顺序加载，并且忽略 `CONFIG_ENV`。\n")
	builder.WriteString("- 如果没有设置 `CONFIG_FILE`，但设置了 `CONFIG_ENV=local`，就会读取 `config.local.yaml`。\n")
	builder.WriteString("- `CONFIG_ENV=test` 会读取 `config.test.yaml`，`CONFIG_ENV=prod` 会读取 `config.prod.yaml`，其他非空值也会按 `config.<env>.yaml` 规则查找。\n")
	builder.WriteString("- `app.env` 是运行时展示值，不再负责选择环境。不要靠修改 `app.env` 切换本地、测试、生产。\n\n")

	builder.WriteString("# 基础配置和覆盖配置怎么分工\n\n")
	builder.WriteString("- `config.yaml` 只放所有环境都共用的默认值和完整结构骨架。\n")
	builder.WriteString("- `config.local.yaml`、`config.test.yaml`、`config.prod.yaml` 只放差异项，不要整份复制 `config.yaml`。\n")
	builder.WriteString("- 数据库、Redis、对象存储、超时、第三方地址这类明显会按环境变化的字段，优先放到覆盖文件里。\n")
	builder.WriteString("- 应用标识类字段，例如 `app.name`，应该稳定地放在基础配置里，不要按环境漂移。\n")
	builder.WriteString("- 环境覆盖文件里不要再写 `app.env` 这种“看起来像切环境，实际上会和加载规则打架”的字段。\n\n")

	builder.WriteString("# 常见使用方式\n\n")
	builder.WriteString("本地直接启动：\n\n```bash\ngo run main.go\n```\n\n")
	builder.WriteString("本地使用 `config.local.yaml`：\n\n```bash\nCONFIG_ENV=local go run main.go\n```\n\n")
	builder.WriteString("测试环境使用 `config.test.yaml`：\n\n```bash\nCONFIG_ENV=test go run main.go\n```\n\n")
	builder.WriteString("生产环境使用 `config.prod.yaml`：\n\n```bash\nCONFIG_ENV=prod go run main.go\n```\n\n")
	builder.WriteString("使用外部挂载配置文件，且优先级高于 `CONFIG_ENV`：\n\n```bash\nCONFIG_FILE=/data/app/config.prod.yaml go run main.go\n```\n\n")
	builder.WriteString("本地覆盖文件常见写法：\n\n```yaml\nserver:\n  addr: \":9082\"\n\ndatabases:\n  mysql:\n    default:\n      host: 127.0.0.1\n      port: 3306\n  redis:\n    default:\n      host: 127.0.0.1\n      port: 6379\n```\n\n")

	builder.WriteString("# 常用 yaml 写法示例\n\n")
	if input.IncludeLatest {
		builder.WriteString(renderLatestTemplateSections(string(input.LatestTemplate)))
	} else {
		builder.WriteString("当前调用未展开 yaml 片段示例。\n")
	}
	builder.WriteString("\n\n")

	builder.WriteString("# 常见错误写法\n\n")
	builder.WriteString("- 把 `config.yaml` 整份复制到 `config.local.yaml` 或 `config.prod.yaml`，这样后续升级会很难看出真正差异。\n")
	builder.WriteString("- 同时设置 `CONFIG_FILE` 和 `CONFIG_ENV`，但误以为两者会一起生效；实际上 `CONFIG_FILE` 会直接覆盖 `CONFIG_ENV` 选择逻辑。\n")
	builder.WriteString("- 通过修改 `app.env` 来切环境；现在真正决定加载哪个覆盖文件的是 `CONFIG_ENV` 和 `CONFIG_FILE`。\n")
	builder.WriteString("- 看到新增配置项就整块复制，而不确认当前项目是否真的需要该能力。\n")
	builder.WriteString("- 手动修改 `.go-common-config-meta.yaml` 里的路径和版本；这个文件是同步器内部状态，正常情况下不建议手改。\n\n")

	builder.WriteString("# 生成文件说明\n\n")
	builder.WriteString(fmt.Sprintf("- `go-common-rules.%s.md`：当前版本唯一主说明，包含升级内容、当前项目待处理项、加载规则和 yaml 示例。\n", currentVersion))
	builder.WriteString("- `.go-common-config-meta.yaml`：同步器内部状态文件，用来记录真实配置路径、上次同步模板版本和检测到的依赖版本。\n")
	builder.WriteString("- 旧的 `config.latest.yaml`、`config.todo.yaml`、`config.rules.md` 不再生成，避免业务仓库里出现多个阅读入口。\n")

	return []byte(builder.String()), nil
}

func renderUpgradeItems(diff *diffFile) string {
	if diff == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("新增配置项：\n")
	if len(diff.Added) == 0 {
		builder.WriteString("- 当前版本没有新增配置项。\n")
	} else {
		for _, item := range diff.Added {
			builder.WriteString(fmt.Sprintf("- `%s`：%s", item.Key, fallbackText(item.Comment, "新增配置项")))
			if module := strings.TrimSpace(item.Module); module != "" {
				builder.WriteString(fmt.Sprintf("；模块：`%s`", module))
			}
			builder.WriteString(fmt.Sprintf("；默认值：`%s`\n", formatYAMLScalar(item.Default)))
		}
	}

	builder.WriteString("\n结构变更项：\n")
	if len(diff.Changed) == 0 {
		builder.WriteString("- 当前版本没有结构变更项。\n")
	} else {
		for _, item := range diff.Changed {
			builder.WriteString(fmt.Sprintf("- `%s`：%s", item.Key, fallbackText(item.Comment, "结构变更项")))
			if module := strings.TrimSpace(item.Module); module != "" {
				builder.WriteString(fmt.Sprintf("；模块：`%s`", module))
			}
			builder.WriteString(fmt.Sprintf("；建议值：`%s`\n", formatYAMLScalar(item.Default)))
		}
	}

	builder.WriteString("\n废弃配置项：\n")
	if len(diff.Deprecated) == 0 {
		builder.WriteString("- 当前版本没有废弃配置项。\n")
	} else {
		for _, item := range diff.Deprecated {
			line := fmt.Sprintf("- `%s`", item.Key)
			if newKey := strings.TrimSpace(item.NewKey); newKey != "" {
				line += fmt.Sprintf(" -> `%s`", newKey)
			}
			line += fmt.Sprintf("：%s\n", fallbackText(item.Comment, "已废弃，请迁移到新结构"))
			builder.WriteString(line)
		}
	}
	builder.WriteString("\n")
	return builder.String()
}

func renderUpgradeActions(input rulesDocumentInput) string {
	steps := make([]string, 0, 4)
	if len(input.MissingKeys) > 0 {
		steps = append(steps, fmt.Sprintf("1. 先补齐当前项目缺失的 `%d` 个新增配置项，优先处理后文“这次需要处理的配置项”里已经列出的 YAML 片段。", len(input.MissingKeys)))
	} else {
		steps = append(steps, "1. 当前项目没有检测到必须补齐的新增配置项，可以直接做环境配置自检。")
	}
	if len(input.DeprecatedKeys) > 0 {
		steps = append(steps, fmt.Sprintf("2. 把当前项目里仍在使用的 `%d` 个废弃配置项迁移到新 key，避免后续版本继续保留历史结构。", len(input.DeprecatedKeys)))
	} else {
		steps = append(steps, "2. 当前项目没有检测到仍在使用的废弃配置项。")
	}
	steps = append(steps, "3. 检查环境覆盖文件是否只保留差异项，确认没有继续依赖 `app.env` 之类的旧习惯切环境。")
	steps = append(steps, "4. 启动服务后确认日志中的真实配置文件、覆盖配置文件和环境解析结果符合预期。")
	return strings.Join(steps, "\n")
}

func renderProjectConfigChanges(input rulesDocumentInput) string {
	addedByKey := make(map[string]diffItem)
	deprecatedByKey := make(map[string]deprecatedItem)
	if input.UpgradeDiff != nil {
		for _, item := range input.UpgradeDiff.Added {
			addedByKey[item.Key] = item
		}
		for _, item := range input.UpgradeDiff.Deprecated {
			deprecatedByKey[item.Key] = item
		}
	}

	var builder strings.Builder
	if len(input.MissingKeys) == 0 && len(input.DeprecatedKeys) == 0 {
		builder.WriteString("当前项目相对本次模板版本没有检测到待处理配置项。后续如果新增业务模块，再回到“常用 yaml 写法示例”按模块补充即可。\n")
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("- 缺失项数量：`%d`\n", len(input.MissingKeys)))
	builder.WriteString(fmt.Sprintf("- 废弃项数量：`%d`\n\n", len(input.DeprecatedKeys)))

	builder.WriteString("缺失项摘要：\n")
	if len(input.MissingKeys) == 0 {
		builder.WriteString("- 当前项目没有缺失项。\n")
	} else {
		for _, key := range input.MissingKeys {
			item, ok := addedByKey[key]
			if !ok {
				builder.WriteString(fmt.Sprintf("- `%s`：当前项目缺少该配置项。\n", key))
				continue
			}
			builder.WriteString(fmt.Sprintf("- `%s`：%s；建议默认值：`%s`\n", key, fallbackText(item.Comment, "当前项目缺少该配置项"), formatYAMLScalar(item.Default)))
		}
		if text, err := marshalYAMLNode(input.MissingContent); err == nil && text != "" {
			builder.WriteString("\n建议直接补入的 YAML 片段：\n\n```yaml\n")
			builder.WriteString(text)
			builder.WriteString("\n```\n")
		}
	}

	builder.WriteString("\n已废弃项摘要：\n")
	if len(input.DeprecatedKeys) == 0 {
		builder.WriteString("- 当前项目没有仍在使用的废弃项。\n")
	} else {
		for _, key := range input.DeprecatedKeys {
			item, ok := deprecatedByKey[key]
			if !ok {
				builder.WriteString(fmt.Sprintf("- `%s`：当前项目仍在使用该旧配置项，建议迁移。\n", key))
				continue
			}
			line := fmt.Sprintf("- `%s`", key)
			if newKey := strings.TrimSpace(item.NewKey); newKey != "" {
				line += fmt.Sprintf(" -> `%s`", newKey)
			}
			line += fmt.Sprintf("：%s\n", fallbackText(item.Comment, "当前项目仍在使用该旧配置项"))
			builder.WriteString(line)
		}
		if text, err := marshalYAMLNode(input.DeprecatedContent); err == nil && text != "" {
			builder.WriteString("\n当前项目检测到的废弃项映射：\n\n```yaml\n")
			builder.WriteString(text)
			builder.WriteString("\n```\n")
		}
	}
	return builder.String()
}

func renderLatestTemplateSections(templateText string) string {
	blocks := extractTopLevelYAMLBlocks(templateText)
	if len(blocks) == 0 {
		if strings.TrimSpace(templateText) == "" {
			return "当前版本没有可展示的 yaml 片段。\n"
		}
		return "当前版本模板无法按模块拆分，下面保留完整参考：\n\n```yaml\n" + strings.TrimSpace(templateText) + "\n```\n"
	}

	blockByName := make(map[string]yamlBlock, len(blocks))
	order := make([]string, 0, len(blocks))
	for _, block := range blocks {
		blockByName[block.Name] = block
		order = append(order, block.Name)
	}

	descriptions := map[string]string{
		"app":       "应用标识和运行环境展示值。通常项目接入时第一眼就要确认这里。",
		"server":    "服务监听地址、HTTP 超时和统一控制口这类基础网络配置。",
		"security":  "Cookie / Token 等认证基础配置。",
		"storage":   "本地文件工作目录和上传后清理策略。",
		"databases": "统一的数据源初始化配置。按节点存在即尝试初始化，不需要的类型可以删掉。",
		"workflow":  "工作流、目录服务、自动分配等扩展能力配置。没有用到的能力可以先保持关闭或留空。",
	}
	preferredOrder := []string{"app", "server", "security", "storage", "databases", "workflow"}

	var builder strings.Builder
	rendered := make(map[string]struct{}, len(blocks))
	for _, name := range preferredOrder {
		block, ok := blockByName[name]
		if !ok {
			continue
		}
		rendered[name] = struct{}{}
		builder.WriteString(fmt.Sprintf("`%s`：%s\n\n```yaml\n%s\n```\n\n", name, fallbackText(descriptions[name], "当前模块配置示例。"), strings.TrimSpace(block.Content)))
	}

	extraNames := make([]string, 0)
	for _, name := range order {
		if _, ok := rendered[name]; ok {
			continue
		}
		extraNames = append(extraNames, name)
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		block := blockByName[name]
		builder.WriteString(fmt.Sprintf("`%s`：当前模块配置示例。\n\n```yaml\n%s\n```\n\n", name, strings.TrimSpace(block.Content)))
	}
	return builder.String()
}

func extractTopLevelYAMLBlocks(templateText string) []yamlBlock {
	lines := strings.Split(templateText, "\n")
	type sectionStart struct {
		name string
		idx  int
	}
	starts := make([]sectionStart, 0)
	for idx, line := range lines {
		if !isTopLevelYAMLKey(line) {
			continue
		}
		name := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
		starts = append(starts, sectionStart{name: name, idx: idx})
	}

	blocks := make([]yamlBlock, 0, len(starts))
	for idx, start := range starts {
		end := len(lines)
		if idx+1 < len(starts) {
			end = starts[idx+1].idx
		}
		content := strings.TrimSpace(strings.Join(lines[start.idx:end], "\n"))
		if content == "" {
			continue
		}
		blocks = append(blocks, yamlBlock{Name: start.name, Content: content})
	}
	return blocks
}

func isTopLevelYAMLKey(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	if line[0] == ' ' || line[0] == '\t' {
		return false
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return false
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return false
	}
	return !strings.ContainsAny(key, " \"'")
}

func marshalYAMLNode(node *yaml.Node) (string, error) {
	if node == nil || len(node.Content) == 0 {
		return "", nil
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func formatYAMLScalar(value interface{}) string {
	if value == nil {
		return ""
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return strings.TrimSpace(string(data))
}

func fallbackText(text, fallback string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
