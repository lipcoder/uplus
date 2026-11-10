// Package config loads local endpoint settings without executing shell code.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var endpointKeys = []string{
	"UPLUS_AUTH_CHECKUP_URL",
	"UPLUS_AUTH_SUBMIT_URL",
	"UPLUS_COURSE_LIST_URL",
	"UPLUS_COURSE_INFO_URL",
	"UPLUS_SIGN_IN_URL",
}

// Load reads KEY=VALUE assignments. Existing environment variables take priority.
// The file may be absent when all endpoints are provided by the environment.
func Load(path string) error {
	file, err := os.Open(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取 .env 失败: %w", err)
	}
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		values := make(map[string]string)
		for lineNo := 1; scanner.Scan(); lineNo++ {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			key, value, ok := strings.Cut(line, "=")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !ok || !validKey(key) {
				return fmt.Errorf(".env 第 %d 行格式错误", lineNo)
			}
			if _, exists := values[key]; exists {
				return fmt.Errorf(".env 第 %d 行重复配置 %s", lineNo, key)
			}
			if strings.HasPrefix(value, "\"") {
				value, err = strconv.Unquote(value)
				if err != nil {
					return fmt.Errorf(".env 第 %d 行引号格式错误", lineNo)
				}
			} else if strings.HasPrefix(value, "'") {
				if len(value) < 2 || !strings.HasSuffix(value, "'") {
					return fmt.Errorf(".env 第 %d 行引号格式错误", lineNo)
				}
				value = value[1 : len(value)-1]
			}
			values[key] = value
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("读取 .env 失败: %w", err)
		}
		for key, value := range values {
			if _, exists := os.LookupEnv(key); !exists {
				if err := os.Setenv(key, value); err != nil {
					return fmt.Errorf("设置环境变量 %s 失败: %w", key, err)
				}
			}
		}
	}
	for _, key := range endpointKeys {
		value := os.Getenv(key)
		if value == "" {
			return fmt.Errorf("缺少接口配置 %s，请填写 .env 或环境变量", key)
		}
		endpoint, err := url.ParseRequestURI(value)
		if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return fmt.Errorf("接口配置 %s 必须是完整的 HTTP(S) 地址", key)
		}
		if key == "UPLUS_COURSE_INFO_URL" && !strings.HasSuffix(value, "/") {
			return fmt.Errorf("接口配置 %s 必须以 / 结尾", key)
		}
		if key == "UPLUS_SIGN_IN_URL" && !strings.HasSuffix(value, "?signInId=") && !strings.HasSuffix(value, "&signInId=") {
			return fmt.Errorf("接口配置 %s 必须以 ?signInId= 或 &signInId= 结尾", key)
		}
	}
	return nil
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i, c := range key {
		if c != '_' && !(c >= 'A' && c <= 'Z') && !(c >= 'a' && c <= 'z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
