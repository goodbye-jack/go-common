package configsync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncProjectInitializesMissingConfigInConfigDir(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "config"), 0o755); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")

	result, err := SyncProject(Options{
		ProjectDir:          projectDir,
		InitializeIfMissing: true,
		WriteLatest:         true,
		WriteMissing:        true,
		Now: func() time.Time {
			return time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("sync project failed: %v", err)
	}
	if !result.Initialized {
		t.Fatal("expected config to be initialized")
	}
	if want := filepath.Join(projectDir, "config", "config.yaml"); result.ConfigPath != want {
		t.Fatalf("unexpected config path: got %s want %s", result.ConfigPath, want)
	}
	assertFileContains(t, result.ConfigPath, "app:")
	if want := filepath.Join(filepath.Dir(result.ConfigPath), buildRulesFileName(CurrentVersion)); result.RulesPath != want {
		t.Fatalf("unexpected rules path: got %s want %s", result.RulesPath, want)
	}
	assertFileContains(t, result.RulesPath, "# 这份文件是干什么的")
	assertFileContains(t, result.RulesPath, "# 这次需要处理的配置项")
	assertFileContains(t, result.RulesPath, "# 常用 yaml 写法示例")
	assertFileContains(t, result.MetaPath, "template_version: "+CurrentVersion)
	assertFileContains(t, result.MetaPath, "config_path: config.yaml")
	if _, err := os.Stat(result.LatestPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected latest artifact to be removed, err=%v", err)
	}
	if _, err := os.Stat(result.TodoPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected todo artifact to be removed, err=%v", err)
	}
	if _, err := os.Stat(result.LayeringPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected layering artifact to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(result.ConfigPath), "config.missing.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected legacy missing artifact to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(result.ConfigPath), "config.deprecated.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected legacy deprecated artifact to be removed, err=%v", err)
	}
	if len(result.MissingKeys) != 0 {
		t.Fatalf("unexpected missing keys for initialized config: %v", result.MissingKeys)
	}
}

func TestSyncProjectUsesExistingRootConfigAndDoesNotOverwrite(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")
	configPath := filepath.Join(projectDir, "config.yaml")
	original := "app:\n  name: relics\nserver:\n  addr: \":9081\"\nworkflow:\n  api:\n    enabled: true\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	result, err := SyncProject(Options{
		ProjectDir:          projectDir,
		InitializeIfMissing: true,
		WriteLatest:         true,
		WriteMissing:        true,
	})
	if err != nil {
		t.Fatalf("sync project failed: %v", err)
	}
	if result.Initialized {
		t.Fatal("did not expect existing config to be overwritten")
	}
	content := readFile(t, configPath)
	if content != original {
		t.Fatalf("existing config changed unexpectedly: %q", content)
	}
}

func TestSyncProjectRespectsMetaConfiguredPath(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")
	customDir := filepath.Join(projectDir, "runtime")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, metaFileName), []byte("config_path: runtime/custom.yaml\ntemplate_version: v1.3.0\n"), 0o644); err != nil {
		t.Fatalf("write meta failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "custom.yaml"), []byte("app:\n  name: custom\n"), 0o644); err != nil {
		t.Fatalf("write custom config failed: %v", err)
	}

	result, err := SyncProject(Options{ProjectDir: projectDir, WriteLatest: true, WriteMissing: true})
	if err != nil {
		t.Fatalf("sync project failed: %v", err)
	}
	if want := filepath.Join(customDir, "custom.yaml"); result.ConfigPath != want {
		t.Fatalf("unexpected meta config path: got %s want %s", result.ConfigPath, want)
	}
	if _, err := os.Stat(filepath.Join(customDir, buildRulesFileName(CurrentVersion))); err != nil {
		t.Fatalf("expected rules doc in runtime dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, latestFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected latest artifact to be removed in runtime dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, todoFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected todo artifact to be removed in runtime dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, layeringFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no layering artifact in runtime dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, "config.missing.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no legacy missing artifact in runtime dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, "config.deprecated.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no legacy deprecated artifact in runtime dir: %v", err)
	}
}

func TestSyncProjectWritesOnlyLocallyMissingAddedKeys(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")
	config := `app:
  name: relics
server:
  addr: ":9081"
workflow:
  api:
    enabled: false
  context:
    user_id_strategy: raw
    user_id_delimiter: "#"
    user_id_header: X-Workflow-User-ID
  directory:
    provider: ldap
  assignment:
    provider: directory
`
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, metaFileName), []byte("template_version: v1.3.0\nconfig_path: config.yaml\n"), 0o644); err != nil {
		t.Fatalf("write meta failed: %v", err)
	}

	result, err := SyncProject(Options{ProjectDir: projectDir, WriteLatest: true, WriteMissing: true})
	if err != nil {
		t.Fatalf("sync project failed: %v", err)
	}
	if result.MissingFromVersion != "v1.3.0" {
		t.Fatalf("unexpected missing from version: %s", result.MissingFromVersion)
	}
	assertFileContains(t, result.RulesPath, "建议直接补入的 YAML 片段")
	assertFileContains(t, result.RulesPath, "role_aliases:")
	assertFileContains(t, result.RulesPath, "candidate_users: nextCandidateUsers")
	for _, key := range result.MissingKeys {
		if key == "workflow.api.enabled" {
			t.Fatal("workflow.api.enabled should not remain missing")
		}
	}
}

func TestSyncProjectKeepsMissingKeysUntilMerged(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")
	config := `app:
  name: relics
server:
  addr: ":9081"
workflow:
  api:
    enabled: false
  identity:
    role_aliases: {}
`
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, metaFileName), []byte("template_version: v1.3.0\nconfig_path: config.yaml\n"), 0o644); err != nil {
		t.Fatalf("write meta failed: %v", err)
	}
	first, err := SyncProject(Options{ProjectDir: projectDir, WriteLatest: true, WriteMissing: true})
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if len(first.MissingKeys) == 0 {
		t.Fatal("expected first sync to produce missing keys")
	}
	second, err := SyncProject(Options{ProjectDir: projectDir, WriteLatest: true, WriteMissing: true})
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if len(second.MissingKeys) == 0 {
		t.Fatal("expected second sync to keep missing keys until config is merged")
	}
	rulesText := readFile(t, second.RulesPath)
	assertFileContains(t, second.RulesPath, "group_aliases: {}")
	if !strings.Contains(rulesText, "workflow:") {
		t.Fatalf("unexpected rules doc content: %s", rulesText)
	}
}

func TestSyncProjectDoesNotReportHostPortMissingWhenAddrExists(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")
	config := `app:
  name: relics
server:
  addr: ":9081"
`
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, metaFileName), []byte("template_version: v1.3.1\nconfig_path: config.yaml\n"), 0o644); err != nil {
		t.Fatalf("write meta failed: %v", err)
	}

	result, err := SyncProject(Options{ProjectDir: projectDir, WriteLatest: true, WriteMissing: true})
	if err != nil {
		t.Fatalf("sync project failed: %v", err)
	}
	for _, key := range result.MissingKeys {
		if key == "server.host" || key == "server.port" {
			t.Fatalf("addr exists, but key still reported missing: %s", key)
		}
	}
}

func TestDetectGoCommonVersionSupportsLocalReplace(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n\nreplace github.com/goodbye-jack/go-common => ../go-common\n")
	detected := detectGoCommonVersion(projectDir)
	if detected.Source != "go.mod:replace_local_require" {
		t.Fatalf("unexpected source: %s", detected.Source)
	}
	if detected.Version != CurrentVersion {
		t.Fatalf("unexpected version: %s", detected.Version)
	}
}

func TestInspectProjectFindsModuleRootFromNestedDir(t *testing.T) {
	projectDir := t.TempDir()
	nestedDir := filepath.Join(projectDir, "internal", "bootstrap")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir failed: %v", err)
	}
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common "+CurrentVersion+"\n")
	inspection, err := InspectProject(nestedDir)
	if err != nil {
		t.Fatalf("inspect project failed: %v", err)
	}
	if inspection.ProjectDir != projectDir {
		t.Fatalf("unexpected project dir: got %s want %s", inspection.ProjectDir, projectDir)
	}
	if !inspection.HasGoMod {
		t.Fatal("expected go.mod to be detected")
	}
	if !inspection.HasGoCommonDependency {
		t.Fatal("expected go-common dependency to be detected")
	}
	if inspection.ModulePath != "example.com/test" {
		t.Fatalf("unexpected module path: %s", inspection.ModulePath)
	}
}

func TestInspectProjectMarksGoCommonSelfModule(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module github.com/goodbye-jack/go-common\n\ngo 1.25.0\n")
	inspection, err := InspectProject(projectDir)
	if err != nil {
		t.Fatalf("inspect project failed: %v", err)
	}
	if !inspection.IsGoCommonModule {
		t.Fatal("expected go-common self module to be marked")
	}
	if inspection.HasGoCommonDependency {
		t.Fatal("go-common self module should not be treated as external dependency")
	}
}

func writeGoMod(t *testing.T, projectDir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("write go.mod failed: %v", err)
	}
}

func assertFileContains(t *testing.T, path, fragment string) {
	t.Helper()
	content := readFile(t, path)
	if !strings.Contains(content, fragment) {
		t.Fatalf("file %s missing fragment %q\ncontent:\n%s", path, fragment, content)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(data)
}
