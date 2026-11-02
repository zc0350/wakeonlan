package db

import "errors"

// ErrNotFound 表示请求的资源不存在。
var ErrNotFound = errors.New("not found")

// ErrAuthFailed 表示用户名或密码错误。
var ErrAuthFailed = errors.New("invalid username or password")

// ErrDuplicate 表示唯一字段（如管理员用户名）重复。
var ErrDuplicate = errors.New("duplicate entry")
