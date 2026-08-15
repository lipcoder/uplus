package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lipcoder/uplus/internal/auth"
	"github.com/lipcoder/uplus/internal/database"
)

const defaultdatabasePath = "accounts.db"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	client := &http.Client{Timeout: 10 * time.Second}

	authService := auth.NewAuthService(client)
	store, err := database.Open(defaultdatabasePath)
	if err != nil {
		logger.Error("无法打开数据库", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	scanner := bufio.NewScanner(os.Stdin)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, //syscall.SIGTERM表示ctrl+C终止信号
	)
	defer stop()

	fmt.Println("程序已启动")
	fmt.Println("输入 exit 可以退出")

	for {
		fmt.Print("请输入手机号：")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				logger.Error("读取手机号失败", "error", err)
			}
			break
		}
		phone := scanner.Text()
		if phone == "exit" {
			fmt.Println("程序退出")
			break
		}
		if phone == "" {
			fmt.Println("手机号不能为空")
			continue
		}

		fmt.Print("请输入密码：")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				logger.Error("读取密码失败", "error", err)
			}
			break
		}
		password := scanner.Text()
		if password == "" {
			fmt.Println("密码不能为空")
			continue
		}

		fmt.Print("请输入邮箱：")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				logger.Error("读取邮箱失败", "error", err)
			}
			break
		}
		email := scanner.Text()
		if email == "" {
			fmt.Println("邮箱不能为空")
			continue
		}

		token, err := authService.AuthWithPassword(phone, password)
		if err != nil {
			logger.Error("认证失败", "error", err)
			continue
		} else {
			logger.Info("认证成功", "token", token)
		}

		err = store.AddAccount(ctx, phone, password, token, email)
		if err != nil {
			logger.Error("添加账号失败", "error", err)
			continue
		}
		logger.Info("添加账号成功", "phone", phone, "email", email)

	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("读取输入失败: %v\n", err)
	}
	logger.Info("Authentication successful")

}
