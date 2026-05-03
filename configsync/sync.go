package configsync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	templatefs "github.com/goodbye-jack/go-common/templates"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion      = "v1.3.5"
	modulePath          = "github.com/goodbye-jack/go-common"
	metaFileName        = ".go-common-config-meta.yaml"
	latestFileName      = "config.latest.yaml"
	todoFileName        = "config.todo.yaml"
	layeringFileName    = "config.layering.yaml"
	legacyRulesFileName = "config.rules.md"
)

var errGoModNotFound = errors.New("go.mod not found")

type Options struct {
	ProjectDir          string
	TargetVersion       string
	InitializeIfMissing bool
	WriteLatest         bool
	WriteMissing        bool
	Now                 func() time.Time
}

type Result struct {
	ProjectDir         string
	ConfigPath         string
	LatestPath         string
	TodoPath           string
	LayeringPath       string
	RulesPath          string
	MetaPath           string
	TargetVersion      string
	DetectedVersion    string
	PreviousVersion    string
	VersionSource      string
	Initialized        bool
	CreatedConfigPath  bool
	MissingKeys        []string
	DeprecatedKeys     []string
	MissingFromVersion string
}

type Inspection struct {
	StartDir              string
	ProjectDir            string
	GoModPath             string
	ModulePath            string
	HasGoMod              bool
	IsGoCommonModule      bool
	HasGoCommonDependency bool
	DetectedVersion       string
	VersionSource         string
}

type Meta struct {
	ModulePath      string `yaml:"module_path"`
	TemplateVersion string `yaml:"template_version"`
	DetectedVersion string `yaml:"detected_version,omitempty"`
	VersionSource   string `yaml:"version_source,omitempty"`
	ConfigPath      string `yaml:"config_path"`
	Initialized     bool   `yaml:"initialized"`
	SyncedAt        string `yaml:"synced_at"`
	MissingFrom     string `yaml:"missing_from,omitempty"`
	MissingKeyCount int    `yaml:"missing_key_count"`
}

type diffFile struct {
	From       string           `yaml:"from"`
	To         string           `yaml:"to"`
	Added      []diffItem       `yaml:"added"`
	Changed    []diffItem       `yaml:"changed"`
	Deprecated []deprecatedItem `yaml:"deprecated"`
}

type diffItem struct {
	Key     string      `yaml:"key"`
	Module  string      `yaml:"module"`
	Default interface{} `yaml:"default"`
	Comment string      `yaml:"comment"`
}

type deprecatedItem struct {
	Key     string `yaml:"key"`
	NewKey  string `yaml:"new_key,omitempty"`
	Comment string `yaml:"comment,omitempty"`
}

type versionDetection struct {
	Version string
	Source  string
}

type pathResolution struct {
	ConfigPath        string
	MetaReadPath      string
	CreatedConfigPath bool
}

func SyncProjectDefault(projectDir string) (*Result, error) {
	return SyncProject(Options{
		ProjectDir:          projectDir,
		InitializeIfMissing: true,
		WriteLatest:         true,
		WriteMissing:        true,
	})
}

func InspectProject(startDir string) (*Inspection, error) {
	projectDir := strings.TrimSpace(startDir)
	if projectDir == "" {
		projectDir = "."
	}
	absStartDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve start dir failed: %w", err)
	}
	rootDir, goModPath, err := findGoModRoot(absStartDir)
	if err != nil {
		if errors.Is(err, errGoModNotFound) {
			return &Inspection{
				StartDir:      absStartDir,
				ProjectDir:    absStartDir,
				VersionSource: "go.mod:missing",
			}, nil
		}
		return nil, err
	}
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("read go.mod failed: %w", err)
	}
	file, err := modfile.Parse(goModPath, goModData, nil)
	if err != nil {
		return &Inspection{
			StartDir:      absStartDir,
			ProjectDir:    rootDir,
			GoModPath:     goModPath,
			HasGoMod:      true,
			VersionSource: "go.mod:parse_error",
		}, nil
	}
	moduleName := ""
	if file.Module != nil {
		moduleName = strings.TrimSpace(file.Module.Mod.Path)
	}
	requireVersion, hasDependency := extractGoCommonDependency(file)
	detected := determineVersionDetection(requireVersion, file)
	return &Inspection{
		StartDir:              absStartDir,
		ProjectDir:            rootDir,
		GoModPath:             goModPath,
		ModulePath:            moduleName,
		HasGoMod:              true,
		IsGoCommonModule:      moduleName == modulePath,
		HasGoCommonDependency: hasDependency,
		DetectedVersion:       detected.Version,
		VersionSource:         detected.Source,
	}, nil
}

func SyncProject(opts Options) (*Result, error) {
	projectDir := strings.TrimSpace(opts.ProjectDir)
	if projectDir == "" {
		projectDir = "."
	}
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project dir failed: %w", err)
	}
	targetVersion := strings.TrimSpace(opts.TargetVersion)
	if targetVersion == "" {
		targetVersion = CurrentVersion
	}
	writeLatest := opts.WriteLatest
	writeMissing := opts.WriteMissing
	if !writeLatest && !writeMissing {
		writeLatest = true
		writeMissing = true
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	resolved, err := resolveProjectPaths(absProjectDir)
	if err != nil {
		return nil, err
	}
	configPath := resolved.ConfigPath
	artifactDir := filepath.Dir(configPath)
	result := &Result{
		ProjectDir:        absProjectDir,
		ConfigPath:        configPath,
		LatestPath:        filepath.Join(artifactDir, latestFileName),
		TodoPath:          filepath.Join(artifactDir, todoFileName),
		LayeringPath:      filepath.Join(artifactDir, layeringFileName),
		RulesPath:         filepath.Join(artifactDir, buildRulesFileName(targetVersion)),
		MetaPath:          filepath.Join(artifactDir, metaFileName),
		TargetVersion:     targetVersion,
		CreatedConfigPath: resolved.CreatedConfigPath,
	}

	metaReadPath := result.MetaPath
	if resolved.MetaReadPath != "" {
		metaReadPath = resolved.MetaReadPath
	}
	meta, err := loadMeta(metaReadPath)
	if err != nil {
		return nil, fmt.Errorf("load meta failed: %w", err)
	}
	if meta != nil && strings.TrimSpace(meta.TemplateVersion) != "" {
		result.PreviousVersion = strings.TrimSpace(meta.TemplateVersion)
	}

	detected := detectGoCommonVersion(absProjectDir)
	result.DetectedVersion = detected.Version
	result.VersionSource = detected.Source

	if opts.InitializeIfMissing {
		if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
			content, err := readTemplate(targetVersion, "config.initial.yaml")
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				return nil, fmt.Errorf("create config dir failed: %w", err)
			}
			if err := os.WriteFile(configPath, content, 0o644); err != nil {
				return nil, fmt.Errorf("write initial config failed: %w", err)
			}
			result.Initialized = true
		}
	}

	latestTemplateContent, err := readTemplate(targetVersion, "config.latest.yaml")
	if err != nil {
		return nil, err
	}

	configTree, err := loadYAMLTree(configPath)
	if err != nil {
		return nil, fmt.Errorf("load runtime config failed: %w", err)
	}

	missingFrom := chooseMissingFromVersion(meta, targetVersion)
	result.MissingFromVersion = missingFrom

	missingKeys := make([]string, 0)
	missingRoot := newMappingNode()
	var upgradeDiff *diffFile
	if writeMissing {
		if missingFrom != "" && missingFrom != targetVersion {
			upgradeDiff, err = loadUpgradeDiff(missingFrom, targetVersion)
			if err == nil {
				for _, item := range upgradeDiff.Added {
					if strings.TrimSpace(item.Key) == "" || hasConfigPath(configTree, item.Key) || satisfiesByAlternativeKey(configTree, item.Key) {
						continue
					}
					if err := insertNodeValue(missingRoot, strings.Split(item.Key, "."), yamlNodeValue(item.Default)); err != nil {
						return nil, fmt.Errorf("build missing config for %s failed: %w", item.Key, err)
					}
					missingKeys = append(missingKeys, item.Key)
				}
			}
		}
		sort.Strings(missingKeys)
	}
	result.MissingKeys = missingKeys

	deprecatedKeys := make([]string, 0)
	deprecatedRoot := newMappingNode()
	if missingFrom != "" {
		if upgradeDiff == nil {
			upgradeDiff, _ = loadUpgradeDiff(missingFrom, targetVersion)
		}
		if upgradeDiff != nil {
			for _, item := range upgradeDiff.Deprecated {
				if strings.TrimSpace(item.Key) == "" || !hasConfigPath(configTree, item.Key) {
					continue
				}
				deprecatedKeys = append(deprecatedKeys, item.Key)
				node := yamlNodeValue(item.NewKey)
				if node == nil {
					node = yamlNodeValue(item.Comment)
				}
				if err := insertNodeValue(deprecatedRoot, strings.Split(item.Key, "."), node); err != nil {
					return nil, fmt.Errorf("build deprecated config for %s failed: %w", item.Key, err)
				}
			}
		}
	}
	sort.Strings(deprecatedKeys)
	result.DeprecatedKeys = deprecatedKeys

	rulesContent, err := renderRulesDocument(rulesDocumentInput{
		TargetVersion:      targetVersion,
		PreviousVersion:    missingFrom,
		ConfigPath:         relativeOrBase(artifactDir, result.ConfigPath),
		MetaPath:           relativeOrBase(artifactDir, result.MetaPath),
		IncludeLatest:      writeLatest,
		IncludeDiffSummary: writeMissing,
		LatestTemplate:     latestTemplateContent,
		UpgradeDiff:        upgradeDiff,
		MissingKeys:        missingKeys,
		MissingContent:     missingRoot,
		DeprecatedKeys:     deprecatedKeys,
		DeprecatedContent:  deprecatedRoot,
	})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(result.RulesPath, rulesContent, 0o644); err != nil {
		return nil, fmt.Errorf("write rules doc failed: %w", err)
	}
	if err := cleanupLegacySupportArtifacts(result); err != nil {
		return nil, err
	}

	metaOut := Meta{
		ModulePath:      modulePath,
		TemplateVersion: targetVersion,
		DetectedVersion: result.DetectedVersion,
		VersionSource:   result.VersionSource,
		ConfigPath:      relativeOrBase(filepath.Dir(result.MetaPath), configPath),
		Initialized:     result.Initialized,
		SyncedAt:        now().Format(time.RFC3339),
		MissingFrom:     missingFrom,
		MissingKeyCount: len(result.MissingKeys),
	}
	metaBytes, err := yaml.Marshal(&metaOut)
	if err != nil {
		return nil, fmt.Errorf("marshal meta failed: %w", err)
	}
	if err := os.WriteFile(result.MetaPath, metaBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write meta failed: %w", err)
	}

	return result, nil
}

func resolveProjectPaths(projectDir string) (*pathResolution, error) {
	for _, metaPath := range findMetaFiles(projectDir) {
		meta, err := loadMeta(metaPath)
		if err != nil || meta == nil || strings.TrimSpace(meta.ConfigPath) == "" {
			continue
		}
		configPath := meta.ConfigPath
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(filepath.Dir(metaPath), configPath)
		}
		return &pathResolution{
			ConfigPath:        filepath.Clean(configPath),
			MetaReadPath:      metaPath,
			CreatedConfigPath: false,
		}, nil
	}

	candidates := []string{
		filepath.Join(projectDir, "config.yaml"),
		filepath.Join(projectDir, "config.yml"),
		filepath.Join(projectDir, "config", "config.yaml"),
		filepath.Join(projectDir, "config", "config.yml"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return &pathResolution{ConfigPath: path}, nil
		}
	}

	configDir := filepath.Join(projectDir, "config")
	if stat, err := os.Stat(configDir); err == nil && stat.IsDir() {
		return &pathResolution{ConfigPath: filepath.Join(configDir, "config.yaml"), CreatedConfigPath: true}, nil
	}
	return &pathResolution{ConfigPath: filepath.Join(projectDir, "config.yaml"), CreatedConfigPath: true}, nil
}

func findMetaFiles(projectDir string) []string {
	candidates := make([]string, 0)
	preferred := []string{
		filepath.Join(projectDir, metaFileName),
		filepath.Join(projectDir, "config", metaFileName),
	}
	seen := make(map[string]struct{})
	for _, path := range preferred {
		if _, err := os.Stat(path); err == nil {
			candidates = append(candidates, path)
			seen[path] = struct{}{}
		}
	}
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(projectDir, path)
		if relErr != nil {
			return nil
		}
		if rel != "." && depth(rel) > 3 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != metaFileName {
			return nil
		}
		if _, ok := seen[path]; ok {
			return nil
		}
		candidates = append(candidates, path)
		seen[path] = struct{}{}
		return nil
	})
	sort.Strings(candidates)
	return candidates
}

func depth(rel string) int {
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

func detectGoCommonVersion(projectDir string) versionDetection {
	inspection, err := InspectProject(projectDir)
	if err != nil || inspection == nil {
		return versionDetection{Source: "go.mod:missing"}
	}
	return versionDetection{
		Version: inspection.DetectedVersion,
		Source:  inspection.VersionSource,
	}
}

func findGoModRoot(startDir string) (string, string, error) {
	currentDir := filepath.Clean(startDir)
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if stat, err := os.Stat(goModPath); err == nil && !stat.IsDir() {
			return currentDir, goModPath, nil
		}
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", "", errGoModNotFound
		}
		currentDir = parentDir
	}
}

func extractGoCommonDependency(file *modfile.File) (string, bool) {
	var requireVersion string
	hasDependency := false
	for _, req := range file.Require {
		if req.Mod.Path != modulePath {
			continue
		}
		requireVersion = req.Mod.Version
		hasDependency = true
		break
	}
	for _, rep := range file.Replace {
		if rep.Old.Path != modulePath {
			continue
		}
		hasDependency = true
		if rep.New.Version != "" {
			return rep.New.Version, true
		}
	}
	return requireVersion, hasDependency
}

func determineVersionDetection(requireVersion string, file *modfile.File) versionDetection {
	for _, rep := range file.Replace {
		if rep.Old.Path != modulePath {
			continue
		}
		if rep.New.Version != "" {
			return versionDetection{Version: rep.New.Version, Source: "go.mod:replace_version"}
		}
		if requireVersion != "" {
			return versionDetection{Version: requireVersion, Source: "go.mod:replace_local_require"}
		}
		return versionDetection{Version: "local-dev", Source: "go.mod:replace_local"}
	}
	if requireVersion != "" {
		return versionDetection{Version: requireVersion, Source: "go.mod:require"}
	}
	return versionDetection{Source: "go.mod:not_found"}
}

func chooseMissingFromVersion(meta *Meta, targetVersion string) string {
	if meta != nil {
		previousTemplateVersion := strings.TrimSpace(meta.TemplateVersion)
		if previousTemplateVersion != "" && previousTemplateVersion != targetVersion {
			return previousTemplateVersion
		}
		previousMissingFrom := strings.TrimSpace(meta.MissingFrom)
		if previousMissingFrom != "" && previousMissingFrom != targetVersion {
			return previousMissingFrom
		}
	}
	return inferPreviousVersion(targetVersion)
}

func inferPreviousVersion(targetVersion string) string {
	entries, err := fs.Glob(templatefs.FS, "diff/*.yaml")
	if err != nil {
		return ""
	}
	bestFrom := ""
	for _, entry := range entries {
		data, err := templatefs.FS.ReadFile(entry)
		if err != nil {
			continue
		}
		var diff diffFile
		if err := yaml.Unmarshal(data, &diff); err != nil {
			continue
		}
		if strings.TrimSpace(diff.To) == targetVersion {
			fromVersion := strings.TrimSpace(diff.From)
			if fromVersion == "" {
				continue
			}
			if bestFrom == "" || semver.Compare(fromVersion, bestFrom) > 0 {
				bestFrom = fromVersion
			}
		}
	}
	return bestFrom
}

func loadDiff(fromVersion, toVersion string) (*diffFile, error) {
	path := fmt.Sprintf("diff/%s_to_%s.yaml", fromVersion, toVersion)
	data, err := templatefs.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read diff template failed: %w", err)
	}
	var diff diffFile
	if err := yaml.Unmarshal(data, &diff); err != nil {
		return nil, fmt.Errorf("parse diff template failed: %w", err)
	}
	return &diff, nil
}

func loadUpgradeDiff(fromVersion, toVersion string) (*diffFile, error) {
	directDiff, err := loadDiff(fromVersion, toVersion)
	if err == nil {
		return directDiff, nil
	}

	chain, chainErr := resolveDiffChain(fromVersion, toVersion)
	if chainErr != nil {
		return nil, err
	}
	return mergeDiffChain(fromVersion, toVersion, chain), nil
}

func resolveDiffChain(fromVersion, toVersion string) ([]diffFile, error) {
	entries, err := fs.Glob(templatefs.FS, "diff/*.yaml")
	if err != nil {
		return nil, fmt.Errorf("glob diff templates failed: %w", err)
	}

	diffsByFrom := make(map[string][]diffFile)
	for _, entry := range entries {
		data, readErr := templatefs.FS.ReadFile(entry)
		if readErr != nil {
			return nil, fmt.Errorf("read diff template failed: %w", readErr)
		}
		var diff diffFile
		if err := yaml.Unmarshal(data, &diff); err != nil {
			return nil, fmt.Errorf("parse diff template failed: %w", err)
		}
		from := strings.TrimSpace(diff.From)
		to := strings.TrimSpace(diff.To)
		if from == "" || to == "" {
			continue
		}
		diffsByFrom[from] = append(diffsByFrom[from], diff)
	}

	visited := make(map[string]bool)
	chain, ok := walkDiffChain(strings.TrimSpace(fromVersion), strings.TrimSpace(toVersion), diffsByFrom, visited)
	if !ok {
		return nil, fmt.Errorf("no diff chain found: %s -> %s", fromVersion, toVersion)
	}
	return chain, nil
}

func walkDiffChain(currentVersion, targetVersion string, diffsByFrom map[string][]diffFile, visited map[string]bool) ([]diffFile, bool) {
	if currentVersion == targetVersion {
		return []diffFile{}, true
	}
	if visited[currentVersion] {
		return nil, false
	}
	visited[currentVersion] = true
	defer delete(visited, currentVersion)

	nextDiffs := append([]diffFile(nil), diffsByFrom[currentVersion]...)
	sort.SliceStable(nextDiffs, func(i, j int) bool {
		return nextDiffs[i].To < nextDiffs[j].To
	})

	for _, nextDiff := range nextDiffs {
		nextVersion := strings.TrimSpace(nextDiff.To)
		childChain, ok := walkDiffChain(nextVersion, targetVersion, diffsByFrom, visited)
		if !ok {
			continue
		}
		return append([]diffFile{nextDiff}, childChain...), true
	}
	return nil, false
}

func mergeDiffChain(fromVersion, toVersion string, chain []diffFile) *diffFile {
	merged := &diffFile{
		From: fromVersion,
		To:   toVersion,
	}
	addedByKey := make(map[string]diffItem)
	changedByKey := make(map[string]diffItem)
	deprecatedByKey := make(map[string]deprecatedItem)

	for _, diff := range chain {
		for _, item := range diff.Added {
			if strings.TrimSpace(item.Key) == "" {
				continue
			}
			addedByKey[item.Key] = item
		}
		for _, item := range diff.Changed {
			if strings.TrimSpace(item.Key) == "" {
				continue
			}
			changedByKey[item.Key] = item
		}
		for _, item := range diff.Deprecated {
			if strings.TrimSpace(item.Key) == "" {
				continue
			}
			deprecatedByKey[item.Key] = item
		}
	}

	merged.Added = sortDiffItems(addedByKey)
	merged.Changed = sortDiffItems(changedByKey)
	merged.Deprecated = sortDeprecatedItems(deprecatedByKey)
	return merged
}

func sortDiffItems(items map[string]diffItem) []diffItem {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]diffItem, 0, len(keys))
	for _, key := range keys {
		out = append(out, items[key])
	}
	return out
}

func sortDeprecatedItems(items map[string]deprecatedItem) []deprecatedItem {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]deprecatedItem, 0, len(keys))
	for _, key := range keys {
		out = append(out, items[key])
	}
	return out
}

func readTemplate(version, name string) ([]byte, error) {
	path := fmt.Sprintf("releases/%s/%s", version, name)
	data, err := templatefs.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %s failed: %w", path, err)
	}
	return data, nil
}

func buildRulesFileName(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		trimmed = CurrentVersion
	}
	return fmt.Sprintf("go-common-rules.%s.md", trimmed)
}

func cleanupLegacySupportArtifacts(result *Result) error {
	artifactDir := filepath.Dir(result.ConfigPath)
	for _, path := range []string{
		result.LatestPath,
		result.TodoPath,
		filepath.Join(artifactDir, "config.missing.yaml"),
		filepath.Join(artifactDir, "config.deprecated.yaml"),
		result.LayeringPath,
		filepath.Join(artifactDir, legacyRulesFileName),
	} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cleanup legacy support artifact failed: %w", err)
		}
	}
	versionedRules, err := filepath.Glob(filepath.Join(artifactDir, "go-common-rules.v*.md"))
	if err != nil {
		return fmt.Errorf("glob versioned rules doc failed: %w", err)
	}
	for _, path := range versionedRules {
		if filepath.Clean(path) == filepath.Clean(result.RulesPath) {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cleanup old versioned rules doc failed: %w", err)
		}
	}
	return nil
}

func loadMeta(path string) (*Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var meta Meta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func loadYAMLTree(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

func hasConfigPath(tree map[string]interface{}, dotKey string) bool {
	if len(tree) == 0 || strings.TrimSpace(dotKey) == "" {
		return false
	}
	current := interface{}(tree)
	for _, segment := range strings.Split(dotKey, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return false
		}
		value, exists := m[segment]
		if !exists {
			return false
		}
		current = value
	}
	return true
}

func satisfiesByAlternativeKey(tree map[string]interface{}, dotKey string) bool {
	switch strings.TrimSpace(dotKey) {
	case "server.host", "server.port":
		return hasConfigPath(tree, "server.addr")
	default:
		return false
	}
}

func marshalTodoFile(fromVersion, toVersion string, missingKeys []string, missingContent *yaml.Node, deprecatedKeys []string, deprecatedContent *yaml.Node) ([]byte, error) {
	head := []string{
		"# -------------------------------------------------------------------",
		fmt.Sprintf("# go-common 配置待处理清单：%s -> %s", blankAsUnknown(fromVersion), toVersion),
		"# 三部分：summary / missing / deprecated。",
		"# 默认安全模式不会自动写回真实 config.yaml，请人工确认后合并。",
		"# -------------------------------------------------------------------",
		"",
	}
	root := newMappingNode()
	summary := map[string]interface{}{
		"from_version":         blankAsUnknown(fromVersion),
		"to_version":           toVersion,
		"missing_key_count":    len(missingKeys),
		"deprecated_key_count": len(deprecatedKeys),
		"missing_keys":         missingKeys,
		"deprecated_keys":      deprecatedKeys,
	}
	summaryNode, err := toYAMLNode(summary)
	if err != nil {
		return nil, fmt.Errorf("marshal todo summary failed: %w", err)
	}
	upsertNodePair(root, "summary", summaryNode)
	if missingContent == nil || len(missingContent.Content) == 0 {
		upsertNodePair(root, "missing", newMappingNode())
	} else {
		upsertNodePair(root, "missing", missingContent)
	}
	if deprecatedContent == nil || len(deprecatedContent.Content) == 0 {
		upsertNodePair(root, "deprecated", newMappingNode())
	} else {
		upsertNodePair(root, "deprecated", deprecatedContent)
	}
	body, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal todo yaml failed: %w", err)
	}
	return append([]byte(strings.Join(head, "\n")), body...), nil
}

func blankAsUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func relativeOrBase(baseDir, target string) string {
	rel, err := filepath.Rel(baseDir, target)
	if err != nil {
		return filepath.Base(target)
	}
	return filepath.ToSlash(rel)
}

func yamlNodeValue(value interface{}) interface{} {
	if value == nil {
		return ""
	}
	return value
}

func newMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func insertNodeValue(node *yaml.Node, segments []string, value interface{}) error {
	if len(segments) == 0 {
		return nil
	}
	key := segments[0]
	if len(segments) == 1 {
		valueNode, err := toYAMLNode(value)
		if err != nil {
			return err
		}
		upsertNodePair(node, key, valueNode)
		return nil
	}
	child := findChildMap(node, key)
	if child == nil {
		child = newMappingNode()
		upsertNodePair(node, key, child)
	}
	return insertNodeValue(child, segments[1:], value)
}

func toYAMLNode(value interface{}) (*yaml.Node, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ""}, nil
	}
	return doc.Content[0], nil
}

func findChildMap(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			if node.Content[i+1].Kind == yaml.MappingNode {
				return node.Content[i+1]
			}
			return nil
		}
	}
	return nil
}

func upsertNodePair(node *yaml.Node, key string, value *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		node.Kind = yaml.MappingNode
		node.Tag = "!!map"
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}
