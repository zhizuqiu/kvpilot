package services

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

const (
	snapshotPollInterval = 300 * time.Millisecond
	snapshotTimeout      = 20 * time.Second
)

// snapshotNodeResult 记录单节点的 RDB 保存结果。
type snapshotNodeResult struct {
	Addr       string `json:"addr"`
	Triggered  bool   `json:"triggered"`
	InProgress bool   `json:"inProgress"`
	LastSave   int64  `json:"lastSave"`
	Status     string `json:"status"`
	ElapsedMS  int64  `json:"elapsedMs"`
	Msg        string `json:"msg,omitempty"`
}

// parseSaveState 从 INFO persistence 中取出后台保存是否仍在进行，以及上次保存状态。
func parseSaveState(info string) (inProgress bool, status string) {
	status = "unknown"
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "rdb_bgsave_in_progress:") {
			inProgress = strings.TrimPrefix(line, "rdb_bgsave_in_progress:") == "1"
		}
		if strings.HasPrefix(line, "rdb_last_bgsave_status:") {
			status = strings.TrimPrefix(line, "rdb_last_bgsave_status:")
		}
	}
	return inProgress, status
}

// CleanSlowLogs 清空慢日志：集群覆盖所有主节点，其它模式覆盖当前节点。
func (b *browserService) CleanSlowLogs(server string) (resp types.JSResp) {
	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	ctx := item.ctx
	var mutex sync.Mutex
	reset := make([]string, 0, 4)
	failed := make([]string, 0, 4)

	if cluster, ok := item.client.(*redis.ClusterClient); ok {
		_ = cluster.ForEachMaster(ctx, func(ctx context.Context, cli *redis.Client) error {
			addr := cli.Options().Addr
			if err := cli.SlowLogReset(ctx).Err(); err != nil {
				mutex.Lock()
				failed = append(failed, addr)
				mutex.Unlock()
				return nil
			}
			mutex.Lock()
			reset = append(reset, addr)
			mutex.Unlock()
			return nil
		})
	} else {
		addr := ""
		if client, ok := item.client.(*redis.Client); ok {
			addr = client.Options().Addr
		}
		if err := item.client.SlowLogReset(ctx).Err(); err != nil {
			resp.Msg = err.Error()
			return
		}
		reset = append(reset, addr)
	}

	if len(reset) == 0 {
		resp.Msg = "clean slow log fail"
		return
	}
	sort.Strings(reset)
	sort.Strings(failed)

	resp.Success = true
	resp.Data = map[string]any{
		"reset":  len(reset),
		"failed": failed,
		"nodes":  reset,
	}
	return
}

// SaveSnapshot 逐节点触发 BGSAVE，并等待后台任务结束。
// 触发与完成分开返回，避免把"已开始保存"当成立即成功。
func (b *browserService) SaveSnapshot(server string) (resp types.JSResp) {
	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	targets := b.configMasterTargets(item.ctx, item.client, server)
	if len(targets) == 0 {
		resp.Msg = "no reachable node"
		return
	}
	defer closeConfigTargets(targets)

	results := make([]snapshotNodeResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, b.snapshotNode(item.ctx, target))
	}

	saved := 0
	for _, result := range results {
		if result.Status == "ok" {
			saved++
		}
	}

	resp.Success = saved > 0
	if saved == 0 {
		resp.Msg = "save snapshot fail"
	}
	resp.Data = map[string]any{
		"nodes":  results,
		"saved":  saved,
		"failed": len(results) - saved,
	}
	return
}

// snapshotNode 对单个节点执行 BGSAVE 并轮询到结束或超时。
func (b *browserService) snapshotNode(ctx context.Context, target configTarget) snapshotNodeResult {
	started := time.Now()
	result := snapshotNodeResult{Addr: target.Addr, Status: "unknown"}

	client, ok := target.Client.(*redis.Client)
	if !ok {
		result.Msg = "unsupported client"
		return result
	}

	if err := client.BgSave(ctx).Err(); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "in progress") {
			result.Status = "error"
			result.Msg = err.Error()
			result.LastSave = mustLastSave(ctx, client)
			result.ElapsedMS = time.Since(started).Milliseconds()
			return result
		}
		// 已有任务在跑时继续等待该任务结束
		result.Msg = err.Error()
	} else {
		result.Triggered = true
	}

	deadline := time.Now().Add(snapshotTimeout)
	for {
		inProgress, status := parseSaveState(mustInfoPersistence(ctx, client))
		result.InProgress = inProgress
		result.Status = status
		result.LastSave = mustLastSave(ctx, client)
		result.ElapsedMS = time.Since(started).Milliseconds()

		if !inProgress {
			return result
		}
		if time.Now().After(deadline) {
			result.Msg = "wait for bgsave timeout"
			return result
		}
		select {
		case <-ctx.Done():
			result.Msg = ctx.Err().Error()
			return result
		case <-time.After(snapshotPollInterval):
		}
	}
}

func mustInfoPersistence(ctx context.Context, client *redis.Client) string {
	info, err := client.Info(ctx, "persistence").Result()
	if err != nil {
		return ""
	}
	return info
}

func mustLastSave(ctx context.Context, client *redis.Client) int64 {
	value, err := client.LastSave(ctx).Result()
	if err != nil {
		return 0
	}
	return value
}
