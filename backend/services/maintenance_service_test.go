package services

import (
	"testing"
)

// 验证后台保存状态解析：进行中与上次结果分别取值。
func TestParseSaveState(t *testing.T) {
	info := "# Persistence\r\n" +
		"loading:0\r\n" +
		"rdb_changes_since_last_save:12\r\n" +
		"rdb_bgsave_in_progress:1\r\n" +
		"rdb_last_save_time:1700000000\r\n" +
		"rdb_last_bgsave_status:ok\r\n"

	inProgress, status := parseSaveState(info)
	if !inProgress {
		t.Fatal("应识别出后台保存进行中")
	}
	if status != "ok" {
		t.Fatalf("状态 = %q, want ok", status)
	}

	inProgress, status = parseSaveState("rdb_bgsave_in_progress:0\r\nrdb_last_bgsave_status:err\r\n")
	if inProgress {
		t.Fatal("已结束时不应标记为进行中")
	}
	if status != "err" {
		t.Fatalf("状态 = %q, want err", status)
	}

	// 缺少字段时给出 unknown，避免误判为成功
	inProgress, status = parseSaveState("")
	if inProgress || status != "unknown" {
		t.Fatalf("空输入应返回 unknown, got inProgress=%v status=%q", inProgress, status)
	}
}
