package configsync

import (
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
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n")

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
	assertFileContains(t, result.ConfigPath, "service_name: your-service-name")
	assertFileContains(t, result.LatestPath, "workflow:")
	assertFileContains(t, result.MissingPath, "workflow:")
	assertFileContains(t, result.MetaPath, "template_version: v1.3.1")
	assertFileContains(t, result.MetaPath, "config_path: config.yaml")
	if len(result.MissingKeys) == 0 {
		t.Fatal("expected missing keys for initial config")
	}
}

func TestSyncProjectUsesExistingRootConfigAndDoesNotOverwrite(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n")
	configPath := filepath.Join(projectDir, "config.yaml")
	original := "service_name: relics\naddr: \":9081\"\nworkflow:\n  api:\n    enabled: true\n"
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
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n")
	customDir := filepath.Join(projectDir, "runtime")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, metaFileName), []byte("config_path: runtime/custom.yaml\ntemplate_version: v1.3.0\n"), 0o644); err != nil {
		t.Fatalf("write meta failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "custom.yaml"), []byte("service_name: custom\n"), 0o644); err != nil {
		t.Fatalf("write custom config failed: %v", err)
	}

	result, err := SyncProject(Options{ProjectDir: projectDir, WriteLatest: true, WriteMissing: true})
	if err != nil {
		t.Fatalf("sync project failed: %v", err)
	}
	if want := filepath.Join(customDir, "custom.yaml"); result.ConfigPath != want {
		t.Fatalf("unexpected meta config path: got %s want %s", result.ConfigPath, want)
	}
	if _, err := os.Stat(filepath.Join(customDir, latestFileName)); err != nil {
		t.Fatalf("expected latest file in runtime dir: %v", err)
	}
}

func TestSyncProjectWritesOnlyLocallyMissingAddedKeys(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n")
	config := `service_name: relics
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
	missingText := readFile(t, result.MissingPath)
	if strings.Contains(missingText, "enabled: false") {
		t.Fatal("workflow.api.enabled already exists and should not be in missing file")
	}
	assertFileContains(t, result.MissingPath, "role_aliases:")
	assertFileContains(t, result.MissingPath, "candidate_users: nextCandidateUsers")
	for _, key := range result.MissingKeys {
		if key == "workflow.api.enabled" {
			t.Fatal("workflow.api.enabled should not remain missing")
		}
	}
}

func TestSyncProjectKeepsMissingKeysUntilMerged(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n")
	config := `service_name: relics
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
	missingText := readFile(t, second.MissingPath)
	assertFileContains(t, second.MissingPath, "group_aliases: {}")
	if !strings.Contains(missingText, "workflow:") {
		t.Fatalf("unexpected missing file content: %s", missingText)
	}
}

func TestDetectGoCommonVersionSupportsLocalReplace(t *testing.T) {
	projectDir := t.TempDir()
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n\nreplace github.com/goodbye-jack/go-common => ../go-common\n")
	detected := detectGoCommonVersion(projectDir)
	if detected.Source != "go.mod:replace_local_require" {
		t.Fatalf("unexpected source: %s", detected.Source)
	}
	if detected.Version != "v1.3.1" {
		t.Fatalf("unexpected version: %s", detected.Version)
	}
}

func TestInspectProjectFindsModuleRootFromNestedDir(t *testing.T) {
	projectDir := t.TempDir()
	nestedDir := filepath.Join(projectDir, "internal", "bootstrap")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir failed: %v", err)
	}
	writeGoMod(t, projectDir, "module example.com/test\n\nrequire github.com/goodbye-jack/go-common v1.3.1\n")
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
