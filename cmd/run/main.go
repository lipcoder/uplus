package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lipcoder/uplus/internal/app"
	"github.com/lipcoder/uplus/internal/auth"
	"github.com/lipcoder/uplus/internal/course"
	"github.com/lipcoder/uplus/internal/database"
	"github.com/lipcoder/uplus/internal/signin"
)

const defaultdatabasePath = "accounts.db"

func main() {
	// 日志器
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// 数据库
	store, err := database.Open(defaultdatabasePath)
	if err != nil {
		logger.Error("无法打开数据库", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("数据库连接成功")

	// 项目级上下文，用于处理系统信号（如 SIGINT 和 SIGTERM），以便在接收到这些信号时优雅地关闭应用程序。
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, //syscall.SIGTERM表示ctrl+C终止信号
	)
	defer stop()

	// 加载账户数据
	accounts, err := store.LoadAccounts(ctx)
	if err != nil {
		logger.Error("无法加载账户数据", "error", err)
		os.Exit(1)
	}

	if len(accounts) == 0 {
		logger.Info("没有找到账户数据，请先添加账户")
		os.Exit(0)
	}

	tokenclient := &http.Client{
		Timeout: 10 * time.Second,
	}

	courseclient := &http.Client{
		Timeout: 5 * time.Second,
	}

	signinclient := &http.Client{
		Timeout: 10 * time.Second,
	}

	course := course.NewCourse(courseclient)
	auth := auth.NewAuthService(tokenclient)
	sign := signin.NewSignInService(signinclient)

	logger.Info("开始检查账户token")

	// 检测是否有账户token信息
	for _, account := range accounts {
		if account.Token == "" {
			logger.Error("账户没有token，故障，数据库文件异常", "phone", account.Phone)
			continue
		}
		if account.Token != "" {
			_, err := course.GetCourseSignInID(account.Token)
			// 检查token是否过期
			if errors.Is(err, app.ErrTokenInvalid) {
				if account.Password != "" {
					// 重新获取token
					newToken, err := auth.AuthWithPassword(account.Phone, account.Password)
					if err != nil {
						logger.Error("重新获取token失败", "phone", account.Phone, "error", err)
						if err := store.DeleteAccount(ctx, account.Phone); err != nil {
							logger.Error("删除账户信息失败", "phone", account.Phone, "error", err)
						} else {
							logger.Info("已删除账户信息", "phone", account.Phone)
						}
						continue
					}
					account.Token = newToken
					err = store.SaveAccount(ctx, account.Phone, account.Password, newToken, account.Email)
					if err != nil {
						logger.Error("保存账户信息失败", "phone", account.Phone, "error", err)
						continue
					}
					logger.Info("重新获取token成功", "phone", account.Phone)
				} else {
					logger.Error("账户没有密码，无法重新获取token，请先登录", "phone", account.Phone)
					continue
				}
			} else if errors.Is(err, app.CourseSignInNill) {
				continue
			} else if err != nil {
				logger.Error("检查token失败", "phone", account.Phone, "error", err)
				continue
			}
			logger.Info("账户token有效", "phone", account.Phone)
		}
	}

	logger.Info("账户token检查完成")

	// 再次加载账户数据，确保获取到最新的token信息
	accounts, err = store.LoadAccounts(ctx)
	if err != nil {
		logger.Error("无法加载账户数据", "error", err)
		os.Exit(1)
	}

	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		go func(account app.Account) {

			defer wg.Done()

			if account.Token == "" {
				logger.Error("账户没有token，故障，数据库文件异常", "phone", account.Phone)
				return
			}
			var CourseSignInID string
			for {
				NewCourseSignInID, err := course.GetCourseSignInID(account.Token)
				if errors.Is(err, app.CourseSignInNill) {
					logger.Info("账户没有开启课程签到", "phone", account.Phone)
					time.Sleep(2 * time.Second)
					continue
				} else if err != nil {
					logger.Error("获取课程签到ID失败", "phone", account.Phone, "error", err)
					return
				}
				logger.Info("获取课程签到ID成功", "phone", account.Phone, "CourseSignInID", NewCourseSignInID)
				CourseSignInID = NewCourseSignInID
				break
			}

			codeDistance, remainingTime, err := course.GetSignInInfoAndParse(account.Token, CourseSignInID)
			if err != nil {
				logger.Error("获取课程签到信息失败", "phone", account.Phone, "error", err)
				return
			}
			logger.Info("获取课程签到信息成功", "phone", account.Phone, "codeDistance", codeDistance, "remainingTime", remainingTime)

			status, err := sign.SignIn(account.Token, CourseSignInID, codeDistance)
			if err != nil {
				logger.Error("课程签到失败", "phone", account.Phone, "error", err)
				return
			}
			if status {
				logger.Info("课程签到成功", "phone", account.Phone)
			} else {
				logger.Error("课程签到失败", "phone", account.Phone)
			}

		}(account)
	}

	wg.Wait()

}
