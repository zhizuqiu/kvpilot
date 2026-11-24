package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 验证真实实例上的会话列表：能看到客户端连接、节点列表完整，健康环境没有未响应节点。
func TestIntegrationListSessions(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.ListSessions(name)
	if !resp.Success {
		t.Fatalf("ListSessions 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(sessionData)
	if !ok {
		t.Fatalf("返回类型应为 sessionData, got %T", resp.Data)
	}
	if len(data.Nodes) == 0 {
		t.Fatal("节点列表不应为空")
	}
	if len(data.Sessions) == 0 {
		t.Fatal("应至少包含当前客户端连接")
	}
	if data.Total != len(data.Sessions) {
		t.Fatalf("总数 = %d, 明细 = %d", data.Total, len(data.Sessions))
	}
	if len(data.Unavailable) != 0 {
		t.Fatalf("健康环境不应有未响应节点: %v", data.Unavailable)
	}

	// 每条会话都必须能追溯到具体节点和地址，否则前端无法定位来源
	for _, session := range data.Sessions {
		if session.Node == "" || session.Addr == "" || session.ID <= 0 {
			t.Fatalf("会话缺少节点、地址或 ID: %+v", session)
		}
	}
}

// 验证关闭会话：打开一条命名连接，确认它出现在列表里，再按 ID 关闭。
func TestIntegrationKillSession(t *testing.T) {
	svc, name := newEndpointService(t)

	addr := os.Getenv("KVPILOT_TEST_REDIS_ADDR")
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
	t.Cleanup(func() { _ = client.Close() })

	probeName := fmt.Sprintf("kvpilot_probe_%d", time.Now().UnixNano())
	ctx := context.Background()
	// 用 Do 设置连接名称：Client 与 ClusterClient 都支持，接口层无需类型分支
	if err := client.Do(ctx, "CLIENT", "SETNAME", probeName).Err(); err != nil {
		t.Fatalf("设置连接名称失败: %v", err)
	}

	findProbe := func() *sessionItem {
		resp := svc.ListSessions(name)
		if !resp.Success {
			t.Fatalf("ListSessions 失败: %s", resp.Msg)
		}
		for _, session := range resp.Data.(sessionData).Sessions {
			if session.Name == probeName {
				return &session
			}
		}
		return nil
	}

	probe := findProbe()
	if probe == nil {
		t.Fatal("新建的连接未出现在会话列表里")
	}

	resp := svc.KillSession(name, probe.Node, probe.ID, probe.Addr)
	if !resp.Success {
		t.Fatalf("KillSession 失败: %s", resp.Msg)
	}
	if killed, _ := resp.Data.(map[string]any)["killed"].(int64); killed < 1 {
		t.Fatalf("关闭数量 = %d, want >= 1", killed)
	}
}

// 验证受保护判断和参数校验：缺少 ID 与地址时直接拒绝，不会发起网络请求。
func TestKillSessionValidation(t *testing.T) {
	svc := &browserService{}
	if resp := svc.KillSession("any", "node", 0, " "); resp.Success ||
		resp.Msg != "session id or addr is required" {
		t.Fatalf("缺少定位参数应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}
}
