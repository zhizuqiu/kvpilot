package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newConfigDeadService 返回一个指向不可达端口的 browserService，用于覆盖错误分支。
func newConfigDeadService(t *testing.T) (*browserService, string) {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	svc := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	svc.connMap["dead"] = &connectionItem{client: client, ctx: ctx, db: 0}
	return svc, "dead"
}

// newConfigDeadClusterService 返回一个指向不可达集群的 browserService。
func newConfigDeadClusterService(t *testing.T) (*browserService, string) {
	t.Helper()

	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        []string{"127.0.0.1:1"},
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		MaxRetries:   -1,
		MaxRedirects: 1,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	svc := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	svc.connMap["dead-cluster"] = &connectionItem{client: client, ctx: ctx, db: 0}
	return svc, "dead-cluster"
}

// 验证连接名不存在时两个接口都返回错误而不是 panic。
func TestConfigUnknownConnection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &browserService{ctx: context.Background(), connMap: map[string]*connectionItem{}}

	if resp := svc.ConfigOverview("kvpilot_missing"); resp.Success ||
		!strings.Contains(resp.Msg, "no match connection") {
		t.Fatalf("ConfigOverview 应返回连接不存在, got success=%v msg=%q", resp.Success, resp.Msg)
	}
	if resp := svc.ConfigSet("kvpilot_missing", "maxmemory", "1mb"); resp.Success ||
		!strings.Contains(resp.Msg, "no match connection") {
		t.Fatalf("ConfigSet 应返回连接不存在, got success=%v msg=%q", resp.Success, resp.Msg)
	}
}

// 验证节点不可达时读取返回明确错误；拓扑探测失败会退化为单节点视图。
func TestConfigOverviewUnreachableNode(t *testing.T) {
	svc, name := newConfigDeadService(t)

	resp := svc.ConfigOverview(name)
	if resp.Success {
		t.Fatal("节点不可达时不应返回成功")
	}
	if resp.Msg != "get config fail" {
		t.Fatalf("错误文案 = %q, want get config fail", resp.Msg)
	}
}

// 验证节点不可达时写入返回失败并带逐节点原因。
func TestConfigSetUnreachableNode(t *testing.T) {
	svc, name := newConfigDeadService(t)

	resp := svc.ConfigSet(name, "maxmemory", "1mb")
	if resp.Success {
		t.Fatal("节点不可达时不应返回成功")
	}
	if resp.Msg != "apply config fail" {
		t.Fatalf("错误文案 = %q, want apply config fail", resp.Msg)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("失败时也应返回逐节点结果, got %T", resp.Data)
	}
	results, _ := data["results"].([]configNodeResult)
	if len(results) != 1 || results[0].Set {
		t.Fatalf("应记录一条失败结果, got %+v", results)
	}
	if results[0].Msg == "" {
		t.Fatal("失败结果应带错误信息")
	}
}

// 验证集群不可用时返回"无可用节点"，覆盖 CLUSTER NODES 失败分支。
func TestConfigClusterUnreachable(t *testing.T) {
	svc, name := newConfigDeadClusterService(t)

	if resp := svc.ConfigOverview(name); resp.Success || resp.Msg != "no reachable node" {
		t.Fatalf("集群不可用应返回 no reachable node, got success=%v msg=%q", resp.Success, resp.Msg)
	}
	if resp := svc.ConfigSet(name, "maxmemory", "1mb"); resp.Success || resp.Msg != "no reachable node" {
		t.Fatalf("集群不可用应返回 no reachable node, got success=%v msg=%q", resp.Success, resp.Msg)
	}
}
