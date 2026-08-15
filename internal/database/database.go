package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/lipcoder/uplus/internal/app"
	_ "modernc.org/sqlite"
)

// Store 封装账号数据库连接和常用持久化操作。
type Store struct {
	db *sql.DB
}

// Open 打开 SQLite 数据库并初始化账号表。
func Open(ctx context.Context, path string) (*Store, error) {

	if path == "" {
		return nil, errors.New("数据库路径不能为空")
	}

	// 返回一个数据库连接池对象
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 数据库失败: %w", err)
	}
	// 设置数据库连接池的最大打开连接数为 1，确保在任何时刻只有一个连接被使用，从而避免并发访问数据库时可能出现的锁竞争问题。
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	if path != ":memory:" {
		if err := os.Chmod(path, 0o600); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("设置数据库文件权限失败: %w", err)
		}
	}

	return store, nil
}

// initialize 创建账号表。
func (s *Store) initialize(ctx context.Context) error {
	const schema = /* sql */ `
	CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		phone TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		token TEXT NOT NULL,
		email TEXT NOT NULL
	);`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("创建账号表失败: %w", err)
	}

	return nil
}

// SaveAccount 新增账号；手机号已存在时更新密码、token 和邮箱。
func (s *Store) SaveAccount(
	ctx context.Context,
	phone string,
	password string,
	token string,
	email string,
) error {
	if phone == "" {
		return errors.New("手机号不能为空")
	}
	if token == "" {
		return errors.New("token 不能为空")
	}
	if email == "" {
		return errors.New("邮箱不能为空")
	}
	const query = /* sql */ `
	INSERT INTO accounts (phone, password, token, email)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(phone) DO UPDATE SET
		password = excluded.password,
		token = excluded.token,
		email = excluded.email;
	`

	if _, err := s.db.ExecContext(ctx, query, phone, password, token, email); err != nil {
		return fmt.Errorf("保存账号失败: %w", err)
	}

	return nil

}

// AddAccount 添加账号。
// 如果手机号已经存在，则只更新 token，保留原密码和邮箱。
func (s *Store) AddAccount(
	ctx context.Context,
	phone string,
	password string,
	token string,
	email string,
) error {
	if phone == "" {
		return errors.New("手机号不能为空")
	}
	if password == "" {
		return errors.New("密码不能为空")
	}
	if token == "" {
		return errors.New("token 不能为空")
	}
	if email == "" {
		return errors.New("邮箱不能为空")
	}

	// 先检查手机号是否已经存在
	var exists bool
	err := s.db.QueryRowContext(
		ctx,
		"SELECT EXISTS(SELECT 1 FROM accounts WHERE phone = ?)",
		phone,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("检查账号是否存在失败: %w", err)
	}

	// 已存在，直接更新 token
	if exists {
		return s.UpdateToken(ctx, phone, token)
	}

	// 不存在则新增
	const query = /* sql */ `
	INSERT INTO accounts (phone, password, token, email)
	VALUES (?, ?, ?, ?);
	`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		phone,
		password,
		token,
		email,
	); err != nil {
		return fmt.Errorf("添加账号失败: %w", err)
	}

	return nil
}

// UpdateToken 按手机号更新 token，并保留原密码。
func (s *Store) UpdateToken(ctx context.Context, phone string, token string) error {
	if phone == "" || token == "" {
		return errors.New("手机号和 token 不能为空")
	}

	result, err := s.db.ExecContext(
		ctx,
		"UPDATE accounts SET token = ? WHERE phone = ?",
		token,
		phone,
	)
	if err != nil {
		return fmt.Errorf("更新 token 失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("检查 token 更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("手机号 %s 不存在", phone)
	}

	return nil

}

// DeleteAccount 按手机号删除账号及其保存的所有账号信息。
func (s *Store) DeleteAccount(ctx context.Context, phone string) error {
	if phone == "" {
		return errors.New("手机号不能为空")
	}

	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM accounts WHERE phone = ?",
		phone,
	)
	if err != nil {
		return fmt.Errorf("删除账号失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("检查账号删除结果失败: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("手机号 %s 不存在", phone)
	}

	return nil
}

// LoadAccounts 按写入顺序读取全部账号凭据。
func (s *Store) LoadAccounts(ctx context.Context) ([]app.Account, error) {
	const query = /* sql */ `
	SELECT phone, password, token, email
	FROM accounts
	ORDER BY id
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("读取账号失败: %w", err)
	}
	defer rows.Close()

	var accounts []app.Account
	for rows.Next() {
		var item app.Account
		if err := rows.Scan(&item.Phone, &item.Password, &item.Token, &item.Email); err != nil {
			return nil, fmt.Errorf("解析账号失败: %w", err)
		}
		accounts = append(accounts, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历账号失败: %w", err)
	}

	return accounts, nil

}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}
