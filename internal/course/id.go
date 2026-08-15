package course

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/lipcoder/uplus/internal/app"
)

const CourseListURL = "https://www.eduplus.net/api/course/courses/v1/study?types=Theory,Train"

type Course struct {
	client *http.Client
}

func NewCourse(client *http.Client) *Course {
	return &Course{client: client}
}

// GetCourseSignInID 获取课程签到 ID，如果没有开启课程签到，则返回 app.CourseSignInNill 错误
func (c *Course) GetCourseSignInID(ctx context.Context, token string) (string, error) {
	body, err := c.GetCourseInfo(ctx, token)
	if err != nil {
		return "", err
	}

	courseSignInID, err := c.ParseCourseInfo(body)
	if err != nil {
		return "", err
	}

	return courseSignInID, nil
}

// GetCourseInfo 获取课程信息，但是不解析课程信息，返回原始的响应体
func (c *Course) GetCourseInfo(ctx context.Context, token string) ([]byte, error) {
	if token == "" {
		return nil, fmt.Errorf("%w: %w", app.ErrTokenNill, app.ErrBuildRequest)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CourseListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", app.ErrBuildRequest, err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("x-access-token", token)
	req.Header.Set("Cookie", "SESSION="+token)
	req.Header.Set("User-Agent", "okhttp/4.12.0")
	req.Header.Set("Host", "www.eduplus.net")
	req.Header.Set("Connection", "Keep-Alive")

	resp, err := c.client.Do(req)
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

type CoureseInfoResponse struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Success bool            `json:"success"`
	Status  int             `json:"status"`
}

type CourseInfoResponseData struct {
	CourseSignInOpen bool   `json:"courseSignInOpen"` // 是否开启课程签到
	CourseSignInID   string `json:"courseSignInId"`   // 课程签到 ID
}

// ParseCourseInfo 解析课程信息，返回课程签到 ID，如果没有开启课程签到，则返回 app.CourseSignInNill 错误
func (c *Course) ParseCourseInfo(body []byte) (string, error) {
	var response CoureseInfoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrParseCourseInfoFailed, err)
	}
	if !response.Success && response.Code != 2000000 {
		return "", fmt.Errorf("%w: status=%d, code=%d", app.ErrTokenInvalid, response.Status, response.Code)
	}

	if response.Data == nil {
		return "", fmt.Errorf("%w: %s", app.ErrParseCourseInfoFailed, "data 字段为空")
	}

	var courses []CourseInfoResponseData
	if err := json.Unmarshal(response.Data, &courses); err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrParseCourseInfoFailed, err)
	}

	for _, course := range courses {
		if !course.CourseSignInOpen {
			continue
		}
		if course.CourseSignInID == "" {
			return "", fmt.Errorf("%w: %s", app.ErrParseCourseInfoFailed, "课程签到 ID 为空")
		}
		return course.CourseSignInID, nil
	}

	return "", app.CourseSignInNill
}
