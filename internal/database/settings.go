package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/lipcoder/uplus/internal/app"
)

const DefaultSignInStartSeconds = 30

// ManagedAccount 包含账号凭据及独立保存的 Web 签到设置。
type ManagedAccount struct {
	app.Account
	ID      int64
	Enabled bool
	// sign_in_seconds 现在表示首次检测到签到后等待的秒数。
	SignInStartSeconds  int
	PollIntervalSeconds int
}

func (s *Store) initializeSettings(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS account_settings (
		phone TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0, 1)),
		sign_in_seconds INTEGER NOT NULL DEFAULT 30 CHECK(sign_in_seconds BETWEEN 0 AND 86400)
	)`)
	if err != nil {
		return fmt.Errorf("创建签到设置表失败: %w", err)
	}
	if err := s.initializePolling(ctx); err != nil {
		return err
	}
	// 沿用原字段保存等待秒数，修正旧版允许的 0–9 秒设置。
	if _, err := s.db.ExecContext(ctx, "UPDATE account_settings SET sign_in_seconds = 10 WHERE sign_in_seconds < 10"); err != nil {
		return fmt.Errorf("更新签到开始秒数失败: %w", err)
	}
	return nil
}

func (s *Store) LoadManagedAccounts(ctx context.Context) ([]ManagedAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.phone, a.password, a.token,
		COALESCE(s.enabled, 0), COALESCE(s.sign_in_seconds, 30), COALESCE(s.poll_interval_seconds, 2)
		FROM accounts a LEFT JOIN account_settings s ON s.phone = a.phone ORDER BY a.id`)
	if err != nil {
		return nil, fmt.Errorf("读取账号设置失败: %w", err)
	}
	defer rows.Close()
	accounts := make([]ManagedAccount, 0)
	for rows.Next() {
		var a ManagedAccount
		if err := rows.Scan(&a.ID, &a.Phone, &a.Password, &a.Token, &a.Enabled, &a.SignInStartSeconds, &a.PollIntervalSeconds); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) SaveSettings(ctx context.Context, phone string, enabled bool, seconds int, pollSeconds int) error {
	if seconds < 10 || seconds > 86400 {
		return errors.New("签到开始秒数必须是 10 到 86400 之间的整数")
	}
	if pollSeconds < 2 || pollSeconds > 86400 {
		return errors.New("查询间隔必须是 2 到 86400 之间的整数秒")
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO account_settings (phone, enabled, sign_in_seconds, poll_interval_seconds)
		SELECT phone, ?, ?, ? FROM accounts WHERE phone = ?
		ON CONFLICT(phone) DO UPDATE SET enabled = excluded.enabled, sign_in_seconds = excluded.sign_in_seconds,
		poll_interval_seconds = excluded.poll_interval_seconds`, enabled, seconds, pollSeconds, phone)
	if err != nil {
		return fmt.Errorf("保存签到设置失败: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("账号不存在")
	}
	return nil
}
