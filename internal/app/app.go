package app

import "errors"

type Account struct {
	Phone    string
	Password string
	Token    string
}

var (
	ErrBuildRequest = errors.New("构建请求失败")     // ErrBuildRequest 表示登录请求无法完成构建。
	ErrHTTPRequest  = errors.New("HTTP 请求失败")  // ErrHTTPRequest 表示登录接口请求或响应读取失败。
	ErrJSONParse    = errors.New("解析 JSON 失败") // ErrJSONParse 表示登录接口响应无法解析。
)

var (
	ErrTokenNill    = errors.New("token 为空") // ErrTokenNill 表示 token 为空。
	ErrTokenInvalid = errors.New("token 无效") // ErrTokenInvalid 表示 token 过期或无效。
)

var (
	CourseSignInNill = errors.New("课程暂无签到") // CourseSignInNill 表示暂无课程签到信息。
)
