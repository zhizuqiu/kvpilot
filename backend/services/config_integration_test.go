package services

import (
	"os"
	"strconv"
	"testing"
)

// 验证真实实例上的配置读取：参数齐全、敏感参数脱敏、节点列表完整。
func TestIntegrationConfigOverview(t *testing.T) {
	svc, name := newEndpointService(t)

	resp := svc.ConfigOverview(name)
	if !resp.Success {
		t.Fatalf("ConfigOverview 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(configData)
	if !ok {
		t.Fatalf("返回类型应为 configData, got %T", resp.Data)
	}
	if len(data.Params) < 100 {
		t.Fatalf("参数数量异常: %d", len(data.Params))
	}
	if len(data.Nodes) == 0 {
		t.Fatal("节点列表不应为空")
	}

	wantMerged := os.Getenv("KVPILOT_TEST_REDIS_CLUSTER") == "1"
	if data.Merged != wantMerged {
		t.Fatalf("merged = %v, want %v", data.Merged, wantMerged)
	}
	if wantMerged && len(data.Nodes) < 2 {
		t.Fatalf("集群应覆盖多个节点, got %d", len(data.Nodes))
	}

	var foundPass, foundMaxmemory bool
	for _, p := range data.Params {
		switch p.Key {
		case "requirepass":
			foundPass = true
			if !p.Sensitive || p.Value != "***" {
				t.Fatalf("requirepass 应脱敏, got sensitive=%v value=%q", p.Sensitive, p.Value)
			}
			if len(p.Values) != 0 {
				t.Fatalf("敏感参数不应返回逐节点取值: %+v", p.Values)
			}
		case "maxmemory":
			foundMaxmemory = true
			if p.Category != "memory" {
				t.Fatalf("maxmemory 分类错误: %q", p.Category)
			}
			if p.Nodes != len(data.Nodes) {
				t.Fatalf("maxmemory 节点数 = %d, want %d", p.Nodes, len(data.Nodes))
			}
		}
	}
	if !foundPass || !foundMaxmemory {
		t.Fatalf("缺少关键参数: requirepass=%v maxmemory=%v", foundPass, foundMaxmemory)
	}
}

// 验证写入：所有节点都生效，且自动尝试 CONFIG REWRITE 并给出正确状态。
func TestIntegrationConfigSetAndRewrite(t *testing.T) {
	svc, name := newEndpointService(t)

	before := svc.ConfigOverview(name)
	if !before.Success {
		t.Fatalf("读取初始配置失败: %s", before.Msg)
	}
	var original string
	for _, p := range before.Data.(configData).Params {
		if p.Key == "maxmemory" {
			original = p.Value
		}
	}
	if original == "" {
		t.Fatal("未读到 maxmemory 原值")
	}
	t.Cleanup(func() {
		svc.ConfigSet(name, "maxmemory", original)
	})

	resp := svc.ConfigSet(name, "maxmemory", "77mb")
	if !resp.Success {
		t.Fatalf("ConfigSet 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("返回类型不正确: %T", resp.Data)
	}
	applied, _ := data["applied"].(int)
	total, _ := data["nodes"].(int)
	if applied != total {
		t.Fatalf("应写入全部节点: applied=%d nodes=%d results=%+v", applied, total, data["results"])
	}

	// 每个节点的 rewrite 结果必须是三种状态之一
	results, _ := data["results"].([]configNodeResult)
	if len(results) != total {
		t.Fatalf("结果数量 = %d, want %d", len(results), total)
	}
	for _, r := range results {
		switch r.Rewrite {
		case "persisted", "memory-only", "failed":
		default:
			t.Fatalf("未知的 rewrite 状态: %+v", r)
		}
		if !r.Set {
			t.Fatalf("节点 %s 写入失败: %s", r.Addr, r.Msg)
		}
	}

	// 写入后各节点取值应一致
	after := svc.ConfigOverview(name)
	if !after.Success {
		t.Fatalf("写入后读取失败: %s", after.Msg)
	}
	for _, p := range after.Data.(configData).Params {
		if p.Key != "maxmemory" {
			continue
		}
		if !p.Consistent {
			t.Fatalf("写入后各节点应一致, got %+v", p.Values)
		}
		if p.Value == original {
			t.Fatalf("maxmemory 未变化, 仍为 %q", p.Value)
		}
		if _, err := strconv.ParseInt(p.Value, 10, 64); err != nil {
			t.Fatalf("maxmemory 应返回字节数, got %q", p.Value)
		}
	}
}
