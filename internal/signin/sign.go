package signin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/lipcoder/uplus/internal/app"
)

type SignIn struct {
	client *http.Client
}

func NewSignInService(client *http.Client) *SignIn {
	return &SignIn{client: client}
}

const PasswdSignInURL = "https://www.eduplus.net/api/course/clock_in/study?signInId="

func (s *SignIn) SignIn(Token string, CourseSignInID string, CodeDistance string) (bool, error) {
	if Token == "" {
		return false, app.ErrTokenNill
	}

	if CourseSignInID == "" {
		return false, app.CourseSignInNill
	}

	var RequestURL string
	if CodeDistance == "200" {
		RequestURL = PasswdSignInURL + url.PathEscape(CourseSignInID)
	} else {
		RequestURL = PasswdSignInURL + url.PathEscape(CourseSignInID) + "&codeDistance=" + CodeDistance
	}

	req, err := http.NewRequest(http.MethodPost, RequestURL, nil)
	if err != nil {
		return false, app.ErrBuildRequest
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Access-Token", Token)
	req.Header.Set("Cookie", "SESSION="+Token)
	req.Header.Set("User-Agent", "okhttp/4.12.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return false, app.ErrHTTPRequest
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, app.ErrHTTPRequest
	}

	type SignInResponse struct {
		Code    int    `json:"code"`
		Data    bool   `json:"data"`
		Success bool   `json:"success"`
		Status  string `json:"status"`
	}

	var signInResponse SignInResponse
	if err := json.Unmarshal(body, &signInResponse); err != nil {
		return false, app.ErrJSONParse
	}

	return signInResponse.Data, nil
}
