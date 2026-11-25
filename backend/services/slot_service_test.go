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

// 验证均衡计划：从超量节点向不足节点迁移，且不超过上限。
func TestBuildRebalancePlan(t *testing.T) {
	masters := []slotMaster{
		{ID: "a", Addr: "a:1", SlotCount: 6000, Keys: 6000, SlotRanges: []slotRange{{Start: 0, End: 5999}}},
		{ID: "b", Addr: "b:1", SlotCount: 5192, Keys: 100, SlotRanges: []slotRange{{Start: 6000, End: 11191}}},
		{ID: "c", Addr: "c:1", SlotCount: 5192, Keys: 100, SlotRanges: []slotRange{{Start: 11192, End: 16383}}},
	}
	moves, total := buildSlotCountPlan(masters, 1000, 0)
	if len(moves) == 0 || total == 0 {
		t.Fatalf("应生成迁移计划, moves=%+v total=%d", moves, total)
	}
	if total > 1000 {
		t.Fatalf("超过上限: %d", total)
	}
	seen := map[int]bool{}
	for _, move := range moves {
		if move.Source.ID != "a" {
			t.Fatalf("只应从超量节点迁出: %+v", move)
		}
		for _, slot := range move.Slots {
			if slot < 0 || slot > 5999 {
				t.Fatalf("槽位越界: %d", slot)
			}
			if seen[slot] {
				t.Fatalf("槽位重复: %d", slot)
			}
			seen[slot] = true
		}
	}

	// 已均衡时不应产生计划
	balanced := []slotMaster{
		{ID: "a", SlotCount: 5461}, {ID: "b", SlotCount: 5461}, {ID: "c", SlotCount: 5462},
	}
	if moves, total := buildSlotCountPlan(balanced, 100, 0); len(moves) != 0 || total != 0 {
		t.Fatalf("均衡状态不应产生计划: %+v", moves)
	}

	// 容忍区间内不再迁移
	if moves, total := buildSlotCountPlan(masters, 1000, 10); len(moves) != 0 || total != 0 {
		t.Fatalf("±10%% 容忍区间内不应生成计划: %+v", moves)
	}

	// 上限生效
	moves, total = buildSlotCountPlan(masters, 10, 0)
	if total != 10 {
		t.Fatalf("上限 10 时迁出 %d 个槽位", total)
	}
}

// 验证均衡计划跳过已经处于迁移中状态的槽位。
func TestBuildRebalancePlanSkipsMigrating(t *testing.T) {
	masters := []slotMaster{
		{ID: "a", SlotCount: 6000, SlotRanges: []slotRange{{Start: 0, End: 5999}}, Migrating: []int{5999, 5998}},
		{ID: "b", SlotCount: 5192, SlotRanges: []slotRange{{Start: 6000, End: 11191}}},
		{ID: "c", SlotCount: 5192, SlotRanges: []slotRange{{Start: 11192, End: 16383}}},
	}
	moves, _ := buildSlotCountPlan(masters, 1000, 0)
	for _, move := range moves {
		for _, slot := range move.Slots {
			if slot == 5999 || slot == 5998 {
				t.Fatalf("不应选择迁移中的槽位: %d", slot)
			}
		}
	}
}

// 验证按键数量的均衡计划：选择不会让目标节点超出容忍上限的槽位。
func TestBuildKeyPlan(t *testing.T) {
	masters := []slotMaster{
		{ID: "a", Addr: "a:1", Keys: 1000},
		{ID: "b", Addr: "b:1", Keys: 200},
		{ID: "c", Addr: "c:1", Keys: 300},
	}
	counts := map[string]map[int]int64{
		"a": {0: 600, 1: 300, 2: 100, 3: 0},
	}
	moves, total, movedKeys, skipped := buildKeyPlan(masters, counts, 3, 10)
	if total != 2 || movedKeys != 400 {
		t.Fatalf("迁移槽位 = %d, 键数 = %d, want 2 / 400", total, movedKeys)
	}
	if !skipped {
		t.Fatal("剩余 600 键的槽位会让目标超出上限，应标记 skipped")
	}
	if len(moves) != 2 {
		t.Fatalf("迁移项 = %d, want 2", len(moves))
	}
	if moves[0].Slots[0] != 1 || moves[0].ApproxKeys != 300 {
		t.Fatalf("应选择放得下的最大槽位: %+v", moves[0])
	}
	if moves[1].Slots[0] != 2 || moves[1].ApproxKeys != 100 {
		t.Fatalf("第二个迁移项错误: %+v", moves[1])
	}
	for _, move := range moves {
		for _, slot := range move.Slots {
			if slot == 3 {
				t.Fatal("不应迁移空槽")
			}
		}
	}
}

// 验证没有合适大小的槽位时不做迁移，避免来回震荡。
func TestBuildKeyPlanAvoidsOvershoot(t *testing.T) {
	masters := []slotMaster{
		{ID: "a", Addr: "a:1", Keys: 1000},
		{ID: "b", Addr: "b:1", Keys: 500},
		{ID: "c", Addr: "c:1", Keys: 0},
	}
	counts := map[string]map[int]int64{"a": {0: 600}}
	moves, total, _, skipped := buildKeyPlan(masters, counts, 100, 10)
	if total != 0 || len(moves) != 0 {
		t.Fatalf("迁 600 键会超出上限，不应生成计划: %+v", moves)
	}
	if !skipped {
		t.Fatal("应标记 skipped")
	}
}
