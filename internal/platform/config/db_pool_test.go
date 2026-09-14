package config

import (
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestAnUnsetPoolKeepsTheSizeEveryDeploymentAlreadyRan(t *testing.T) {
	pool, err := parseDBPool(environment(nil))
	if err != nil {
		t.Fatalf("parseDBPool() error = %v", err)
	}
	if pool != (DBPool{MaxOpenConns: 25, MaxIdleConns: 10, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute}) {
		t.Fatalf("pool = %+v, want the previous fixed values", pool)
	}
}

func TestThePoolCanBeSizedToTheDatabase(t *testing.T) {
	// A small managed database, or a PgBouncer, needs the pool smaller than the
	// old fixed twenty-five; that used to take a code change.
	pool, err := parseDBPool(environment(map[string]string{
		"DB_MAX_OPEN_CONNS": "8", "DB_MAX_IDLE_CONNS": "0",
		"DB_CONN_MAX_LIFETIME": "10m", "DB_CONN_MAX_IDLE_TIME": " 90s ",
	}))
	if err != nil {
		t.Fatalf("parseDBPool() error = %v", err)
	}
	want := DBPool{MaxOpenConns: 8, MaxIdleConns: 0, ConnMaxLifetime: 10 * time.Minute, ConnMaxIdleTime: 90 * time.Second}
	if pool != want {
		t.Fatalf("pool = %+v, want %+v", pool, want)
	}
}

func TestAPoolSettingThatCannotBeHonouredFailsTheBoot(t *testing.T) {
	for name, testCase := range map[string]struct {
		env  map[string]string
		want string
	}{
		// getEnvInt would have ignored this and kept 25.
		"typo in the size":        {map[string]string{"DB_MAX_OPEN_CONNS": "2O"}, "DB_MAX_OPEN_CONNS"},
		"no connections":          {map[string]string{"DB_MAX_OPEN_CONNS": "0"}, "DB_MAX_OPEN_CONNS"},
		"negative idle":           {map[string]string{"DB_MAX_IDLE_CONNS": "-1"}, "DB_MAX_IDLE_CONNS"},
		"more idle than open":     {map[string]string{"DB_MAX_OPEN_CONNS": "5", "DB_MAX_IDLE_CONNS": "10"}, "must not exceed"},
		"lifetime without a unit": {map[string]string{"DB_CONN_MAX_LIFETIME": "30"}, "DB_CONN_MAX_LIFETIME"},
		"zero lifetime":           {map[string]string{"DB_CONN_MAX_LIFETIME": "0s"}, "DB_CONN_MAX_LIFETIME"},
		"negative idle time":      {map[string]string{"DB_CONN_MAX_IDLE_TIME": "-5m"}, "DB_CONN_MAX_IDLE_TIME"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseDBPool(environment(testCase.env))
			if err == nil {
				t.Fatalf("parseDBPool(%v) accepted a pool it cannot honour", testCase.env)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

func TestLoweringOnlyTheOpenSizeKeepsTheIdleSizeWithinIt(t *testing.T) {
	// Sizing the pool down to a small database is why the variable exists. It
	// must not also require setting DB_MAX_IDLE_CONNS, whose default of ten
	// would otherwise exceed the new limit and fail startup.
	pool, err := parseDBPool(environment(map[string]string{"DB_MAX_OPEN_CONNS": "5"}))
	if err != nil {
		t.Fatalf("parseDBPool() error = %v", err)
	}
	if pool.MaxOpenConns != 5 || pool.MaxIdleConns != 5 {
		t.Fatalf("pool = %+v, want open 5 and idle lowered to 5", pool)
	}
}
