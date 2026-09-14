package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DBPool sizes the PostgreSQL connection pool of one process.
//
// It used to be fixed in code at twenty-five open connections. A process runs up
// to twenty-one background workers beside its HTTP handlers and every replica
// takes a pool of its own, so two replicas asked for fifty connections whatever
// the database allowed. A small managed PostgreSQL, or a PgBouncer in front of
// one, could not be sized down to without editing the source.
type DBPool struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// defaultDBPool is what every deployment ran with before the pool was
// configurable, so leaving the variables unset changes nothing.
var defaultDBPool = DBPool{MaxOpenConns: 25, MaxIdleConns: 10, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute}

// parseDBPool reads the pool through lookup, so it is tested without touching
// the process environment.
//
// Unlike getEnvInt, a value that does not parse is an error, not a quiet fall
// back to the default. A typo in a capacity setting would otherwise surface as
// connection exhaustion under load, long after the deploy that introduced it.
func parseDBPool(lookup func(string) string) (DBPool, error) {
	pool := defaultDBPool
	var err error
	if pool.MaxOpenConns, err = poolInt(lookup, "DB_MAX_OPEN_CONNS", pool.MaxOpenConns); err != nil {
		return DBPool{}, err
	}
	if pool.MaxIdleConns, err = poolInt(lookup, "DB_MAX_IDLE_CONNS", pool.MaxIdleConns); err != nil {
		return DBPool{}, err
	}
	if pool.ConnMaxLifetime, err = poolDuration(lookup, "DB_CONN_MAX_LIFETIME", pool.ConnMaxLifetime); err != nil {
		return DBPool{}, err
	}
	if pool.ConnMaxIdleTime, err = poolDuration(lookup, "DB_CONN_MAX_IDLE_TIME", pool.ConnMaxIdleTime); err != nil {
		return DBPool{}, err
	}
	// An unset idle size follows a lowered open size down. The default of ten
	// belongs to the default of twenty-five: left as it was, the commonest reason
	// to set DB_MAX_OPEN_CONNS at all — a database that allows fewer than ten
	// connections — failed startup over a variable the operator never touched.
	// An idle size that was set explicitly is still held to the rule below.
	if strings.TrimSpace(lookup("DB_MAX_IDLE_CONNS")) == "" && pool.MaxIdleConns > pool.MaxOpenConns {
		pool.MaxIdleConns = max(pool.MaxOpenConns, 0)
	}
	switch {
	case pool.MaxOpenConns < 1:
		return DBPool{}, fmt.Errorf("DB_MAX_OPEN_CONNS must be at least 1")
	case pool.MaxIdleConns < 0:
		return DBPool{}, fmt.Errorf("DB_MAX_IDLE_CONNS must not be negative")
	case pool.MaxIdleConns > pool.MaxOpenConns:
		return DBPool{}, fmt.Errorf("DB_MAX_IDLE_CONNS (%d) must not exceed DB_MAX_OPEN_CONNS (%d)", pool.MaxIdleConns, pool.MaxOpenConns)
	case pool.ConnMaxLifetime <= 0:
		return DBPool{}, fmt.Errorf("DB_CONN_MAX_LIFETIME must be a positive duration")
	case pool.ConnMaxIdleTime <= 0:
		return DBPool{}, fmt.Errorf("DB_CONN_MAX_IDLE_TIME must be a positive duration")
	}
	return pool, nil
}

func poolInt(lookup func(string) string, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(lookup(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number, got %q", key, raw)
	}
	return value, nil
}

func poolDuration(lookup func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(lookup(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 30m, got %q", key, raw)
	}
	return value, nil
}
