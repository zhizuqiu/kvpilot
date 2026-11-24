package redis

import (
	"strings"
	"testing"
)

// 验证 AUTH 的密码不会出现在命令历史或日志里。
func TestRedactArgsAuth(t *testing.T) {
	got := commandLine([]interface{}{"auth", "secret123"})
	if got != "auth ***" {
		t.Fatalf("got %q, want %q", got, "auth ***")
	}
}

// 验证 HELLO 握手中的密码被隐藏，用户名保留便于排查。
func TestRedactArgsHelloAuth(t *testing.T) {
	got := commandLine([]interface{}{"hello", "2", "auth", "default", "secret123", "setname", "kvpilot"})
	want := "hello 2 auth default *** setname kvpilot"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// 验证 CONFIG SET 只隐藏敏感参数，普通参数保持可读。
func TestRedactArgsConfigSet(t *testing.T) {
	got := commandLine([]interface{}{"config", "set", "requirepass", "secret123"})
	if got != "config set requirepass ***" {
		t.Fatalf("敏感配置未脱敏: %q", got)
	}

	// 多组参数混排时只掩掉敏感项
	got = commandLine([]interface{}{"config", "set", "maxmemory", "64mb", "masterauth", "secret123"})
	if !strings.Contains(got, "maxmemory 64mb") {
		t.Fatalf("普通配置不应被隐藏: %q", got)
	}
	if strings.Contains(got, "secret123") {
		t.Fatalf("masterauth 未脱敏: %q", got)
	}

	// CONFIG GET 不属于写入路径，保持原样
	if got := commandLine([]interface{}{"config", "get", "maxmemory"}); got != "config get maxmemory" {
		t.Fatalf("CONFIG GET 不应被改写: %q", got)
	}
}

// 验证 ACL SETUSER 的明文密码令牌被隐藏，其余规则保留。
func TestRedactArgsAclSetUser(t *testing.T) {
	got := commandLine([]interface{}{
		"acl", "setuser", "app", "reset", "on", ">secret123", "<old456", "~cache:*", "+get",
	})
	if strings.Contains(got, "secret123") || strings.Contains(got, "old456") {
		t.Fatalf("ACL 密码未脱敏: %q", got)
	}
	if !strings.Contains(got, "~cache:*") || !strings.Contains(got, "+get") {
		t.Fatalf("非密码规则应保留: %q", got)
	}
}

// 验证哨兵 auth-pass 与 MIGRATE 的认证参数被隐藏。
func TestRedactArgsSentinelAndMigrate(t *testing.T) {
	got := commandLine([]interface{}{"sentinel", "set", "kvpmaster", "auth-pass", "secret123"})
	if strings.Contains(got, "secret123") {
		t.Fatalf("SENTINEL SET auth-pass 未脱敏: %q", got)
	}

	got = commandLine([]interface{}{"migrate", "10.0.0.1", "6379", "k", "0", "5000", "auth", "secret123"})
	if strings.Contains(got, "secret123") {
		t.Fatalf("MIGRATE AUTH 未脱敏: %q", got)
	}
}

// 验证普通命令完全不受影响。
func TestRedactArgsKeepsNormalCommands(t *testing.T) {
	got := commandLine([]interface{}{"get", "user:1"})
	if got != "get user:1" {
		t.Fatalf("普通命令被改写: %q", got)
	}
	if commandLine(nil) != "" {
		t.Fatal("空参数应返回空字符串")
	}
}
