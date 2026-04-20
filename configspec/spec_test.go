package configspec

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type moduleSpec struct {
	Module      string     `yaml:"module"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Owner       string     `yaml:"owner"`
	Order       int        `yaml:"order"`
	Items       []itemSpec `yaml:"items"`
}

type itemSpec struct {
	Key               string      `yaml:"key"`
	Kind              string      `yaml:"kind"`
	Type              string      `yaml:"type"`
	Since             string      `yaml:"since"`
	Comment           string      `yaml:"comment"`
	Example           interface{} `yaml:"example"`
	Default           interface{} `yaml:"default"`
	Sensitive         bool        `yaml:"sensitive"`
	CompatibilityOnly bool        `yaml:"compatibility_only"`
}

func TestModuleSpecsBasicValidity(t *testing.T) {
	moduleFiles, err := filepath.Glob(filepath.Join("modules", "*.yaml"))
	if err != nil {
		t.Fatalf("glob module files failed: %v", err)
	}
	if len(moduleFiles) == 0 {
		t.Fatal("no configspec module files found")
	}
	versionPattern := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	seenKeys := map[string]string{}
	for _, moduleFile := range moduleFiles {
		data, err := os.ReadFile(moduleFile)
		if err != nil {
			t.Fatalf("read %s failed: %v", moduleFile, err)
		}
		var spec moduleSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			t.Fatalf("unmarshal %s failed: %v", moduleFile, err)
		}
		if strings.TrimSpace(spec.Module) == "" {
			t.Fatalf("%s has empty module name", moduleFile)
		}
		if strings.TrimSpace(spec.Title) == "" {
			t.Fatalf("%s has empty title", moduleFile)
		}
		if len(spec.Items) == 0 {
			t.Fatalf("%s has no items", moduleFile)
		}
		for _, item := range spec.Items {
			if strings.TrimSpace(item.Key) == "" {
				t.Fatalf("%s contains empty item key", moduleFile)
			}
			if strings.TrimSpace(item.Kind) == "" {
				t.Fatalf("%s item %s has empty kind", moduleFile, item.Key)
			}
			if strings.TrimSpace(item.Comment) == "" {
				t.Fatalf("%s item %s has empty comment", moduleFile, item.Key)
			}
			if !versionPattern.MatchString(item.Since) {
				t.Fatalf("%s item %s has invalid since version %q", moduleFile, item.Key, item.Since)
			}
			if prev, ok := seenKeys[item.Key]; ok {
				t.Fatalf("duplicate config key %s found in %s and %s", item.Key, prev, moduleFile)
			}
			seenKeys[item.Key] = moduleFile
			if item.Sensitive {
				exampleText := strings.ToLower(strings.TrimSpace(toString(item.Example)))
				if strings.Contains(exampleText, "@2026") || strings.Contains(exampleText, "12345678") || strings.Contains(exampleText, "msss@") {
					t.Fatalf("%s item %s contains suspicious real secret-like example value %q", moduleFile, item.Key, exampleText)
				}
			}
		}
	}
}

func TestReleaseTemplatesContainExpectedSections(t *testing.T) {
	initialPath := filepath.Join("..", "templates", "releases", "v1.3.1", "config.initial.yaml")
	latestPath := filepath.Join("..", "templates", "releases", "v1.3.1", "config.latest.yaml")
	compatibilityPath := filepath.Join("..", "templates", "releases", "v1.3.1", "config.compatibility.yaml")

	initial, err := os.ReadFile(initialPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", initialPath, err)
	}
	initialText := string(initial)
	requiredInitialFragments := []string{
		"service_name:",
		"addr:",
		"mysql:",
		"redis:",
		"enabled: false",
		"provider: none",
	}
	for _, fragment := range requiredInitialFragments {
		if !strings.Contains(initialText, fragment) {
			t.Fatalf("initial template missing fragment %q", fragment)
		}
	}

	latest, err := os.ReadFile(latestPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", latestPath, err)
	}
	latestText := string(latest)
	requiredLatestFragments := []string{
		"postgres:",
		"kingbase:",
		"dm:",
		"mongo:",
		"workflow:",
		"directory:",
		"assignment:",
	}
	for _, fragment := range requiredLatestFragments {
		if !strings.Contains(latestText, fragment) {
			t.Fatalf("latest template missing fragment %q", fragment)
		}
	}

	compatibility, err := os.ReadFile(compatibilityPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", compatibilityPath, err)
	}
	compatibilityText := string(compatibility)
	requiredCompatibilityFragments := []string{
		"redis_addr:",
		"redis_password:",
		"ldap_addr:",
		"db_dsn:",
	}
	for _, fragment := range requiredCompatibilityFragments {
		if !strings.Contains(compatibilityText, fragment) {
			t.Fatalf("compatibility template missing fragment %q", fragment)
		}
	}
}

func TestReleaseTemplateYAMLFilesAreParseable(t *testing.T) {
	templateFiles, err := filepath.Glob(filepath.Join("..", "templates", "releases", "v1.3.1", "*.yaml"))
	if err != nil {
		t.Fatalf("glob release template files failed: %v", err)
	}
	diffFiles, err := filepath.Glob(filepath.Join("..", "templates", "diff", "*.yaml"))
	if err != nil {
		t.Fatalf("glob diff template files failed: %v", err)
	}
	templateFiles = append(templateFiles, diffFiles...)
	if len(templateFiles) == 0 {
		t.Fatal("no template yaml files found")
	}
	for _, templateFile := range templateFiles {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			t.Fatalf("read %s failed: %v", templateFile, err)
		}
		var parsed interface{}
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("template %s is not valid yaml: %v", templateFile, err)
		}
	}
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		out, err := yaml.Marshal(v)
		if err != nil {
			return ""
		}
		return string(out)
	}
}
