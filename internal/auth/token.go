package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/lipcoder/uplus/internal/app"
)

const (
	defaultGetPemURL    = "https://uc.eduplus.net/spi/login/checkup"
	defaultPostTokenURL = "https://uc.eduplus.net/spi/login/submit"
)

type AuthService struct {
	client *http.Client
}

func NewAuthService(client *http.Client) *AuthService {
	return &AuthService{client: client}
}

func (s *AuthService) AuthWithPassword(phone string, password string) (string, error) {
	publicKey, err := s.GetPublicKey()
	if err != nil {
		return "", fmt.Errorf("获取公钥失败: %w", err)
	}

	encryptedPassword, err := EncryptByPhone(publicKey, phone, password)
	if err != nil {
		return "", fmt.Errorf("加密密码失败: %w", err)
	}

	token, err := s.GetToken(publicKey, encryptedPassword)
	if err != nil {
		return "", fmt.Errorf("获取 token 失败: %w", err)
	}

	return token, nil
}

// 获取公钥
func (s *AuthService) GetPublicKey() (string, error) {
	PublicKey, err := s.getPublicKey()
	if err != nil {
		return "", fmt.Errorf("获取公钥失败: %w", err)
	}

	return PublicKey, nil
}

func (s *AuthService) getPublicKey() (string, error) {
	// 创建一个内存缓冲区，用于保存编码后的 JSON 请求体
	var body bytes.Buffer

	payload := struct {
		Mode             string `json:"mode"`
		LoginAndRegister bool   `json:"loginAndRegister"`
	}{
		Mode:             "Password",
		LoginAndRegister: true,
	}

	// 将请求数据编码为 JSON 并写入请求体
	// json.NewEncoder(&body)创建了一个新的 JSON 编码器，将请求数据编码为 JSON 格式，并将其写入内存缓冲区 body 中
	// Encode 方法将请求数据requestData编码为 JSON 格式，并将其写入内存缓冲区 body 中
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrBuildRequest, err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		defaultGetPemURL,
		&body,
	)

	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrBuildRequest, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "okhttp/4.12.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrHTTPRequest, err)
	}

	var pemResponse struct {
		Code int `json:"code"`

		Data struct {
			EncryptionKey          string `json:"encryptionKey"`
			AuthenticationModeList []any  `json:"authenticationModeList"`
		} `json:"data"`

		Success bool   `json:"success"`
		Tracer  string `json:"tracer"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	}

	if err := json.Unmarshal(respBody, &pemResponse); err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrJSONParse, err)
	}

	publicKey := pemResponse.Data.EncryptionKey

	return publicKey, nil
}

// 使用公钥和加密后的账号密码来获取token
func (s *AuthService) GetToken(publicKey string, cryptogram string) (string, error) {
	token, err := s.getToken(publicKey, cryptogram)
	if err != nil {
		return "", fmt.Errorf("获取 token 失败: %w", err)
	}

	return token, nil
}

func (s *AuthService) getToken(publicKey string, cryptogram string) (string, error) {
	var body bytes.Buffer

	payload := struct {
		Mode          string `json:"mode"`
		EncryptionKey string `json:"encryptionKey"`
		Cryptogram    string `json:"cryptogram"`
	}{
		Mode:          "Password",
		EncryptionKey: publicKey,
		Cryptogram:    cryptogram,
	}

	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrBuildRequest, err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		defaultPostTokenURL,
		&body,
	)

	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrBuildRequest, err)
	}

	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("host", "uc.eduplus.net")
	req.Header.Set("connection", "Keep-Alive")
	req.Header.Set("user-agent", "okhttp/4.12.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrHTTPRequest, err)
	}

	var tokenResponse struct {
		Code int `json:"code"`

		Data struct {
			AccessToken            string `json:"accessToken"`
			RefreshToken           string `json:"refreshToken"`
			AuthenticationModeList []any  `json:"authenticationModeList"`
		} `json:"data"`

		Success bool   `json:"success"`
		Tracer  string `json:"tracer"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	}

	if err := json.Unmarshal(respBody, &tokenResponse); err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrJSONParse, err)
	}

	return tokenResponse.Data.AccessToken, nil
}
