package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newIntegrationClient 按环境变量创建真实 Redis 客户端：
// KVPILOT_TEST_REDIS_CLUSTER=1 时创建集群客户端，用于覆盖 ForEachShard 分支。
// 未设置地址时调用方必须跳过测试，保证默认 go test ./... 不访问网络。
func newIntegrationClient(t *testing.T) (redis.UniversalClient, bool) {
	t.Helper()

	addr := os.Getenv("KVPILOT_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_ADDR，跳过真实 Redis 集成测试")
	}
	password := os.Getenv("KVPILOT_TEST_REDIS_PASSWORD")

	if os.Getenv("KVPILOT_TEST_REDIS_CLUSTER") == "1" {
		client := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        []string{addr},
			Password:     password,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
		// 关闭动作也交给 Cleanup：它比测试内注册的用户清理更早注册，
		// 因此会最后执行，保证清理用户时连接仍然可用。
		t.Cleanup(func() { _ = client.Close() })
		return client, true
	}
	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    password,
		DialTimeout: 3 * time.Second,
		ReadTimeout: 3 * time.Second,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client, false
}

// 验证 aclAllShards 在单机上只返回一个结果，在集群上覆盖全部分片且按地址升序，
// 这是保证 ACL 列表"刷新结果稳定"的前提。
func TestIntegrationAclAllShards(t *testing.T) {
	client, isCluster := newIntegrationClient(t)

	ctx := context.Background()
	results := (&browserService{}).aclAllShards(ctx, client, "ACL", "WHOAMI")

	if len(results) == 0 {
		t.Fatal("应至少返回一个分片结果")
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("ACL WHOAMI 失败: %v", r.Err)
		}
		if toText(r.Value) == "" {
			t.Fatalf("ACL WHOAMI 返回空用户名: %+v", r)
		}
	}
	if !isCluster && len(results) != 1 {
		t.Fatalf("单机应只有 1 个分片, got %d", len(results))
	}
	for i := 1; i < len(results); i++ {
		if results[i-1].Addr > results[i].Addr {
			t.Fatalf("分片结果未按地址升序: %s > %s", results[i-1].Addr, results[i].Addr)
		}
	}
}

// 验证 ACL LIST 的真实返回能被解析并聚合：
// 集群下同名用户必须出现在每一个节点，Nodes 与 TotalNodes 相等。
func TestIntegrationAclListAggregation(t *testing.T) {
	client, isCluster := newIntegrationClient(t)

	results := (&browserService{}).aclAllShards(context.Background(), client, "ACL", "LIST")
	users, err := aggregateAclUsers(results)
	if err != nil {
		t.Fatalf("ACL LIST 聚合失败: %v", err)
	}

	var defaultUser *aclUserInfo
	for i := range users {
		if users[i].Name == "default" {
			defaultUser = &users[i]
			break
		}
	}
	if defaultUser == nil {
		t.Fatalf("真实实例应存在 default 用户, got %+v", users)
	}
	if defaultUser.Nodes != defaultUser.TotalNodes {
		t.Fatalf("default 应存在于所有节点, got %d/%d", defaultUser.Nodes, defaultUser.TotalNodes)
	}
	if isCluster && defaultUser.TotalNodes < 2 {
		t.Fatalf("集群模式应覆盖多个分片, got %d", defaultUser.TotalNodes)
	}
	if defaultUser.Commands == "" {
		t.Fatal("default 用户应带有命令规则")
	}
}

// 验证 ACL 写命令在集群模式下确实扇出到每一个分片：
// 创建的用户必须在所有分片上可读，删除后也必须从所有分片消失。
// 使用唯一命名的临时用户，并在 Cleanup 中兜底删除，避免残留影响手工验证。
func TestIntegrationAclDoFanOut(t *testing.T) {
	client, isCluster := newIntegrationClient(t)

	ctx := context.Background()
	svc := &browserService{}

	user := fmt.Sprintf("kvpilot_probe_%d", time.Now().UnixNano())
	password := fmt.Sprintf(">KvpProbe-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.aclDo(cleanupCtx, client, "ACL", "DELUSER", user); err != nil {
			t.Errorf("清理临时用户 %s 失败: %v", user, err)
		}
	})

	// 先 reset 再逐条给规则，确保不受同名历史用户影响
	rules := []interface{}{
		"ACL", "SETUSER", user,
		"reset", "on", password,
		"~kvpilot:probe:*", "&kvpilot:probe:*",
		"-@all", "+get", "+set",
	}
	if err := svc.aclDo(ctx, client, rules...); err != nil {
		t.Fatalf("ACL SETUSER 失败: %v", err)
	}

	results := svc.aclAllShards(ctx, client, "ACL", "GETUSER", user)
	if len(results) == 0 {
		t.Fatal("应至少返回一个分片结果")
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("分片 %s 上未创建用户: %v", r.Addr, r.Err)
		}
		if r.Value == nil {
			t.Fatalf("分片 %s 返回空用户详情", r.Addr)
		}
	}
	if isCluster && len(results) < 2 {
		t.Fatalf("集群模式应覆盖多个分片, got %d", len(results))
	}

	if err := svc.aclDo(ctx, client, "ACL", "DELUSER", user); err != nil {
		t.Fatalf("ACL DELUSER 失败: %v", err)
	}
	// 删除同样要扇出，任何一个分片上残留都算失败
	for _, r := range svc.aclAllShards(ctx, client, "ACL", "GETUSER", user) {
		if r.Err == nil && r.Value != nil {
			t.Fatalf("分片 %s 上用户未被删除: %+v", r.Addr, r.Value)
		}
	}
}

// 验证非法规则会被服务端拒绝，且错误能透传给调用方，
// 避免前端以为保存成功但实际未生效。
func TestIntegrationAclDoRejectsInvalidRule(t *testing.T) {
	client, _ := newIntegrationClient(t)

	ctx := context.Background()
	svc := &browserService{}

	user := fmt.Sprintf("kvpilot_probe_bad_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.aclDo(cleanupCtx, client, "ACL", "DELUSER", user); err != nil {
			t.Errorf("清理临时用户 %s 失败: %v", user, err)
		}
	})

	if err := svc.aclDo(ctx, client, "ACL", "SETUSER", user, "reset", "not-a-valid-rule"); err == nil {
		t.Fatal("非法 ACL 规则应返回错误")
	}
}

// 验证 ACL DRYRUN 的真实判定：允许的命令返回 OK，越权命令返回无权限，
// 且后者必须被识别为"判定结果"而不是执行失败。
func TestIntegrationAclDryRun(t *testing.T) {
	client, _ := newIntegrationClient(t)

	ctx := context.Background()
	svc := &browserService{}

	user := fmt.Sprintf("kvpilot_probe_dry_%d", time.Now().UnixNano())
	password := fmt.Sprintf(">KvpProbe-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.aclDo(cleanupCtx, client, "ACL", "DELUSER", user); err != nil {
			t.Errorf("清理临时用户 %s 失败: %v", user, err)
		}
	})

	rules := []interface{}{
		"ACL", "SETUSER", user,
		"reset", "on", password,
		"~kvpilot:probe:*", "&kvpilot:probe:*",
		"-@all", "+get",
	}
	if err := svc.aclDo(ctx, client, rules...); err != nil {
		t.Fatalf("ACL SETUSER 失败: %v", err)
	}

	allowed, err := svc.aclFirst(ctx, client, "acl", "dryrun", user, "get", "kvpilot:probe:a")
	if err != nil {
		t.Fatalf("允许的命令应返回 OK, got %v", err)
	}
	if !strings.EqualFold(toText(allowed), "OK") {
		t.Fatalf("允许的命令应返回 OK, got %q", toText(allowed))
	}

	denied, err := svc.aclFirst(ctx, client, "acl", "dryrun", user, "del", "kvpilot:probe:a")
	// Redis 7.2 把拒绝信息作为普通字符串返回（不是错误回复），
	// 因此这里同时接受 err 与文本两种形态，与 AclDryRun 的判定逻辑保持一致。
	msg := toText(denied)
	if err != nil {
		msg = err.Error()
	}
	if !strings.Contains(msg, "no permissions") {
		t.Fatalf("越权命令应提示无权限, got err=%v text=%q", err, toText(denied))
	}
}

// 验证 ACL CAT 与 ACL GETUSER 的真实返回能被规整成前端可用的结构。
func TestIntegrationAclCategoriesAndUser(t *testing.T) {
	client, _ := newIntegrationClient(t)

	ctx := context.Background()
	svc := &browserService{}

	raw, err := svc.aclFirst(ctx, client, "ACL", "CAT")
	if err != nil {
		t.Fatalf("ACL CAT 失败: %v", err)
	}
	list, _ := raw.([]interface{})
	if len(list) == 0 {
		t.Fatalf("ACL CAT 应返回命令类别, got %T %+v", raw, raw)
	}

	raw, err = svc.aclFirst(ctx, client, "acl", "getuser", "default")
	if err != nil {
		t.Fatalf("ACL GETUSER 失败: %v", err)
	}
	detail := toStringMap(raw)
	if detail["commands"] == nil {
		t.Fatalf("default 用户详情缺少 commands 字段: %+v", detail)
	}
	if len(toSlice(detail["flags"])) == 0 {
		t.Fatalf("default 用户详情缺少 flags 字段: %+v", detail)
	}
}
