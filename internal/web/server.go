package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lipcoder/uplus/internal/database"
)

//go:embed index.html
var indexHTML string

type accountView struct {
	ID                  string `json:"id"`
	MaskedPhone         string `json:"maskedPhone"`
	Enabled             bool   `json:"enabled"`
	SignInStartSeconds  int    `json:"signInStartSeconds"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	accountStatus
}

func Run(ctx context.Context, store *database.Store, logger *slog.Logger, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newManager(workerCtx, store, logger)
	if err := m.start(); err != nil {
		return err
	}
	defer m.close()
	server := &http.Server{Addr: addr, Handler: m.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
		case <-stopped:
			return
		}
		shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("关闭 Web 服务失败", "error", err)
		}
	}()
	logger.Info("Web 管理已启动", "url", "http://"+listener.Addr().String())
	err = server.Serve(listener)
	cancel()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(w, http.StatusUnsupportedMediaType, "请使用 JSON 请求")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "请求数据格式错误")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, http.StatusBadRequest, "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}

func (m *manager) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, indexHTML)
	})
	mux.HandleFunc("GET /api/accounts", m.list)
	mux.HandleFunc("POST /api/accounts", m.add)
	mux.HandleFunc("PATCH /api/accounts/{id}", m.settings)
	return http.NewCrossOriginProtection().Handler(mux)
}

func maskedPhone(phone string) string {
	chars := []rune(phone)
	if len(chars) <= 7 {
		return strings.Repeat("*", len(chars))
	}
	return string(chars[:3]) + "**" + string(chars[len(chars)-4:])
}

func (m *manager) list(w http.ResponseWriter, r *http.Request) {
	accounts, err := m.store.LoadManagedAccounts(r.Context())
	if err != nil {
		m.logger.Error("加载账号失败", "error", err)
		fail(w, 500, "加载账号失败")
		return
	}
	views := make([]accountView, 0, len(accounts))
	now := time.Now()
	m.statusMu.RLock()
	for _, a := range accounts {
		s, ok := m.statuses[a.Phone]
		if !ok {
			s.Status = "已关闭"
		}
		if !now.Before(s.codeVisibleUntil) {
			s.SignInCode = ""
		}
		masked := maskedPhone(a.Phone)
		// 状态错误中也可能带有手机号，响应前统一去除完整号码。
		s.Status = strings.ReplaceAll(s.Status, a.Phone, masked)
		views = append(views, accountView{
			ID: strconv.FormatInt(a.ID, 10), MaskedPhone: masked,
			Enabled: a.Enabled, SignInStartSeconds: a.SignInStartSeconds,
			PollIntervalSeconds: a.PollIntervalSeconds, accountStatus: s,
		})
	}
	m.statusMu.RUnlock()
	jsonResponse(w, 200, views)
}

func (m *manager) add(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Phone = strings.TrimSpace(input.Phone)
	if input.Phone == "" || input.Password == "" {
		fail(w, 400, "手机号和密码不能为空")
		return
	}
	token, err := m.auth.AuthWithPassword(r.Context(), input.Phone, input.Password)
	if err != nil || token == "" {
		fail(w, 400, "认证失败，请检查手机号和密码后重试")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.SaveAccount(r.Context(), input.Phone, input.Password, token); err != nil {
		fail(w, 500, "保存账号失败")
		return
	}
	accounts, err := m.store.LoadManagedAccounts(r.Context())
	if err != nil {
		fail(w, 500, "读取账号失败")
		return
	}
	for _, a := range accounts {
		if a.Phone == input.Phone {
			m.replace(a)
			break
		}
	}
	jsonResponse(w, 200, map[string]string{"message": "账号已保存"})
}

func (m *manager) settings(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 404, "账号不存在")
		return
	}
	var input struct {
		Enabled             *bool `json:"enabled"`
		SignInStartSeconds  *int  `json:"signInStartSeconds"`
		PollIntervalSeconds *int  `json:"pollIntervalSeconds"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Enabled == nil && input.SignInStartSeconds == nil && input.PollIntervalSeconds == nil {
		fail(w, 400, "请提供要修改的账号设置")
		return
	}
	if input.SignInStartSeconds != nil && (*input.SignInStartSeconds < 10 || *input.SignInStartSeconds > 86400) {
		fail(w, 400, "签到开始秒数必须是 10 到 86400 之间的整数")
		return
	}
	if input.PollIntervalSeconds != nil && (*input.PollIntervalSeconds < 2 || *input.PollIntervalSeconds > 86400) {
		fail(w, 400, "查询间隔必须是 2 到 86400 之间的整数秒")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	accounts, err := m.store.LoadManagedAccounts(r.Context())
	if err != nil {
		fail(w, 500, "读取账号失败")
		return
	}
	for _, a := range accounts {
		if a.ID != id {
			continue
		}
		previous := a
		if input.Enabled != nil {
			a.Enabled = *input.Enabled
		}
		if input.SignInStartSeconds != nil {
			a.SignInStartSeconds = *input.SignInStartSeconds
		}
		if input.PollIntervalSeconds != nil {
			a.PollIntervalSeconds = *input.PollIntervalSeconds
		}
		if a.SignInStartSeconds < 10 || a.SignInStartSeconds > 86400 {
			fail(w, 400, "签到开始秒数必须是 10 到 86400 之间的整数")
			return
		}
		if a.PollIntervalSeconds < 2 || a.PollIntervalSeconds > 86400 {
			fail(w, 400, "查询间隔必须是 2 到 86400 之间的整数秒")
			return
		}
		if err := m.store.SaveSettings(r.Context(), a.Phone, a.Enabled, a.SignInStartSeconds, a.PollIntervalSeconds); err != nil {
			fail(w, 500, "保存设置失败")
			return
		}
		if a.Enabled != previous.Enabled || a.SignInStartSeconds != previous.SignInStartSeconds {
			m.replace(a)
		} else if j, ok := m.jobs[a.Phone]; ok {
			j.polling.setPollInterval(time.Duration(a.PollIntervalSeconds) * time.Second)
		}
		jsonResponse(w, 200, map[string]string{"message": "设置已保存"})
		return
	}
	fail(w, 404, "账号不存在")
}
