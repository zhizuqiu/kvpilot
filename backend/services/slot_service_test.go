package services

import (
	"testing"
)

// 验证槽位字段解析：区间、单槽、迁移标记与对端节点 ID。
func TestParseSlotText(t *testing.T) {
	ranges, markers := parseSlotText("0-5460 6000 [100->-abc123] [200-<-def456]")
	if len(ranges) != 2 {
		t.Fatalf("区间数量 = %d, want 2 (%+v)", len(ranges), ranges)
	}
	if ranges[0] != (slotRange{Start: 0, End: 5460}) || ranges[1] != (slotRange{Start: 6000, End: 6000}) {
		t.Fatalf("区间解析错误: %+v", ranges)
	}
	if len(markers) != 2 {
		t.Fatalf("标记数量 = %d, want 2 (%+v)", len(markers), markers)
	}
	if markers[0].Slot != 100 || markers[0].Peer != "abc123" || !markers[0].Outward {
		t.Fatalf("迁出标记错误: %+v", markers[0])
	}
	if markers[1].Slot != 200 || markers[1].Peer != "def456" || markers[1].Outward {
		t.Fatalf("迁入标记错误: %+v", markers[1])
	}
}

// 验证槽位归属判断与区间计数。
func TestNodeOwnsSlot(t *testing.T) {
	ranges := []slotRange{{Start: 0, End: 100}, {Start: 200, End: 300}}
	if !nodeOwnsSlot(ranges, 0) || !nodeOwnsSlot(ranges, 100) || !nodeOwnsSlot(ranges, 250) {
		t.Fatal("边界内槽位应归属该节点")
	}
	if nodeOwnsSlot(ranges, 150) || nodeOwnsSlot(ranges, 301) {
		t.Fatal("区间外槽位不应归属该节点")
	}
	if slotRangeCount(ranges) != 202 {
		t.Fatalf("槽位计数 = %d, want 202", slotRangeCount(ranges))
	}
}

// 验证限速参数会收敛到安全范围。
func TestClampSlotOptions(t *testing.T) {
	options := clampSlotOptions(slotMigrationOptions{})
	if options.BatchKeys != slotDefaultBatchKeys || options.BatchBytes != slotDefaultBatchBytes {
		t.Fatalf("默认值错误: %+v", options)
	}
	if options.MigrateTimeoutMs != slotDefaultTimeoutMs {
		t.Fatalf("默认超时错误: %+v", options)
	}
	// 0 表示不限速，前端默认值 10ms 由界面提供
	if options.BatchIntervalMs != 0 {
		t.Fatalf("间隔默认应为 0: %+v", options)
	}

	options = clampSlotOptions(slotMigrationOptions{BatchKeys: 99999, BatchBytes: -1, MigrateTimeoutMs: 999999, BatchIntervalMs: 99999})
	if options.BatchKeys != slotMaxBatchKeys || options.BatchBytes != slotDefaultBatchBytes {
		t.Fatalf("上限收敛错误: %+v", options)
	}
	if options.MigrateTimeoutMs != 60000 || options.BatchIntervalMs != 1000 {
		t.Fatalf("超时/间隔上限错误: %+v", options)
	}
}

// 验证按字节裁剪批次：至少保留一个键，不超上限。
func TestTrimBatchByBytes(t *testing.T) {
	sizes := []int64{100, 100, 100, 100}
	if got := trimBatchByBytes(sizes, 250); got != 2 {
		t.Fatalf("裁剪数量 = %d, want 2", got)
	}
	if got := trimBatchByBytes(sizes, 50); got != 1 {
		t.Fatalf("超大键应至少保留一个, got %d", got)
	}
	if got := trimBatchByBytes(sizes, 1000); got != 4 {
		t.Fatalf("未超上限应全部保留, got %d", got)
	}
	if got := trimBatchByBytes(nil, 100); got != 0 {
		t.Fatalf("空输入应返回 0, got %d", got)
	}
}
