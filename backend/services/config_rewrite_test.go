package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newConfigServiceForAddr 针对指定地址构造 browserService，供 rewrite 场景测试使用。
func newConfigServiceForAddr(t *testing.T, addr string) (*browserService, string) {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
		MaxRetries:  1,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	svc := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	name := "kvpilot_config_rewrite"
	svc.connMap[name] = &connectionItem{client: client, ctx: ctx, db: 0}
	return svc, name
}

// 验证节点有可写配置文件时，CONFIG SET 后自动落盘并标记 persisted。
func TestIntegrationConfigRewritePersisted(t *testing.T) {
	addr := os.Getenv("KVPILOT_TEST_REDIS_CONFIGFILE_ADDR")
	if addr == "" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_CONFIGFILE_ADDR，跳过 rewritable config 测试")
	}
	svc, name := newConfigServiceForAddr(t, addr)
	t.Cleanup(func() { svc.ConfigSet(name, "maxmemory", "0") })

	resp := svc.ConfigSet(name, "maxmemory", "13mb")
	if !resp.Success {
		t.Fatalf("ConfigSet 失败: %s", resp.Msg)
	}
	results, _ := resp.Data.(map[string]any)["results"].([]configNodeResult)
	if len(results) != 1 {
		t.Fatalf("结果数量 = %d, want 1", len(results))
	}
	if results[0].Rewrite != "persisted" {
		t.Fatalf("有可写配置文件时应落盘, got rewrite=%q msg=%q", results[0].Rewrite, results[0].Msg)
	}
}

// 验证配置文件只读时仍写入内存，但 rewrite 标记为 failed 并带原因。
func TestIntegrationConfigRewriteFailed(t *testing.T) {
	addr := os.Getenv("KVPILOT_TEST_REDIS_READONLYCONF_ADDR")
	if addr == "" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_READONLYCONF_ADDR，跳过 read-only config 测试")
	}
	svc, name := newConfigServiceForAddr(t, addr)
	t.Cleanup(func() { svc.ConfigSet(name, "maxmemory", "0") })

	resp := svc.ConfigSet(name, "maxmemory", "13mb")
	if !resp.Success {
		t.Fatalf("内存写入应成功: %s", resp.Msg)
	}
	results, _ := resp.Data.(map[string]any)["results"].([]configNodeResult)
	if len(results) != 1 {
		t.Fatalf("结果数量 = %d, want 1", len(results))
	}
	if results[0].Rewrite != "failed" {
		t.Fatalf("只读配置应标记 failed, got rewrite=%q", results[0].Rewrite)
	}
	if results[0].Msg == "" {
		t.Fatal("failed 状态应带错误原因")
	}
	if results[0].Set != true {
		t.Fatal("CONFIG SET 本身应成功")
	}
}
