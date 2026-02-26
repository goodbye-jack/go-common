package s3

import (
	"testing"

	"github.com/spf13/viper"
)

func TestNormalizeDir(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		wantDir  string
		wantLv1  string
		wantLv2  string
		wantErr  bool
	}{
		{"empty", "", "", "", "", false},
		{"one", "project", "project", "project", "", false},
		{"two", "project/contract", "project/contract", "project", "contract", false},
		{"trim", "/project/contract/", "project/contract", "project", "contract", false},
		{"overflow", "a/b/c", "", "", "", true},
		{"invalid-dot", "../a", "", "", "", true},
		{"invalid-abs", "/a/b/c", "", "", "", true},
		{"invalid-slash", "a//b", "", "", "", true},
		{"invalid-backslash", "a\\b", "", "", "", true},
		{"invalid-colon", "a:b", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotLv1, gotLv2, err := NormalizeDir(tt.dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err mismatch: %v", err)
			}
			if tt.wantErr {
				return
			}
			if gotDir != tt.wantDir || gotLv1 != tt.wantLv1 || gotLv2 != tt.wantLv2 {
				t.Fatalf("got (%s,%s,%s) want (%s,%s,%s)", gotDir, gotLv1, gotLv2, tt.wantDir, tt.wantLv1, tt.wantLv2)
			}
		})
	}
}

func TestBuildObjectKey(t *testing.T) {
	key, dir, lv1, lv2, err := BuildObjectKey("uploads", "project/contract", "fid", "txt")
	if err != nil {
		t.Fatalf("BuildObjectKey error: %v", err)
	}
	if key != "uploads/project/contract/fid.txt" {
		t.Fatalf("key mismatch: %s", key)
	}
	if dir != "project/contract" || lv1 != "project" || lv2 != "contract" {
		t.Fatalf("dir mismatch: %s/%s/%s", dir, lv1, lv2)
	}

	key, _, _, _, err = BuildObjectKey("uploads", "", "fid", ".txt")
	if err != nil {
		t.Fatalf("BuildObjectKey error: %v", err)
	}
	if key != "uploads/fid.txt" {
		t.Fatalf("key mismatch: %s", key)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	viper.Set(s3EndpointKey, "http://127.0.0.1:9000")
	viper.Set(s3AccessKeyKey, "ak")
	viper.Set(s3SecretKeyKey, "sk")
	viper.Set(s3BucketKey, "bucket")
	viper.Set(s3RegionKey, "")
	viper.Set(s3SignExpireSecondsKey, 0)
	viper.Set(s3BasePrefixKey, "/uploads/")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Region != defaultRegion {
		t.Fatalf("region default mismatch: %s", cfg.Region)
	}
	if cfg.SignExpireSeconds != defaultSignExpireSecond {
		t.Fatalf("sign expire default mismatch: %d", cfg.SignExpireSeconds)
	}
	if cfg.BasePrefix != "uploads" {
		t.Fatalf("base prefix normalize mismatch: %s", cfg.BasePrefix)
	}
}
