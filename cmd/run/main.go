package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lipcoder/uplus/internal/config"
	"github.com/lipcoder/uplus/internal/database"
	"github.com/lipcoder/uplus/internal/web"
)

const defaultdatabasePath = "accounts.db"

func main() {
	// 日志器
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := config.Load(".env"); err != nil {
		logger.Error("接口配置加载失败", "error", err)
		os.Exit(1)
	}

	// 根上下文负责把 Ctrl+C/SIGTERM 传递给数据库、HTTP 请求和后台任务。
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// 数据库
	store, err := database.Open(ctx, defaultdatabasePath)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("收到退出信号，程序已停止")
			return
		}
		logger.Error("无法打开数据库", "error", err)
		os.Exit(1)
	}
	defer func(store *database.Store) {
		err := store.Close()
		if err != nil {
			logger.Error("关闭数据库出错", "error", err)
		}
	}(store)
	logger.Info("数据库连接成功")

	addr := os.Getenv("WEB_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8097"
	}
	if err := web.Run(ctx, store, logger, addr); err != nil {
		logger.Error("Web 服务启动失败", "error", err)
		os.Exit(1)
	}
}
