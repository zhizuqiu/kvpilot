package main

import (
	"os"
	"strings"
	"testing"
)

// 验证版本号来自 VERSION 文件且不为空：
// 之前 main.go 里硬编码 0.0.0，导致关于页显示 v0.0.0。
func TestVersionFromFile(t *testing.T) {
	if strings.TrimSpace(version) == "" {
		t.Fatal("版本号不能为空")
	}
	if version == "0.0.0" {
		t.Fatalf("版本号仍为占位值 0.0.0，检查 VERSION 文件与 go:embed 配置")
	}

	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("读取 VERSION 失败: %v", err)
	}
	if want := strings.TrimSpace(string(raw)); version != want {
		t.Fatalf("二进制内版本 %q 与 VERSION 文件 %q 不一致", version, want)
	}
}
