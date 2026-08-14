package course

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/lipcoder/uplus/internal/app"
)

const CourseInfoURL = "https://www.eduplus.net/api/course/clock_in/"

// GetSignInInfoAndParse 获取课程签到信息和签到码
func (s *Course) GetSignInInfoAndParse(Token string, CourseSignInID string) (string, int, error) {
	body, err := s.GetSignInInfo(Token, CourseSignInID)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", "获取课程签到信息失败", err)
	}

	codeDistance, remainingTime, err := s.ParseSignInInfo(body)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", "解析课程签到信息失败", err)
	}

	return codeDistance, remainingTime, nil
}

// GetSignInInfo 获取课程签到信息，但是不解析课程签到信息，返回原始的响应体
func (s *Course) GetSignInInfo(Token string, CourseSignInID string) ([]byte, error) {
	if Token == "" {
		return nil, app.ErrTokenNill
	}

	if CourseSignInID == "" {
		return nil, app.CourseSignInNill
	}

	requestURL := CourseInfoURL + url.PathEscape(CourseSignInID) + "/student"

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", app.ErrBuildRequest, err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Access-Token", Token)
	req.Header.Set("Cookie", "SESSION="+Token)
	req.Header.Set("User-Agent", "okhttp/4.12.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", app.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", app.ErrHTTPRequest, err)
	}

	return body, nil
}

type SignInResponse struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Success bool            `json:"success"`
	Status  string          `json:"status"`
}

type SignInResponseData struct {
	CodeDistance  string `json:"codeDistance"`
	RemainingTime int    `json:"remainingTime"`
}

// ParseSignInInfo 解析课程签到信息，返回签到码和剩余时间
func (s *Course) ParseSignInInfo(body []byte) (string, int, error) {
	var response SignInResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", 0, fmt.Errorf("%w: %w", app.ErrJSONParse, err)
	}

	var data SignInResponseData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return "", 0, fmt.Errorf("%w: %w", app.ErrJSONParse, err)
	}

	return data.CodeDistance, data.RemainingTime, nil
}
