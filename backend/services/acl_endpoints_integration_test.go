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

// newEndpointService 构造一个直连真实 Redis 的 browserService：
// 直接把客户端放进 connMap，getRedisClient(name, -1) 会命中缓存，
// 因此不需要初始化全局 Connection() 单例，也不会读写用户本地的 connections.yaml。
func newEndpointService(t *testing.T) (*browserService, string) {
	t.Helper()

	addr := os.Getenv("KVPILOT_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_ADDR，跳过 ACL 接口级集成测试")
	}
	password := os.Getenv("KVPILOT_TEST_REDIS_PASSWORD")

	var client redis.UniversalClient
	if os.Getenv("KVPILOT_TEST_REDIS_CLUSTER") == "1" {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        []string{addr},
			Password:     password,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:        addr,
			Password:    password,
			DialTimeout: 3 * time.Second,
			ReadTimeout: 3 * time.Second,
		})
	}
	// 先注册关闭，业务清理后注册，保证清理时连接仍可用
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	svc := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	name := "kvpilot_endpoint_test"
	svc.connMap[name] = &connectionItem{client: client, ctx: ctx, db: 0}
	return svc, name
}

// createProbeUser 通过接口创建一个唯一命名的临时用户，并注册清理。
func createProbeUser(t *testing.T, svc *browserService, name, prefix string, rules []string) string {
	t.Helper()

	user := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	all := append([]string{"reset", "on"}, rules...)
	resp := svc.AclSetUser(name, user, all)
	if !resp.Success {
		t.Fatalf("创建临时用户失败: %s", resp.Msg)
	}
	t.Cleanup(func() {
		// 用例自己删过之后这里会得到 "user not found"，属于预期，不再刷日志
		if resp := svc.AclDeleteUser(name, user); !resp.Success && resp.Msg != "user not found" {
			t.Logf("清理临时用户 %s 返回: %s", user, resp.Msg)
		}
	})
	return user
}

// 验证用户列表接口的返回契约：whoami、用户集合与节点数必须齐全，
// 前端依赖 users[].nodes/totalNodes 渲染"节点一致性"标记。
func TestIntegrationAclOverviewEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.AclOverview(name)
	if !resp.Success {
		t.Fatalf("AclOverview 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("返回类型应为 map, got %T", resp.Data)
	}
	if data["whoami"] == "" {
		t.Fatal("whoami 不应为空")
	}
	nodes, _ := data["nodes"].(int)
	if nodes < 1 {
		t.Fatalf("nodes 至少为 1, got %d", nodes)
	}

	users, _ := data["users"].([]aclUserInfo)
	if len(users) == 0 {
		t.Fatal("用户列表不应为空")
	}

	var foundDefault bool
	for _, u := range users {
		if u.TotalNodes != nodes {
			t.Fatalf("用户 %s 的 totalNodes = %d, 应等于 nodes %d", u.Name, u.TotalNodes, nodes)
		}
		if u.Nodes < 1 || u.Nodes > u.TotalNodes {
			t.Fatalf("用户 %s 的 nodes 越界: %d/%d", u.Name, u.Nodes, u.TotalNodes)
		}
		if u.Name == "default" {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatal("用户列表应包含 default")
	}
}

// 验证用户详情接口：密码个数与哈希列表长度一致，
// 集群下 nodes/totalNodes 必须相等（default 存在于所有节点）。
func TestIntegrationAclUserEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.AclUser(name, "default")
	if !resp.Success {
		t.Fatalf("AclUser 失败: %s", resp.Msg)
	}
	detail, ok := resp.Data.(aclUserDetail)
	if !ok {
		t.Fatalf("返回类型应为 aclUserDetail, got %T", resp.Data)
	}
	if detail.Passwords != len(detail.PasswordHashes) {
		t.Fatalf("密码个数 %d 与哈希数 %d 不一致", detail.Passwords, len(detail.PasswordHashes))
	}
	if detail.TotalNodes < 1 || detail.Nodes != detail.TotalNodes {
		t.Fatalf("default 应存在于所有节点, got %d/%d", detail.Nodes, detail.TotalNodes)
	}
	if len(detail.Flags) == 0 {
		t.Fatal("default 应带有 flags")
	}
}

// 验证查询不存在的用户时返回失败而不是空详情，
// 前端依赖 msg 判断是否需要提示用户。
func TestIntegrationAclUserNotFound(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.AclUser(name, "kvpilot_no_such_user_xyz")
	if resp.Success {
		t.Fatal("不存在的用户不应返回成功")
	}
	if resp.Msg != "user not found" {
		t.Fatalf("错误文案 = %q, want user not found", resp.Msg)
	}
}

// 验证命令类别接口返回非空字符串数组，供规则编辑器生成 +@category 选项。
func TestIntegrationAclCategoriesEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.AclCategories(name)
	if !resp.Success {
		t.Fatalf("AclCategories 失败: %s", resp.Msg)
	}
	categories, ok := resp.Data.([]string)
	if !ok {
		t.Fatalf("返回类型应为 []string, got %T", resp.Data)
	}
	if len(categories) == 0 {
		t.Fatal("类别列表不应为空")
	}
	for _, c := range categories {
		if strings.TrimSpace(c) == "" {
			t.Fatal("类别列表不应包含空字符串")
		}
	}
}

// 验证保存规则接口：写入后详情立即反映新规则，且返回结构可直接回填表单。
func TestIntegrationAclSetUserEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	user := createProbeUser(t, svc, name, "kvpilot_ep_set", []string{"~kvpilot:probe:*", "+get"})

	resp := svc.AclUser(name, user)
	if !resp.Success {
		t.Fatalf("保存后读取详情失败: %s", resp.Msg)
	}
	detail := resp.Data.(aclUserDetail)
	var enabled bool
	for _, f := range detail.Flags {
		if f == "on" {
			enabled = true
		}
	}
	if !enabled {
		t.Fatalf("用户应为启用状态, got flags=%v", detail.Flags)
	}
	if !strings.Contains(detail.Commands, "+get") {
		t.Fatalf("命令规则应包含 +get, got %q", detail.Commands)
	}
	if !strings.Contains(detail.Keys, "kvpilot:probe:*") {
		t.Fatalf("key 规则应包含探测模式, got %q", detail.Keys)
	}
}

// 验证保存规则接口的参数校验：用户或规则为空时直接拒绝，不发出命令。
func TestIntegrationAclSetUserValidation(t *testing.T) {
	svc, name := newEndpointService(t)

	if resp := svc.AclSetUser(name, "", []string{"on"}); resp.Success || resp.Msg != "user and rules are required" {
		t.Fatalf("空用户名应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}
	user := createProbeUser(t, svc, name, "kvpilot_ep_empty", []string{"-@all"})
	if resp := svc.AclSetUser(name, user, nil); resp.Success || resp.Msg != "user and rules are required" {
		t.Fatalf("空规则应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}
}

// 验证删除用户接口：正常删除返回 deleted=1，删除当前连接用户被守卫拦下。
func TestIntegrationAclDeleteUserEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	overview := svc.AclOverview(name)
	whoami := overview.Data.(map[string]any)["whoami"].(string)

	user := createProbeUser(t, svc, name, "kvpilot_ep_del", []string{"-@all"})
	resp := svc.AclDeleteUser(name, user)
	if !resp.Success {
		t.Fatalf("删除用户失败: %s", resp.Msg)
	}
	data, _ := resp.Data.(map[string]any)
	if deleted, _ := data["deleted"].(int); deleted != 1 {
		t.Fatalf("deleted = %v, want 1", data["deleted"])
	}

	// 守卫：不允许删除当前连接使用的用户
	if resp := svc.AclDeleteUser(name, whoami); resp.Success ||
		resp.Msg != "cannot delete the user used by the current connection" {
		t.Fatalf("应拒绝删除当前连接用户, got success=%v msg=%q", resp.Success, resp.Msg)
	}
	// 守卫：用户名为空
	if resp := svc.AclDeleteUser(name, ""); resp.Success || resp.Msg != "user is required" {
		t.Fatalf("应拒绝空用户名, got success=%v msg=%q", resp.Success, resp.Msg)
	}
}

// 验证删除不存在的用户必须失败：DELUSER 返回 0 时不能谎报成功。
func TestIntegrationAclDeleteMissingUser(t *testing.T) {
	svc, name := newEndpointService(t)

	missing := fmt.Sprintf("kvpilot_absent_%d", time.Now().UnixNano())
	resp := svc.AclDeleteUser(name, missing)
	if resp.Success {
		t.Fatal("删除不存在的用户应返回失败")
	}
	if resp.Msg != "user not found" {
		t.Fatalf("错误文案 = %q, want user not found", resp.Msg)
	}
}

// 验证查询非法用户名时同样返回"用户不存在"，
// 覆盖"服务端返回空结构"而不是 nil 的分支。
func TestIntegrationAclUserWithInvalidName(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.AclUser(name, "bad name with spaces")
	if resp.Success {
		t.Fatal("非法用户名不应返回成功")
	}
	if resp.Msg != "user not found" {
		t.Fatalf("错误文案 = %q, want user not found", resp.Msg)
	}
}

// 验证只被授予 ACL WHOAMI 的受限用户无法删除其他用户：
// 删除命令必须因权限不足而失败，且错误透传给前端。
func TestIntegrationAclDeleteDeniedForLimitedUser(t *testing.T) {
	admin, name := newEndpointService(t)

	addr := os.Getenv("KVPILOT_TEST_REDIS_ADDR")
	user := fmt.Sprintf("kvpilot_limited_%d", time.Now().UnixNano())
	password := fmt.Sprintf("KvpLimited-%d", time.Now().UnixNano())

	resp := admin.AclSetUser(name, user, []string{
		"reset", "on", ">" + password, "~*", "&*", "-@all", "+acl|whoami",
	})
	if !resp.Success {
		t.Fatalf("创建受限用户失败: %s", resp.Msg)
	}
	t.Cleanup(func() {
		if r := admin.AclDeleteUser(name, user); !r.Success {
			t.Logf("清理受限用户失败: %s", r.Msg)
		}
	})

	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		Username:    user,
		Password:    password,
		DialTimeout: 3 * time.Second,
		ReadTimeout: 3 * time.Second,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	limited := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	limitedName := "kvpilot_limited_conn"
	limited.connMap[limitedName] = &connectionItem{client: client, ctx: ctx, db: 0}

	// WHOAMI 能通过，但 DELUSER 没有权限
	del := limited.AclDeleteUser(limitedName, "kvpilot_victim")
	if del.Success {
		t.Fatal("受限用户不应能删除其他用户")
	}
	if !strings.Contains(del.Msg, "NOPERM") && !strings.Contains(del.Msg, "permissions") {
		t.Fatalf("应返回权限错误, got %q", del.Msg)
	}
}

// 验证集群下所有分片都拒绝时，aclRead 返回错误而不是空数据。
// 仅在集群模式执行，单机模式的等价路径已由受限用户删除用例覆盖。
func TestIntegrationAclReadAllShardsDenied(t *testing.T) {
	if os.Getenv("KVPILOT_TEST_REDIS_CLUSTER") != "1" {
		t.Skip("仅集群模式覆盖多分片同时失败")
	}

	admin, name := newEndpointService(t)
	addr := os.Getenv("KVPILOT_TEST_REDIS_ADDR")
	user := fmt.Sprintf("kvpilot_denied_%d", time.Now().UnixNano())
	password := fmt.Sprintf("KvpDenied-%d", time.Now().UnixNano())

	// 只允许 CLUSTER 子命令（用于加载集群拓扑），不授予任何 ACL 权限：
	// 这样 ForEachShard 能正常遍历，而每个分片上的 ACL WHOAMI 都会被拒绝。
	if resp := admin.AclSetUser(name, user, []string{"reset", "on", ">" + password, "-@all", "+cluster"}); !resp.Success {
		t.Fatalf("创建无 ACL 权限用户失败: %s", resp.Msg)
	}
	t.Cleanup(func() { _ = admin.AclDeleteUser(name, user) })

	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        []string{addr},
		Username:     user,
		Password:     password,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	t.Cleanup(func() { _ = client.Close() })

	svc := &browserService{ctx: context.Background(), connMap: map[string]*connectionItem{}}
	if _, err := svc.aclRead(context.Background(), client, "ACL", "WHOAMI"); err == nil {
		t.Fatal("所有分片被拒绝时应返回错误")
	}
}

// 验证日志接口在异常条数下仍能成功返回，且条目字段完整。
func TestIntegrationAclLogEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	// count=0 会被钳制为默认值 20，这里验证接口不报错
	resp := svc.AclLog(name, 0)
	if !resp.Success {
		t.Fatalf("AclLog 失败: %s", resp.Msg)
	}
	entries, ok := resp.Data.([]map[string]string)
	if !ok {
		t.Fatalf("返回类型应为 []map[string]string, got %T", resp.Data)
	}
	if len(entries) > 200 {
		t.Fatalf("日志条数超出上限: %d", len(entries))
	}
	for _, e := range entries {
		if e["reason"] == "" {
			t.Fatalf("日志条目缺少 reason: %+v", e)
		}
	}
}

// 验证规则试算接口的判定映射：允许返回 allowed=true，
// 越权返回 allowed=false 且 message 带原因，而不是整体失败。
func TestIntegrationAclDryRunEndpoint(t *testing.T) {
	svc, name := newEndpointService(t)

	user := createProbeUser(t, svc, name, "kvpilot_ep_dry", []string{"~kvpilot:probe:*", "-@all", "+get"})

	resp := svc.AclDryRun(name, user, []string{"get", "kvpilot:probe:a"})
	if !resp.Success {
		t.Fatalf("AclDryRun 允许场景失败: %s", resp.Msg)
	}
	data := resp.Data.(map[string]any)
	if allowed, _ := data["allowed"].(bool); !allowed {
		t.Fatalf("get 应被允许, got %+v", data)
	}

	resp = svc.AclDryRun(name, user, []string{"del", "kvpilot:probe:a"})
	if !resp.Success {
		t.Fatalf("ACL 拒绝属于判定结果而非执行失败: %s", resp.Msg)
	}
	data = resp.Data.(map[string]any)
	if allowed, _ := data["allowed"].(bool); allowed {
		t.Fatalf("del 应被拒绝, got %+v", data)
	}
	if msg, _ := data["message"].(string); !strings.Contains(msg, "no permissions") {
		t.Fatalf("拒绝原因应包含 no permissions, got %q", msg)
	}

	if resp := svc.AclDryRun(name, user, nil); resp.Success || resp.Msg != "user and command are required" {
		t.Fatalf("空命令应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}

	// 不存在的命令属于真正的执行错误，必须整体失败而不是返回 allowed=false
	if resp := svc.AclDryRun(name, user, []string{"nosuchcommand", "x"}); resp.Success {
		t.Fatalf("未知命令应返回失败, got %+v", resp.Data)
	} else if !strings.Contains(resp.Msg, "not found") {
		t.Fatalf("错误信息应包含 not found, got %q", resp.Msg)
	}
}
