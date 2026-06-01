package localpager

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	DefaultDBPath        = "~/.local/state/localpager/localpager.sqlite"
	DefaultRepo          = "owner/repo"
	DefaultProcessorName = "github_interest_classifier"
	DefaultProcessorVer  = "v1"
)

type Pool struct {
	gdb   *gorm.DB
	sqlDB *sql.DB
}

func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func NewPool(ctx context.Context, path string) (*Pool, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(expanded), 0o755); err != nil {
		return nil, err
	}
	gdb, err := gorm.Open(sqlite.Open(sqliteDSN(expanded)), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm database: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get gorm sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	pool := &Pool{gdb: gdb, sqlDB: sqlDB}
	if err := pool.AutoMigrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := pool.BackfillGenericIdentity(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return pool, nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	pragmas := []string{
		"_pragma=busy_timeout(60000)",
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=foreign_keys(ON)",
	}
	return path + separator + strings.Join(pragmas, "&")
}

func (p *Pool) AutoMigrate(ctx context.Context) error {
	if p == nil || p.gdb == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	if err := p.gdb.WithContext(ctx).AutoMigrate(autoMigrateModels()...); err != nil {
		return fmt.Errorf("gorm auto-migrate models: %w", err)
	}
	return nil
}

func (p *Pool) BackfillGenericIdentity(ctx context.Context) error {
	if p == nil || p.gdb == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return p.gdb.WithContext(ctx).Exec(`
UPDATE localpager_items
SET source = 'gitcrawl',
    type = source_kind,
    ref = source_ref
WHERE source IS NULL
  AND type IS NULL
  AND ref IS NULL
  AND source_kind IN ('github_pr', 'github_issue')`).Error
}

func (p *Pool) GORM() *gorm.DB {
	if p == nil {
		return nil
	}
	return p.gdb
}

func (p *Pool) Close() error {
	if p == nil || p.sqlDB == nil {
		return nil
	}
	return p.sqlDB.Close()
}
