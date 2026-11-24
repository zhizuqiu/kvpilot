package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// waitBigKeyScan 轮询扫描任务直到结束，返回最终进度。
func waitBigKeyScan(t *testing.T, svc *browserService, name string, timeout time.Duration) bigKeyProgress {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		resp := svc.GetBigKeyScan(name)
		if !resp.Success {
			t.Fatalf("GetBigKeyScan 失败: %s", resp.Msg)
		}
		progress := resp.Data.(bigKeyProgress)
		if progress.State != "running" {
			return progress
		}
		if time.Now().After(deadline) {
			svc.CancelBigKeyScan(name)
			t.Fatalf("扫描在 %v 内未结束, 当前进度: %+v", timeout, progress)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// seedBigKeys 在目标实例写入探针键：一个大字符串和若干小键，返回清理函数。
func seedBigKeys(t *testing.T, client redis.UniversalClient, count int) (string, func()) {
	t.Helper()
	ctx := context.Background()
	prefix := fmt.Sprintf("kvpilot_bigkey_probe_%d_", time.Now().UnixNano())
	bigKey := prefix + "big"
	if err := client.Set(ctx, bigKey, strings.Repeat("x", 2*1024*1024), 0).Err(); err != nil {
		t.Fatalf("写入大 key 失败: %v", err)
	}
	pipe := client.Pipeline()
	for i := 0; i < count; i++ {
		pipe.Set(ctx, fmt.Sprintf("%s%d", prefix, i), "v", 0)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("批量写入探针键失败: %v", err)
	}

	cleanup := func() {
		ctx := context.Background()
		iter := client.Scan(ctx, 0, prefix+"*", 500).Iterator()
		for iter.Next(ctx) {
			_ = client.Del(ctx, iter.Val()).Err()
		}
		_ = client.Del(ctx, bigKey).Err()
	}
	return bigKey, cleanup
}

// 验证采样扫描能找到超过阈值的大 key，并正确结束任务。
func TestIntegrationBigKeyScan(t *testing.T) {
	svc, name := newEndpointService(t)
	client := svc.connMap[name].client

	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") == "1" {
		bigKey, cleanup := seedBigKeys(t, client, 100)
		t.Cleanup(cleanup)

		resp := svc.StartBigKeyScan(name, 500, 1, false)
		if !resp.Success {
			t.Fatalf("StartBigKeyScan 失败: %s", resp.Msg)
		}
		progress := waitBigKeyScan(t, svc, name, 60*time.Second)
		if progress.State != "done" {
			t.Fatalf("扫描状态 = %s, error=%s", progress.State, progress.Error)
		}
		if progress.Scanned == 0 {
			t.Fatal("未扫描任何 key")
		}

		var found bool
		for _, item := range progress.Results {
			if item.Key == bigKey {
				found = true
				if item.Size < 1024*1024 {
					t.Fatalf("大 key 大小异常: %d", item.Size)
				}
				if item.Type != "string" || item.Node == "" {
					t.Fatalf("结果字段不完整: %+v", item)
				}
			}
		}
		if !found {
			t.Fatalf("未发现探针大 key, 结果数量 %d", len(progress.Results))
		}
		return
	}

	// 未写入探针时只验证任务能正常跑完（只读，不写线上实例）
	if totalKeys(t, client) == 0 {
		t.Log("目标实例没有 key，跳过扫描断言")
		return
	}

	resp := svc.StartBigKeyScan(name, 200, 1, false)
	if !resp.Success {
		t.Fatalf("StartBigKeyScan 失败: %s", resp.Msg)
	}
	progress := waitBigKeyScan(t, svc, name, 60*time.Second)
	if progress.State != "done" {
		t.Fatalf("扫描状态 = %s, error=%s", progress.State, progress.Error)
	}
	if progress.Scanned == 0 {
		t.Fatal("未扫描任何 key")
	}
}

// 验证全量扫描可以被取消：取消后状态为 canceled 且保留已扫描进度。
func TestIntegrationBigKeyScanCancel(t *testing.T) {
	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") != "1" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_SEED_BIGKEY，跳过取消测试以免影响共享实例")
	}
	svc, name := newEndpointService(t)
	client := svc.connMap[name].client
	_, cleanup := seedBigKeys(t, client, 20000)
	t.Cleanup(cleanup)

	resp := svc.StartBigKeyScan(name, 0, 1, true)
	if !resp.Success {
		t.Fatalf("StartBigKeyScan 失败: %s", resp.Msg)
	}
	time.Sleep(200 * time.Millisecond)

	cancel := svc.CancelBigKeyScan(name)
	if !cancel.Success {
		t.Fatalf("CancelBigKeyScan 失败: %s", cancel.Msg)
	}
	progress := waitBigKeyScan(t, svc, name, 30*time.Second)
	if progress.State != "canceled" {
		t.Fatalf("取消后状态 = %s, want canceled", progress.State)
	}
	if progress.Scanned == 0 {
		t.Fatal("取消前应已扫描部分 key")
	}
}

// 验证同一连接不允许并发启动两个扫描任务。
func TestBigKeyScanAlreadyRunning(t *testing.T) {
	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") != "1" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_SEED_BIGKEY，跳过多任务校验")
	}
	svc, name := newEndpointService(t)
	client := svc.connMap[name].client
	_, cleanup := seedBigKeys(t, client, 5000)
	t.Cleanup(cleanup)

	if resp := svc.StartBigKeyScan(name, 0, 1, true); !resp.Success {
		t.Fatalf("首次启动失败: %s", resp.Msg)
	}
	t.Cleanup(func() { svc.CancelBigKeyScan(name) })

	if resp := svc.StartBigKeyScan(name, 0, 1, true); resp.Success {
		t.Fatal("运行中的任务不应被第二个任务覆盖")
	}
}

// totalKeys 汇总实例所有数据库的键数量，用于判断是否有可扫描数据。
func totalKeys(t *testing.T, client redis.UniversalClient) int64 {
	t.Helper()
	info, err := client.Info(context.Background(), "keyspace").Result()
	if err != nil {
		t.Fatalf("读取 keyspace 失败: %v", err)
	}
	var total int64
	for _, count := range parseKeyspaceSizes(info) {
		total += count
	}
	return total
}
