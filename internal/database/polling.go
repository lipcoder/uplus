package database

import (
	"context"
	"fmt"
)

const DefaultPollIntervalSeconds = 2

// initializePolling 将旧的全局查询间隔一次性迁移到每个已有账号。
func (s *Store) initializePolling(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(account_settings)")
	if err != nil {
		return err
	}
	hasInterval := false
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&id, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "poll_interval_seconds" {
			hasInterval = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if hasInterval {
		return tx.Commit()
	}

	seconds := DefaultPollIntervalSeconds
	var legacyTable bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'service_settings')`).Scan(&legacyTable); err != nil {
		return err
	}
	if legacyTable {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT poll_interval_seconds FROM service_settings WHERE id = 1), ?)`, seconds).Scan(&seconds); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE account_settings ADD COLUMN poll_interval_seconds INTEGER NOT NULL DEFAULT 2 CHECK(poll_interval_seconds BETWEEN 1 AND 86400)`); err != nil {
		return fmt.Errorf("迁移账号查询间隔失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE account_settings SET poll_interval_seconds = ?`, seconds); err != nil {
		return err
	}
	// 没有单独签到设置的旧账号也继承原全局间隔，保留默认关闭状态。
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO account_settings(phone, poll_interval_seconds) SELECT phone, ? FROM accounts`, seconds); err != nil {
		return err
	}
	return tx.Commit()
}
