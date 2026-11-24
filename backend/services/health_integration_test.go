package services

import (
	"os"
	"testing"
)

// 验证真实实例的巡检：所有规则都有明确级别，统计与明细一致。
func TestIntegrationCheckHealth(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.CheckHealth(name)
	if !resp.Success {
		t.Fatalf("CheckHealth 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(healthData)
	if !ok {
		t.Fatalf("返回类型应为 healthData, got %T", resp.Data)
	}
	if len(data.Nodes) == 0 {
		t.Fatal("节点列表不应为空")
	}
	if len(data.Unavailable) != 0 {
		t.Fatalf("健康环境不应有未响应节点: %v", data.Unavailable)
	}
	if os.Getenv("KVPILOT_TEST_REDIS_CLUSTER") == "1" && data.Mode != "cluster" {
		t.Fatalf("集群连接的模式 = %q, want cluster", data.Mode)
	}

	total := 0
	for severity, count := range data.Counts {
		if severity != "ok" && severity != "warn" && severity != "error" {
			t.Fatalf("未知级别: %s", severity)
		}
		total += count
	}
	if total != len(data.Items) {
		t.Fatalf("统计总数 = %d, 明细 = %d", total, len(data.Items))
	}
	for _, item := range data.Items {
		if item.Node == "" || item.Rule == "" {
			t.Fatalf("巡检项缺少节点或规则: %+v", item)
		}
		if item.Severity != "ok" && item.Severity != "warn" && item.Severity != "error" {
			t.Fatalf("巡检项级别非法: %+v", item)
		}
	}
}
