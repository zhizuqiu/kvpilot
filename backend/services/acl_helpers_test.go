package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 验证删除用户的守卫顺序与文案：空用户名 → 当前连接用户 → default。
// 这三条决定"误点删除"会不会把当前会话或内置用户删掉，必须逐条锁死。
func TestValidateAclDelete(t *testing.T) {
	cases := []struct {
		name   string
		whoami string
		user   string
		want   string
	}{
		{
			name:   "空用户名直接拒绝",
			whoami: "default",
			user:   "",
			want:   "user is required",
		},
		{
			name:   "禁止删除当前连接使用的用户",
			whoami: "default",
			user:   "default",
			want:   "cannot delete the user used by the current connection",
		},
		{
			name:   "禁止删除 default 用户",
			whoami: "app",
			user:   "default",
			want:   "the default user cannot be deleted",
		},
		{
			name:   "普通用户允许删除",
			whoami: "app",
			user:   "tempuser",
			want:   "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAclDelete(c.whoami, c.user)
			if c.want == "" {
				if err != nil {
					t.Fatalf("应允许删除, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("应拒绝删除, want %q", c.want)
			}
			if err.Error() != c.want {
				t.Fatalf("错误文案 = %q, want %q", err.Error(), c.want)
			}
		})
	}
}

// 验证 ACL GETUSER 的返回规整：resp2 扁平数组要正确映射成详情结构。
func TestBuildAclUserDetail(t *testing.T) {
	resp2 := []interface{}{
		"flags", []interface{}{"on", "nopass"},
		"passwords", []interface{}{},
		"commands", "+@all",
		"keys", "~*",
		"channels", "&*",
		"selectors", []interface{}{map[interface{}]interface{}{"commands": "-@all"}},
	}

	detail, ok := buildAclUserDetail(resp2, 2, 3)
	if !ok {
		t.Fatal("合法返回应解析成功")
	}
	if len(detail.Flags) != 2 || detail.Flags[0] != "on" {
		t.Fatalf("flags 解析不正确: %v", detail.Flags)
	}
	if detail.Passwords != 0 || len(detail.PasswordHashes) != 0 {
		t.Fatalf("密码应为空, got %d", detail.Passwords)
	}
	if detail.Commands != "+@all" || detail.Keys != "~*" || detail.Channels != "&*" {
		t.Fatalf("规则字段解析不正确: %+v", detail)
	}
	if detail.Selectors != 1 {
		t.Fatalf("selector 数量 = %d, want 1", detail.Selectors)
	}
	if detail.Nodes != 2 || detail.TotalNodes != 3 {
		t.Fatalf("节点信息 = %d/%d, want 2/3", detail.Nodes, detail.TotalNodes)
	}
}

// 验证返回为空时判定为"用户不存在"：nil 与空数组都不能当成合法详情。
func TestBuildAclUserDetailEmpty(t *testing.T) {
	if _, ok := buildAclUserDetail(nil, 0, 3); ok {
		t.Fatal("nil 应判定为不存在")
	}
	if _, ok := buildAclUserDetail([]interface{}{}, 0, 3); ok {
		t.Fatal("空数组应判定为不存在")
	}
}

// 验证 ACL DRYRUN 的判定规整：允许、文本拒绝、错误拒绝、真实错误四种情况。
func TestNormalizeDryRunResult(t *testing.T) {
	allowed, message, failure := normalizeDryRunResult("OK", nil)
	if !allowed || message != "OK" || failure != "" {
		t.Fatalf("OK 应判定为允许, got %v %q %q", allowed, message, failure)
	}

	// Redis 7.2 实测：拒绝信息是普通字符串而不是错误
	allowed, message, failure = normalizeDryRunResult(
		"User app has no permissions to run the 'del' command", nil)
	if allowed || failure != "" || !strings.Contains(message, "no permissions") {
		t.Fatalf("文本拒绝应判定为无权限, got %v %q %q", allowed, message, failure)
	}

	// 部分版本把拒绝作为错误返回，同样属于判定结果
	allowed, message, failure = normalizeDryRunResult(nil, errors.New(
		"user app has no permissions to run the 'del' command"))
	if allowed || failure != "" || !strings.Contains(message, "no permissions") {
		t.Fatalf("错误拒绝应判定为无权限, got %v %q %q", allowed, message, failure)
	}

	// 命令不存在等真实错误必须整体失败
	allowed, _, failure = normalizeDryRunResult(nil, errors.New("ERR Command 'nosuchcmd' not found"))
	if allowed || failure == "" {
		t.Fatalf("真实错误应返回 failure, got %v %q", allowed, failure)
	}
}

// 验证 ACL LIST 中无法解析的行被跳过，不影响其余用户。
func TestAggregateAclUsersSkipsInvalidLine(t *testing.T) {
	results := []aclShardResult{
		{Addr: "a", Value: []interface{}{"notauser x", "user app on +get"}},
	}

	users, err := aggregateAclUsers(results)
	if err != nil {
		t.Fatalf("不应返回错误, got %v", err)
	}
	if len(users) != 1 || users[0].Name != "app" {
		t.Fatalf("应只保留 app, got %+v", users)
	}
}

// 验证 aclFirst 与 aclRead 在集群完全没有可用分片时返回 redis.Nil，
// 而不是返回 nil 数据让调用方误判成功。
func TestAclFirstAndReadWithoutShards(t *testing.T) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        []string{"127.0.0.1:1"},
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		MaxRetries:   -1,
		MaxRedirects: 1,
	})
	t.Cleanup(func() { _ = client.Close() })

	svc := &browserService{ctx: context.Background(), connMap: map[string]*connectionItem{}}
	ctx := context.Background()

	if _, err := svc.aclFirst(ctx, client, "ACL", "WHOAMI"); err == nil {
		t.Fatal("无分片时 aclFirst 应返回错误")
	}
	if _, err := svc.aclRead(ctx, client, "ACL", "WHOAMI"); err == nil {
		t.Fatal("无分片时 aclRead 应返回错误")
	}
}

// 验证 ACL LOG 条数钳制：非正数与超过 200 一律回退到 20，
// 防止前端传入异常值时把服务端日志刷爆。
func TestClampAclLogCount(t *testing.T) {
	cases := map[int]int{
		-5:   20,
		0:    20,
		1:    1,
		20:   20,
		200:  200,
		201:  20,
		1000: 20,
	}
	for in, want := range cases {
		if got := clampAclLogCount(in); got != want {
			t.Fatalf("clampAclLogCount(%d) = %d, want %d", in, got, want)
		}
	}
}

// 验证 ACL SETUSER 参数拼装：固定前缀 + 丢弃空字符串规则，
// 否则空规则会被当成非法修饰符导致整条命令失败。
func TestBuildAclSetUserArgs(t *testing.T) {
	got := buildAclSetUserArgs("app", []string{"reset", "on", "#abc", "", "  ", "~cache:*"})
	want := []interface{}{"acl", "setuser", "app", "reset", "on", "#abc", "~cache:*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildAclSetUserArgs = %v, want %v", got, want)
	}

	// 全部规则为空时仍要保留固定前缀，交由调用方校验
	got = buildAclSetUserArgs("app", []string{"", ""})
	want = []interface{}{"acl", "setuser", "app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("空规则时应只返回前缀, got %v", got)
	}
}
