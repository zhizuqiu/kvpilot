package services

import (
	"context"
	"testing"
	"time"
)

// 验证 INFO keyspace 解析出每个 db 的键数量，忽略空行。
func TestParseKeyspaceSizes(t *testing.T) {
	text := "# Keyspace\r\n" +
		"db0:keys=120,expires=10,avg_ttl=0\r\n" +
		"db2:keys=7,expires=0,avg_ttl=0\r\n" +
		"\r\n"
	sizes := parseKeyspaceSizes(text)
	if len(sizes) != 2 {
		t.Fatalf("解析数量 = %d, want 2 (%v)", len(sizes), sizes)
	}
	if sizes[0] != 120 || sizes[2] != 7 {
		t.Fatalf("解析结果错误: %v", sizes)
	}
}

// 验证结果集只保留最大的若干个 key，较小的命中会被替换掉。
func TestInsertBigKeyResultKeepsLargest(t *testing.T) {
	list := []bigKeyItem{}
	limit := 3
	for _, size := range []int64{5, 30, 20, 10, 40} {
		list = insertBigKeyResult(list, bigKeyItem{Key: "k", Size: size}, limit)
	}
	if len(list) != limit {
		t.Fatalf("保留数量 = %d, want %d", len(list), limit)
	}
	sum := int64(0)
	for _, item := range list {
		sum += item.Size
	}
	if sum != 90 {
		t.Fatalf("应保留 30+20+40, 实际总和 = %d (%+v)", sum, list)
	}
}

// 验证进度百分比：运行中按扫描比例，结束时固定 100，空目标不除零。
func TestBigKeyPercent(t *testing.T) {
	if got := bigKeyPercent(50, 200, false); got != 25 {
		t.Fatalf("运行中百分比 = %d, want 25", got)
	}
	if got := bigKeyPercent(50, 200, true); got != 100 {
		t.Fatalf("完成时百分比 = %d, want 100", got)
	}
	if got := bigKeyPercent(0, 0, false); got != 0 {
		t.Fatalf("空目标百分比 = %d, want 0", got)
	}
	if got := bigKeyPercent(300, 200, false); got != 100 {
		t.Fatalf("超出目标时应封顶在 100, got %d", got)
	}
}

// 验证采样预算按剩余库数量均分，至少保证每个库扫描 1 个 key。
func TestBigKeyBudget(t *testing.T) {
	cases := []struct{ budget, dbs, want int }{
		{1000, 1, 1000},
		{1000, 2, 500},
		{1000, 3, 333},
		{2, 3, 1},
		{0, 5, 0},
		{1, 5, 1},
	}
	for _, c := range cases {
		if got := bigKeyBudget(c.budget, c.dbs); got != c.want {
			t.Fatalf("budget(%d,%d) = %d, want %d", c.budget, c.dbs, got, c.want)
		}
	}
}

// 验证任务结束后耗时冻结，不会随着查看时间继续增长。
func TestBigKeyTaskElapsedFreezes(t *testing.T) {
	task := newBigKeyScanTask(context.Background(), func() {}, bigKeyOptions{SampleSize: 10, ThresholdMB: 1})
	task.started = time.Now().Add(-10 * time.Second)
	task.finishedAt = task.started.Add(2 * time.Second)

	progress := task.snapshot()
	if progress.ElapsedMS != 2000 {
		t.Fatalf("结束后耗时 = %dms, want 2000ms", progress.ElapsedMS)
	}
}

// 验证采样总量取采样数与节点实际键数的较小值。
func TestBigKeyNodeTotal(t *testing.T) {
	cases := []struct {
		sample int
		keys   int64
		want   int64
	}{
		{1000, 1216, 1000},
		{5000, 417, 417},
		{1000, 0, 0},
	}
	for _, c := range cases {
		if got := bigKeyNodeTotal(c.sample, c.keys); got != c.want {
			t.Fatalf("bigKeyNodeTotal(%d,%d) = %d, want %d", c.sample, c.keys, got, c.want)
		}
	}
}
