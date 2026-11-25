package services

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// waitSlotMigration 轮询迁移任务直到结束。
func waitSlotMigration(t *testing.T, svc *browserService, name string, timeout time.Duration) slotProgress {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		resp := svc.GetSlotMigration(name)
		if !resp.Success {
			t.Fatalf("GetSlotMigration 失败: %s", resp.Msg)
		}
		progress := resp.Data.(slotProgress)
		if progress.State != "running" && progress.State != "paused" {
			return progress
		}
		if time.Now().After(deadline) {
			t.Fatalf("迁移在 %v 内未结束: %+v", timeout, progress)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// 验证集群槽位概览与单槽迁移：带键的槽迁移后归属变更、键与 TTL 保留。
func TestIntegrationSlotMigrationWithKeys(t *testing.T) {
	svc, name := newEndpointService(t)

	overviewResp := svc.SlotOverview(name)
	if !overviewResp.Success {
		t.Fatalf("SlotOverview 失败: %s", overviewResp.Msg)
	}
	overview := overviewResp.Data.(slotOverviewData)
	if overview.Mode != "cluster" {
		t.Fatalf("模式 = %q, want cluster", overview.Mode)
	}
	if len(overview.Masters) < 2 {
		t.Fatalf("至少需要两个主节点, got %d", len(overview.Masters))
	}

	// 用 hash tag 生成同一槽位下的键，再确认槽位所属的主节点
	ctx := context.Background()
	client := svc.connMap[name].client
	tag := fmt.Sprintf("kvpilot-slottest-%d", time.Now().UnixNano())
	firstKey := "{" + tag + "}:0"
	slot, err := client.Do(ctx, "CLUSTER", "KEYSLOT", firstKey).Int()
	if err != nil {
		t.Fatalf("计算槽位失败: %v", err)
	}

	var source, target slotMaster
	for _, master := range overview.Masters {
		if nodeOwnsSlot(master.SlotRanges, slot) {
			source = master
		} else if target.ID == "" {
			target = master
		}
	}
	if source.ID == "" || target.ID == "" {
		t.Fatalf("未找到槽位 %d 的源/目标节点", slot)
	}

	// 写入 10 个带 TTL 的键
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("{%s}:%d", tag, i)
		if err := client.Set(ctx, key, "value", 10*time.Minute).Err(); err != nil {
			t.Fatalf("写入测试键失败: %v", err)
		}
	}
	t.Cleanup(func() {
		for i := 0; i < 10; i++ {
			_ = client.Del(context.Background(), fmt.Sprintf("{%s}:%d", tag, i)).Err()
		}
	})

	previewResp := svc.PreviewSlotMigration(name, source.ID, target.ID, slot)
	if !previewResp.Success {
		t.Fatalf("PreviewSlotMigration 失败: %s", previewResp.Msg)
	}
	preview := previewResp.Data.(slotPreviewData)
	if !preview.CanStart {
		t.Fatalf("预检未通过: %+v", preview.Warnings)
	}
	if preview.KeyCount < 10 {
		t.Fatalf("槽位键数 = %d, want >= 10", preview.KeyCount)
	}

	startResp := svc.StartSlotMigration(name, source.ID, target.ID, slot, slotMigrationOptions{
		BatchKeys: 5, BatchBytes: 1 << 20, MigrateTimeoutMs: 5000, BatchIntervalMs: 0,
	})
	if !startResp.Success {
		t.Fatalf("StartSlotMigration 失败: %s", startResp.Msg)
	}

	progress := waitSlotMigration(t, svc, name, 60*time.Second)
	if progress.State != "completed" {
		t.Fatalf("迁移状态 = %s, error=%s, recovery=%v", progress.State, progress.Error, progress.Recovery)
	}
	if progress.MovedKeys < 10 {
		t.Fatalf("迁移键数 = %d, want >= 10", progress.MovedKeys)
	}

	// 迁移后键仍可访问且 TTL 保留
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("{%s}:%d", tag, i)
		value, err := client.Get(ctx, key).Result()
		if err != nil || value != "value" {
			t.Fatalf("迁移后键 %s 不可用: value=%q err=%v", key, value, err)
		}
		if ttl, err := client.TTL(ctx, key).Result(); err != nil || ttl <= 0 {
			t.Fatalf("迁移后键 %s 的 TTL 异常: %v %v", key, ttl, err)
		}
	}

	// 槽位归属已切换到目标节点
	afterResp := svc.SlotOverview(name)
	if !afterResp.Success {
		t.Fatalf("迁移后 SlotOverview 失败: %s", afterResp.Msg)
	}
	for _, master := range afterResp.Data.(slotOverviewData).Masters {
		owns := nodeOwnsSlot(master.SlotRanges, slot)
		if master.ID == target.ID && !owns {
			t.Fatalf("槽位 %d 应归属目标节点 %s", slot, target.Addr)
		}
		if master.ID == source.ID && owns {
			t.Fatalf("槽位 %d 仍归属源节点 %s", slot, source.Addr)
		}
	}
}

// 验证未搬键时可以取消，取消后状态为 canceled 且槽位不变。
func TestIntegrationSlotMigrationCancel(t *testing.T) {
	svc, name := newEndpointService(t)

	overviewResp := svc.SlotOverview(name)
	if !overviewResp.Success {
		t.Fatalf("SlotOverview 失败: %s", overviewResp.Msg)
	}
	overview := overviewResp.Data.(slotOverviewData)
	if len(overview.Masters) < 2 {
		t.Skip("至少需要两个主节点")
	}

	// 选一个源节点的空槽位
	source := overview.Masters[0]
	target := overview.Masters[1]
	slot := source.SlotRanges[0].Start

	// 暂停后立即取消：此时尚未搬键
	if resp := svc.StartSlotMigration(name, source.ID, target.ID, slot, slotMigrationOptions{}); !resp.Success {
		t.Fatalf("StartSlotMigration 失败: %s", resp.Msg)
	}
	_ = svc.PauseSlotMigration(name)
	cancel := svc.CancelSlotMigration(name)
	if !cancel.Success {
		t.Fatalf("取消失败: %s", cancel.Msg)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		progress := svc.GetSlotMigration(name).Data.(slotProgress)
		if progress.State == "canceled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("取消后状态 = %s, want canceled", progress.State)
		}
		time.Sleep(100 * time.Millisecond)
	}

	after := svc.SlotOverview(name).Data.(slotOverviewData)
	for _, master := range after.Masters {
		if master.ID == source.ID && !nodeOwnsSlot(master.SlotRanges, slot) {
			t.Fatalf("取消后槽位 %d 不应离开源节点", slot)
		}
	}
}
