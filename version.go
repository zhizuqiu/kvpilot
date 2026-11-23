package main

import (
	_ "embed"
	"strings"
)

// embeddedVersion 来自仓库根目录的 VERSION 文件，作为版本号的唯一来源。
//
//go:embed VERSION
var embeddedVersion string

// version 会被注入到系统/偏好服务并展示在关于页。
// 构建时仍可用 -ldflags "-X main.version=x.y.z" 覆盖（发布流水线使用）。
var version = strings.TrimSpace(embeddedVersion)
