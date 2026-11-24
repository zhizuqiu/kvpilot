package services

import (
	"os"
	"testing"
)

// 验证清空慢日志：先制造一条慢日志，清空后列表为空。
func TestIntegrationCleanSlowLogs(t *testing.T) {
	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") != "1" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_SEED_BIGKEY，跳过会修改配置的维护测试")
	}
	svc, name := newEndpointService(t)
	client := svc.connMap[name].client

	// 临时把所有命令记为慢日志，制造可清理的数据
	if err := client.Do(t.Context(), "CONFIG", "SET", "slowlog-log-slower-than", "0").Err(); err != nil {
		t.Fatalf("设置慢日志阈值失败: %v", err)
	}
	if err := client.Do(t.Context(), "SLOWLOG", "RESET").Err(); err != nil {
		t.Fatalf("准备阶段清空慢日志失败: %v", err)
	}
	_ = client.Do(t.Context(), "PING").Err()
	// 先把阈值恢复，避免后续读取本身也被计入慢日志
	if err := client.Do(t.Context(), "CONFIG", "SET", "slowlog-log-slower-than", "10000").Err(); err != nil {
		t.Fatalf("恢复慢日志阈值失败: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Do(t.Context(), "SLOWLOG", "RESET").Err()
	})

	before := svc.GetSlowLogs(name, 10)
	if !before.Success {
		t.Fatalf("读取慢日志失败: %s", before.Msg)
	}
	if len(before.Data.(map[string]any)["list"].([]slowLogItem)) == 0 {
		t.Fatal("准备阶段应产生至少一条慢日志")
	}

	resp := svc.CleanSlowLogs(name)
	if !resp.Success {
		t.Fatalf("CleanSlowLogs 失败: %s", resp.Msg)
	}
	if reset, _ := resp.Data.(map[string]any)["reset"].(int); reset < 1 {
		t.Fatalf("清理节点数 = %d, want >= 1", reset)
	}

	after := svc.GetSlowLogs(name, 10)
	if !after.Success {
		t.Fatalf("清空后读取慢日志失败: %s", after.Msg)
	}
	if logs := after.Data.(map[string]any)["list"].([]slowLogItem); len(logs) != 0 {
		t.Fatalf("清空后仍有 %d 条慢日志", len(logs))
	}
}

// 验证手动落盘：触发 BGSAVE 后等待完成，返回状态与最后保存时间。
func TestIntegrationSaveSnapshot(t *testing.T) {
	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") != "1" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_SEED_BIGKEY，跳过会写 RDB 的维护测试")
	}
	svc, name := newEndpointService(t)

	resp := svc.SaveSnapshot(name)
	if !resp.Success {
		t.Fatalf("SaveSnapshot 失败: %s", resp.Msg)
	}
	data := resp.Data.(map[string]any)
	if saved, _ := data["saved"].(int); saved < 1 {
		t.Fatalf("成功保存节点数 = %d, want >= 1", saved)
	}
	nodes, _ := data["nodes"].([]snapshotNodeResult)
	if len(nodes) == 0 {
		t.Fatal("应返回逐节点结果")
	}
	for _, node := range nodes {
		if node.Addr == "" {
			t.Fatalf("节点结果缺少地址: %+v", node)
		}
		if node.Status != "ok" {
			t.Fatalf("节点 %s 保存状态 = %q, msg=%q", node.Addr, node.Status, node.Msg)
		}
		if node.LastSave <= 0 {
			t.Fatalf("节点 %s 最后保存时间异常: %d", node.Addr, node.LastSave)
		}
	}
}
