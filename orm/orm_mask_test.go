package orm

import (
	"testing"

	"github.com/goodbye-jack/go-common/utils"
)

func TestMaskDSN(t *testing.T) {
	testCases := []struct {
		name   string
		dbType utils.DBType
		dsn    string
		want   string
	}{
		{
			name:   "mysql",
			dbType: utils.DBTypeMySQL,
			dsn:    "root:123456@tcp(127.0.0.1:3306)/demo?charset=utf8mb4",
			want:   "root:******@tcp(127.0.0.1:3306)/demo?charset=utf8mb4",
		},
		{
			name:   "postgres",
			dbType: utils.DBTypePostgres,
			dsn:    "user=postgres password=secret host=127.0.0.1 port=5432 dbname=demo",
			want:   "user=postgres password=****** host=127.0.0.1 port=5432 dbname=demo",
		},
		{
			name:   "redis",
			dbType: utils.DBTypeRedis,
			dsn:    "redis://:secret@127.0.0.1:6379/0?dial_timeout=5s",
			want:   "redis://:******@127.0.0.1:6379/0?dial_timeout=5s",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := maskDSN(testCase.dsn, testCase.dbType); got != testCase.want {
				t.Fatalf("unexpected masked dsn: got %s want %s", got, testCase.want)
			}
		})
	}
}
