package dbconfig

import (
	"testing"

	"github.com/goodbye-jack/go-common/utils"
	"github.com/spf13/viper"
)

func TestConfigGenDSNRelational(t *testing.T) {
	parseTime := true
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "mysql keeps legacy default params when charset absent",
			cfg: Config{
				DBType:   utils.DBTypeMySQL,
				Host:     "127.0.0.1",
				Port:     3306,
				User:     "root",
				Password: "123456",
				Database: "relics_protect",
			},
			want: "root:123456@tcp(127.0.0.1:3306)/relics_protect?parseTime=True&loc=Local",
		},
		{
			name: "mysql structured defaults",
			cfg: Config{
				DBType:    utils.DBTypeMySQL,
				Host:      "127.0.0.1",
				Port:      3306,
				User:      "root",
				Password:  "123456",
				Database:  "relics_protect",
				Charset:   "utf8mb4",
				ParseTime: &parseTime,
				Loc:       "Local",
			},
			want: "root:123456@tcp(127.0.0.1:3306)/relics_protect?charset=utf8mb4&parseTime=True&loc=Local",
		},
		{
			name: "mysql escapes structured params",
			cfg: Config{
				DBType:    utils.DBTypeMySQL,
				Host:      "127.0.0.1",
				Port:      3306,
				User:      "root",
				Password:  "123456",
				Database:  "relics_protect",
				Charset:   "utf8mb4",
				ParseTime: &parseTime,
				Loc:       "Asia/Shanghai",
				Params: map[string]string{
					"timeout": "30s",
				},
			},
			want: "root:123456@tcp(127.0.0.1:3306)/relics_protect?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai&timeout=30s",
		},
		{
			name: "mysql dsn has highest priority",
			cfg: Config{
				DBType: utils.DBTypeMySQL,
				DSN:    "custom-dsn",
			},
			want: "custom-dsn",
		},
		{
			name: "postgres structured defaults",
			cfg: Config{
				DBType:   utils.DBTypePostgres,
				Host:     "127.0.0.1",
				Port:     5432,
				User:     "postgres",
				Password: "postgres",
				Database: "relics",
			},
			want: "user=postgres password=postgres host=127.0.0.1 port=5432 dbname=relics sslmode=disable TimeZone=Asia/Shanghai",
		},
		{
			name: "kingbase structured defaults",
			cfg: Config{
				DBName:   "default",
				DBType:   utils.DBTypeKingBase,
				Host:     "127.0.0.1",
				Port:     54321,
				User:     "system",
				Password: "kingbase",
				Database: "relics",
			},
			want: "user=system password=kingbase host=127.0.0.1 port=54321 dbname=relics sslmode=disable TimeZone=Asia/Shanghai application_name=default",
		},
		{
			name: "dm schema fallback to database",
			cfg: Config{
				DBType:   utils.DBTypeDM,
				Host:     "127.0.0.1",
				Port:     5236,
				User:     "SYSDBA",
				Password: "SYSDBA",
				Database: "RELICS",
			},
			want: "dm://SYSDBA:SYSDBA@127.0.0.1:5236?schema=RELICS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GenDSN(); got != tt.want {
				t.Fatalf("GenDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigGenDSNRedisPasswordModes(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "redis without password",
			cfg: Config{
				DBType:   utils.DBTypeRedis,
				Mode:     utils.DBModeSingle,
				Host:     "127.0.0.1",
				Port:     6379,
				Database: "0",
			},
			want: "redis://127.0.0.1:6379/0?dial_timeout=5s&read_timeout=3s&write_timeout=3s",
		},
		{
			name: "redis with password",
			cfg: Config{
				DBType:   utils.DBTypeRedis,
				Mode:     utils.DBModeSingle,
				Host:     "127.0.0.1",
				Port:     6379,
				Password: "secret",
				Database: "2",
			},
			want: "redis://:secret@127.0.0.1:6379/2?dial_timeout=5s&read_timeout=3s&write_timeout=3s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDefaultValuesByType(&tt.cfg)
			if got := tt.cfg.GenDSN(); got != tt.want {
				t.Fatalf("GenDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadDBConfigSupportsPostgresAliases(t *testing.T) {
	v := viper.New()
	v.Set("databases.pgsql.default.mode", "single")
	v.Set("databases.pgsql.default.host", "127.0.0.1")
	v.Set("databases.pgsql.default.port", 5432)
	v.Set("databases.pgsql.default.user", "postgres")
	v.Set("databases.pgsql.default.password", "postgres")
	v.Set("databases.pgsql.default.database", "relics")

	cfg, err := LoadDBConfig(v, "pgsql.default")
	if err != nil {
		t.Fatalf("LoadDBConfig() error = %v", err)
	}
	if cfg.DBType != utils.DBTypePostgres {
		t.Fatalf("DBType = %q, want %q", cfg.DBType, utils.DBTypePostgres)
	}
	if got := cfg.GenDSN(); got != "user=postgres password=postgres host=127.0.0.1 port=5432 dbname=relics sslmode=disable TimeZone=Asia/Shanghai" {
		t.Fatalf("GenDSN() = %q", got)
	}
}
