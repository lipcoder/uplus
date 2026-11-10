package config_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lipcoder/uplus/internal/auth"
	"github.com/lipcoder/uplus/internal/config"
	"github.com/lipcoder/uplus/internal/course"
	"github.com/lipcoder/uplus/internal/signin"
)

func endpoints() map[string]string {
	return map[string]string{
		"UPLUS_AUTH_CHECKUP_URL": "https://mock.invalid/checkup",
		"UPLUS_AUTH_SUBMIT_URL":  "https://mock.invalid/submit",
		"UPLUS_COURSE_LIST_URL":  "https://mock.invalid/courses?types=Theory,Train",
		"UPLUS_COURSE_INFO_URL":  "https://mock.invalid/clock/",
		"UPLUS_SIGN_IN_URL":      "https://mock.invalid/study?signInId=",
	}
}

func TestLoadFileAndEnvironmentPrecedence(t *testing.T) {
	expected := endpoints()
	var lines []string
	for key, value := range expected {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, "export "+key+"='"+value+"'")
	}
	expected["UPLUS_AUTH_SUBMIT_URL"] = "https://override.invalid/submit"
	t.Setenv("UPLUS_AUTH_SUBMIT_URL", expected["UPLUS_AUTH_SUBMIT_URL"])
	lines = append(lines, "# A comment", "", `UPLUS_TEST_QUOTED="contains=equals"`)
	t.Setenv("UPLUS_TEST_QUOTED", "")
	if err := os.Unsetenv("UPLUS_TEST_QUOTED"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	for key, want := range expected {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
	if got := os.Getenv("UPLUS_TEST_QUOTED"); got != "contains=equals" {
		t.Fatal(got)
	}
}

func TestLoadRejectsInvalidEndpoints(t *testing.T) {
	cases := []struct{ name, key, value string }{
		{"missing", "UPLUS_AUTH_CHECKUP_URL", ""},
		{"relative", "UPLUS_AUTH_SUBMIT_URL", "/submit"},
		{"scheme", "UPLUS_COURSE_LIST_URL", "ftp://mock.invalid/courses"},
		{"course suffix", "UPLUS_COURSE_INFO_URL", "https://mock.invalid/clock"},
		{"sign-in suffix", "UPLUS_SIGN_IN_URL", "https://mock.invalid/study"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range endpoints() {
				t.Setenv(key, value)
			}
			t.Setenv(tc.key, tc.value)
			err := config.Load(filepath.Join(t.TempDir(), "absent.env"))
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected error for %s, got %v", tc.key, err)
			}
		})
	}
}

func TestLoadRejectsMalformedFile(t *testing.T) {
	for _, content := range []string{"not an assignment", "BAD-KEY=value", `KEY="unclosed`, "KEY='unclosed", "KEY=a\nKEY=b"} {
		t.Run(content, func(t *testing.T) {
			for key, value := range endpoints() {
				t.Setenv(key, value)
			}
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if err := config.Load(path); err == nil {
				t.Fatal("expected malformed-file error")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Intercept every HTTP request in memory, so no real login or sign-in is sent.
func TestAllRequestsUseConfiguredEndpoints(t *testing.T) {
	for key, value := range endpoints() {
		t.Setenv(key, value)
	}
	if err := config.Load(filepath.Join(t.TempDir(), "absent.env")); err != nil {
		t.Fatal(err)
	}
	expected := []struct{ method, target, body string }{
		{"POST", "https://mock.invalid/checkup", `{"data":{"encryptionKey":"public-key"}}`},
		{"POST", "https://mock.invalid/submit", `{"data":{"accessToken":"access-token"}}`},
		{"GET", "https://mock.invalid/courses?types=Theory,Train", `{}`},
		{"GET", "https://mock.invalid/clock/signin-123/student", `{}`},
		{"POST", "https://mock.invalid/study?signInId=signin-123", `{"data":true}`},
		{"POST", "https://mock.invalid/study?signInId=signin-123&codeDistance=1234", `{"data":true}`},
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls >= len(expected) {
			t.Fatal("unexpected additional request")
		}
		want := expected[calls]
		calls++
		if r.Method != want.method || r.URL.String() != want.target {
			t.Fatalf("got %s %s, want %s %s", r.Method, r.URL, want.method, want.target)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(want.body)), Request: r}, nil
	})}
	ctx := context.Background()
	a := auth.NewAuthService(client)
	if key, err := a.GetPublicKey(ctx); err != nil || key != "public-key" {
		t.Fatalf("public key: %q %v", key, err)
	}
	if token, err := a.GetToken(ctx, "public-key", "ciphertext"); err != nil || token != "access-token" {
		t.Fatalf("token: %q %v", token, err)
	}
	c := course.NewCourse(client)
	if _, err := c.GetCourseInfo(ctx, "access-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSignInInfo(ctx, "access-token", "signin-123"); err != nil {
		t.Fatal(err)
	}
	s := signin.NewSignInService(client)
	for _, code := range []string{"200", "1234"} {
		if ok, err := s.SignIn(ctx, "access-token", "signin-123", code); err != nil || !ok {
			t.Fatalf("sign-in: %v %v", ok, err)
		}
	}
	if calls != len(expected) {
		t.Fatalf("got %d requests, want %d", calls, len(expected))
	}
}
