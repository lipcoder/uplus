package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
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

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	client := &http.Client{Timeout: 10 * time.Second}

	authService := auth.NewAuthService(client)
	store, err := database.Open(ctx, defaultdatabasePath)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("收到退出信号，程序已停止")
			return
		}
		logger.Error("无法打开数据库", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	scanner := bufio.NewScanner(os.Stdin)
	input := scanInput(ctx, scanner)

	fmt.Println("程序已启动")
	fmt.Println("输入 exit 可以退出")

	for {
		phone, err := readInput(ctx, input, "请输入手机号：")
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if err != io.EOF {
				logger.Error("读取手机号失败", "error", err)
			}
			break
		}
		if phone == "exit" {
			fmt.Println("程序退出")
			break
		}
		if phone == "" {
			fmt.Println("手机号不能为空")
			continue
		}

		password, err := readInput(ctx, input, "请输入密码：")
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if err != io.EOF {
				logger.Error("读取密码失败", "error", err)
			}
			break
		}
		if password == "" {
			fmt.Println("密码不能为空")
			continue
		}

		email, err := readInput(ctx, input, "请输入邮箱：")
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if err != io.EOF {
				logger.Error("读取邮箱失败", "error", err)
			}
			break
		}
		if email == "" {
			fmt.Println("邮箱不能为空")
			continue
		}

		token, err := authService.AuthWithPassword(ctx, phone, password)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Error("认证失败", "error", err)
			continue
		} else {
			logger.Info("认证成功", "token", token)
		}

		err = store.AddAccount(ctx, phone, password, token, email)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Error("添加账号失败", "error", err)
			continue
		}
		logger.Info("添加账号成功", "phone", phone, "email", email)

	}
	if ctx.Err() != nil {
		logger.Info("收到退出信号，程序已停止")
		return
	}
	logger.Info("程序已退出")

}

type inputResult struct {
	text string
	err  error
	done bool
}

// scanInput 将阻塞的 Scanner 与主控制流隔离，使主 goroutine 可以响应 ctx 取消。
func scanInput(ctx context.Context, scanner *bufio.Scanner) <-chan inputResult {
	results := make(chan inputResult)
	go func() {
		defer close(results)
		for scanner.Scan() {
			select {
			case results <- inputResult{text: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case results <- inputResult{err: scanner.Err(), done: true}:
		case <-ctx.Done():
		}
	}()
	return results
}

func readInput(ctx context.Context, input <-chan inputResult, prompt string) (string, error) {
	fmt.Print(prompt)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result, ok := <-input:
		if !ok || result.done {
			if result.err != nil {
				return "", result.err
			}
			return "", io.EOF
		}
		if result.err != nil {
			return "", result.err
		}
		return result.text, nil
	}
}
