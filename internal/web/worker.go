package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/lipcoder/uplus/internal/app"
	"github.com/lipcoder/uplus/internal/auth"
	"github.com/lipcoder/uplus/internal/course"
	"github.com/lipcoder/uplus/internal/database"
	"github.com/lipcoder/uplus/internal/signin"
)

type courseService interface {
	GetCourseSignInID(context.Context, string) (string, error)
	GetSignInInfoAndParse(context.Context, string, string) (string, int, error)
}
type authService interface {
	AuthWithPassword(context.Context, string, string) (string, error)
}
type signService interface {
	SignIn(context.Context, string, string, string) (bool, error)
}

const signInCodeDisplayDuration = 20 * time.Second

type accountStatus struct {
	Status           string `json:"status"`
	RemainingSeconds *int   `json:"remainingSeconds"`
	CourseSignInID   string `json:"courseSignInId"`
	LastSignIn       string `json:"lastSignIn"`
	SignInCode       string `json:"signInCode,omitempty"`
	codeVisibleUntil time.Time
}
type job struct {
	cancel  context.CancelFunc
	done    chan struct{}
	polling *pollingControl
}

type signInKey struct{ phone, courseID string }

type manager struct {
	ctx       context.Context
	store     *database.Store
	logger    *slog.Logger
	course    courseService
	auth      authService
	sign      signService
	mu        sync.Mutex // 串行处理设置保存、账号更新及任务替换。
	jobs      map[string]job
	statusMu  sync.RWMutex
	statuses  map[string]accountStatus
	completed map[string]string
	detected  map[signInKey]time.Time
}

func newManager(ctx context.Context, store *database.Store, logger *slog.Logger) *manager {
	m := &manager{ctx: ctx, store: store, logger: logger,
		course: course.NewCourse(&http.Client{Timeout: 5 * time.Second}),
		auth:   auth.NewAuthService(&http.Client{Timeout: 10 * time.Second}),
		sign:   signin.NewSignInService(&http.Client{Timeout: 10 * time.Second}),
		jobs:   make(map[string]job), statuses: make(map[string]accountStatus), completed: make(map[string]string), detected: make(map[signInKey]time.Time)}
	return m
}

func (m *manager) start() error {
	accounts, err := m.store.LoadManagedAccounts(m.ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range accounts {
		m.replace(a)
	}
	return nil
}

// replace 必须在持有 mu 时调用；等待旧任务退出后再创建新任务，避免重复签到。
func (m *manager) replace(a database.ManagedAccount) {
	if old, ok := m.jobs[a.Phone]; ok {
		old.cancel()
		<-old.done
		delete(m.jobs, a.Phone)
	}
	m.setStatus(a.Phone, "已关闭", nil, "")
	if !a.Enabled || m.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	j := job{cancel: cancel, done: make(chan struct{}), polling: newPollingControl(a.PollIntervalSeconds)}
	m.jobs[a.Phone] = j
	m.setStatus(a.Phone, "等待课程签到", nil, "")
	go func() { defer close(j.done); m.work(ctx, a, j.polling) }()
}

func (m *manager) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		j.cancel()
	}
	for phone, j := range m.jobs {
		<-j.done
		delete(m.jobs, phone)
	}
}

func (m *manager) setStatus(phone, text string, remaining *int, id string) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	s := m.statuses[phone]
	s.Status, s.RemainingSeconds, s.CourseSignInID = text, remaining, id
	m.statuses[phone] = s
}

// 签到码独立于运行状态保留，至少显示 20 秒，课程仍在进行时继续显示。
func (m *manager) setSignInCode(phone, code string, remaining int) {
	if code == "" || code == "200" {
		return
	}
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	s := m.statuses[phone]
	s.SignInCode = code
	s.codeVisibleUntil = time.Now().Add(max(signInCodeDisplayDuration, time.Duration(remaining)*time.Second))
	m.statuses[phone] = s
}

func (m *manager) report(phone string, err error) {
	m.setStatus(phone, err.Error(), nil, "")
	m.logger.Error("账号签到任务失败", "phone", phone, "error", err)
}

func sleep(ctx context.Context, duration time.Duration) bool {
	t := time.NewTimer(duration)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// 同一账号的同一次签到沿用首次发现时间，重试及修改设置不会重置计时。
func (m *manager) firstDetected(phone, id string) time.Time {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	key := signInKey{phone, id}
	if detected, ok := m.detected[key]; ok {
		return detected
	}
	detected := time.Now()
	m.detected[key] = detected
	return detected
}

func signInReadyAt(detected time.Time, seconds int) time.Time {
	return detected.Add(time.Duration(max(10, seconds)) * time.Second)
}

func (m *manager) work(ctx context.Context, a database.ManagedAccount, polling *pollingControl) {
	for ctx.Err() == nil {
		m.check(ctx, &a)
		if !polling.waitForPoll(ctx, time.Now()) {
			return
		}
	}
}

func (m *manager) check(ctx context.Context, a *database.ManagedAccount) {
	id, err := m.course.GetCourseSignInID(ctx, a.Token)
	if errors.Is(err, app.ErrTokenInvalid) || errors.Is(err, app.ErrTokenNill) {
		token, authErr := m.auth.AuthWithPassword(ctx, a.Phone, a.Password)
		if authErr != nil {
			if ctx.Err() == nil {
				m.report(a.Phone, fmt.Errorf("重新认证失败: %w", authErr))
			}
			return
		}
		if err := m.store.UpdateToken(ctx, a.Phone, token); err != nil {
			m.report(a.Phone, err)
			return
		}
		a.Token = token
		id, err = m.course.GetCourseSignInID(ctx, a.Token)
	}
	if ctx.Err() != nil {
		return
	}
	if errors.Is(err, app.CourseSignInNill) {
		m.setStatus(a.Phone, "等待课程签到", nil, "")
		return
	}
	if err != nil {
		m.report(a.Phone, err)
		return
	}
	m.statusMu.RLock()
	completed := m.completed[a.Phone] == id
	m.statusMu.RUnlock()
	if completed {
		return
	}
	detected := m.firstDetected(a.Phone, id)
	code, remaining, err := m.course.GetSignInInfoAndParse(ctx, a.Token, id)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		m.report(a.Phone, err)
		return
	}
	m.setSignInCode(a.Phone, code, remaining)
	if code != "200" && remaining <= 0 {
		m.setStatus(a.Phone, "本次签到已结束", nil, id)
		return
	}
	deadline := time.Now().Add(time.Duration(remaining) * time.Second)
	hasDeadline := remaining > 0
	readyAt := signInReadyAt(detected, a.SignInStartSeconds)
	var courseRemaining *int
	for {
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		if hasDeadline {
			seconds := max(0, int((deadline.Sub(now)+time.Second-1)/time.Second))
			courseRemaining = &seconds
			if !now.Before(deadline) {
				m.setStatus(a.Phone, "本次签到已结束", nil, id)
				return
			}
		}
		if !now.Before(readyAt) {
			break
		}
		waitSeconds := int((readyAt.Sub(now) + time.Second - 1) / time.Second)
		m.setStatus(a.Phone, fmt.Sprintf("等待签到：%d 秒后执行", waitSeconds), courseRemaining, id)
		if !sleep(ctx, time.Second) {
			return
		}
	}
	m.setStatus(a.Phone, "正在签到", courseRemaining, id)
	ok, err := m.sign.SignIn(ctx, a.Token, id, code)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		m.report(a.Phone, err)
		return
	}
	if !ok {
		m.report(a.Phone, errors.New("签到失败，稍后重试"))
		return
	}
	m.statusMu.Lock()
	m.completed[a.Phone] = id
	s := m.statuses[a.Phone]
	s.Status, s.RemainingSeconds, s.LastSignIn = "签到成功", nil, time.Now().Format(time.RFC3339)
	m.statuses[a.Phone] = s
	m.statusMu.Unlock()
	m.logger.Info("课程签到成功", "phone", a.Phone, "courseSignInId", id)
}
