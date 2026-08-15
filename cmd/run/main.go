package main

import (
	"context"
	"errors"
	"fmt"
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
	emailer "github.com/lipcoder/uplus/internal/emailer"
	"github.com/lipcoder/uplus/internal/signin"
)

const (
	defaultdatabasePath = "accounts.db" // defaultdatabasePath 表示默认的数据库文件路径
	defaultsignintime   = 10            // defaultsignintime 表示课程签到剩余时间小于多少秒时，开始进行签到操作
)

var (
	emailFrom     string // emailFrom 表示发信邮箱地址
	emailAuthCode string // emailAuthCode 表示发信邮箱授权码
)

func main() {
	// 日志器
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

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
	defer store.Close()
	logger.Info("数据库连接成功")

	// 加载账户数据
	accounts, err := store.LoadAccounts(ctx)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("收到退出信号，程序已停止")
			return
		}
		logger.Error("无法加载账户数据", "error", err)
		os.Exit(1)
	}
	logger.Info("账户数据加载成功", "count", len(accounts))

	// email
	if v := os.Getenv("QQ_MAIL"); v != "" {
		emailFrom = v
		logger.Info("emailFrom 环境变量已覆盖")
	}
	if v := os.Getenv("QQ_MAIL_AUTH_CODE"); v != "" {
		emailAuthCode = v
		logger.Info("emailAuthCode 环境变量已覆盖")
	}
	if emailFrom == "" || emailAuthCode == "" {
		logger.Error("请设置环境变量 QQ_MAIL 和 QQ_MAIL_AUTH_CODE")
		os.Exit(1)
	}
	logger.Info("发信邮箱配置成功", "from", emailFrom)
	mailer := emailer.New(emailFrom, emailAuthCode)

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
		if ctx.Err() != nil {
			logger.Info("收到退出信号，程序已停止")
			return
		}
		if account.Token == "" {
			logger.Error("账户没有token，故障，数据库文件异常", "phone", account.Phone)
			continue
		}
		if account.Token != "" {
			_, err := course.GetCourseSignInID(ctx, account.Token)
			if ctx.Err() != nil {
				logger.Info("收到退出信号，程序已停止")
				return
			}
			// 检查token是否过期
			if errors.Is(err, app.ErrTokenInvalid) {
				if account.Password != "" {
					// 重新获取token
					newToken, err := auth.AuthWithPassword(ctx, account.Phone, account.Password)
					if err != nil {
						if ctx.Err() != nil {
							logger.Info("收到退出信号，程序已停止")
							return
						}
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
		if ctx.Err() != nil {
			logger.Info("收到退出信号，程序已停止")
			return
		}
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
				NewCourseSignInID, err := course.GetCourseSignInID(ctx, account.Token)
				if ctx.Err() != nil {
					return
				}
				if errors.Is(err, app.CourseSignInNill) {
					logger.Info("账户没有开启课程签到", "phone", account.Phone)
					if !sleepContext(ctx, 2*time.Second) {
						return
					}
					continue
				} else if err != nil {
					logger.Error("获取课程签到ID失败", "phone", account.Phone, "error", err)
					return
				}
				logger.Info("获取课程签到ID成功", "phone", account.Phone, "CourseSignInID", NewCourseSignInID)
				CourseSignInID = NewCourseSignInID
				break
			}

			codeDistance, remainingTime, err := course.GetSignInInfoAndParse(ctx, account.Token, CourseSignInID)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Error("获取课程签到信息失败", "phone", account.Phone, "error", err)
				return
			}
			logger.Info("获取课程签到信息成功", "phone", account.Phone, "codeDistance", codeDistance, "remainingTime", remainingTime)

			if err := mailer.Send(account.Email, codeDistance); err != nil { // 发送邮件通知用户课程签到信息
				logger.Error("发送邮件通知失败", "phone", account.Phone, "error", err)
				return
			}
			logger.Info("已发送邮件通知用户课程签到信息", "phone", account.Phone, "email", account.Email, "codeDistance", codeDistance)

			// 如果是验证码签到，则加入等待时间，直到剩余时间小于固定时间时再进行签到操作
			if codeDistance != "200" {
				for {
					if remainingTime <= 0 {
						logger.Info("课程签到时间已过", "phone", account.Phone)
						break
					}
					if remainingTime%5 == 0 {
						logger.Info("课程签到剩余时间", "phone", account.Phone, "remainingTime", remainingTime)
					}
					if !sleepContext(ctx, time.Second) {
						return
					}
					remainingTime--
					if remainingTime < defaultsignintime {
						logger.Info("课程签到剩余时间小于"+fmt.Sprintf("%d", defaultsignintime)+"秒，准备签到", "phone", account.Phone, "remainingTime", remainingTime)
						break
					}
				}
			}

			status, err := sign.SignIn(ctx, account.Token, CourseSignInID, codeDistance)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
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
	if ctx.Err() != nil {
		logger.Info("收到退出信号，所有任务已停止")
	}

}

// sleepContext 与 time.Sleep 类似，但会在上下文取消时立刻返回。
func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
