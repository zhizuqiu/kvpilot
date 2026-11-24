package services

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

const (
	bigKeyDefaultSampleSize  = 1000
	bigKeyMaxSampleSize      = 100000
	bigKeyDefaultThresholdMB = 10
	bigKeyScanBatch          = 200
	bigKeyResultLimit        = 500
	bigKeyRateLimitEvery     = 200
	bigKeyRateLimitSleep     = 10 * time.Millisecond
)

// bigKeyItem 是一个超过阈值的大 key。
type bigKeyItem struct {
	Key  string `json:"key"`
	Node string `json:"node"`
	DB   int    `json:"db"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// BigKeyItem 是导出接口使用的行类型，与内部结构保持一致。
type BigKeyItem = bigKeyItem

// bigKeyOptions 是一次扫描的配置。
type bigKeyOptions struct {
	SampleSize  int  `json:"sampleSize"`
	ThresholdMB int  `json:"thresholdMB"`
	Full        bool `json:"full"`
}

// bigKeyProgress 是扫描任务的当前状态。
type bigKeyProgress struct {
	State       string        `json:"state"` // idle / running / done / canceled / failed
	Scanned     int64         `json:"scanned"`
	Total       int64         `json:"total"`
	Percent     int           `json:"percent"`
	Found       int           `json:"found"`
	Results     []bigKeyItem  `json:"results"`
	Unavailable []string      `json:"unavailable,omitempty"`
	ElapsedMS   int64         `json:"elapsedMs"`
	Error       string        `json:"error,omitempty"`
	Options     bigKeyOptions `json:"options"`
}

// bigKeyScanTask 保存一次扫描的进度与结果，同一连接同时只允许一个任务。
type bigKeyScanTask struct {
	mutex       sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	options     bigKeyOptions
	threshold   int64
	state       string
	scanned     int64
	total       int64
	found       int
	results     []bigKeyItem
	unavailable []string
	errMsg      string
	started     time.Time
	finishedAt  time.Time
}

func newBigKeyScanTask(ctx context.Context, cancel context.CancelFunc, options bigKeyOptions) *bigKeyScanTask {
	return &bigKeyScanTask{
		ctx:       ctx,
		cancel:    cancel,
		options:   options,
		threshold: int64(options.ThresholdMB) * 1024 * 1024,
		state:     "running",
		started:   time.Now(),
		results:   make([]bigKeyItem, 0, 32),
	}
}

// normalizeBigKeyOptions 收敛用户输入：采样数有上限，阈值至少 1MB。
func normalizeBigKeyOptions(sampleSize, thresholdMB int, full bool) bigKeyOptions {
	if sampleSize <= 0 {
		sampleSize = bigKeyDefaultSampleSize
	}
	if sampleSize > bigKeyMaxSampleSize {
		sampleSize = bigKeyMaxSampleSize
	}
	if thresholdMB <= 0 {
		thresholdMB = bigKeyDefaultThresholdMB
	}
	return bigKeyOptions{SampleSize: sampleSize, ThresholdMB: thresholdMB, Full: full}
}

// parseKeyspaceSizes 解析 INFO keyspace，返回 db 序号到键数量的映射。
func parseKeyspaceSizes(info string) map[int]int64 {
	sizes := map[int]int64{}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "db") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		db, err := strconv.Atoi(strings.TrimPrefix(name, "db"))
		if err != nil {
			continue
		}
		fields := strings.Split(rest, ",")
		if len(fields) == 0 {
			continue
		}
		keys, ok := strings.CutPrefix(fields[0], "keys=")
		if !ok {
			continue
		}
		count, err := strconv.ParseInt(keys, 10, 64)
		if err != nil {
			continue
		}
		sizes[db] = count
	}
	return sizes
}

// bigKeyBudget 把采样预算均分给剩余数据库，至少给每个库留 1 个名额。
func bigKeyBudget(budget, remainingDBs int) int {
	if budget <= 0 || remainingDBs <= 0 {
		return 0
	}
	quota := budget / remainingDBs
	if quota < 1 {
		quota = 1
	}
	return quota
}

// insertBigKeyResult 只保留最大的若干个结果，避免全量模式把内存撑大。
func insertBigKeyResult(list []bigKeyItem, item bigKeyItem, limit int) []bigKeyItem {
	if limit <= 0 {
		return list
	}
	if len(list) < limit {
		return append(list, item)
	}
	minIndex := 0
	for i := range list {
		if list[i].Size < list[minIndex].Size {
			minIndex = i
		}
	}
	if item.Size > list[minIndex].Size {
		list[minIndex] = item
	}
	return list
}

// bigKeyNodeTotal 返回单节点预计扫描量：采样时不超过该节点实际键数。
func bigKeyNodeTotal(sampleSize int, nodeKeys int64) int64 {
	if nodeKeys <= 0 {
		return 0
	}
	if int64(sampleSize) < nodeKeys {
		return int64(sampleSize)
	}
	return nodeKeys
}

// bigKeyPercent 计算扫描进度：完成时固定 100，空目标返回 0，超出目标封顶。
func bigKeyPercent(scanned, total int64, finished bool) int {
	if finished {
		return 100
	}
	if total <= 0 {
		return 0
	}
	percent := int(scanned * 100 / total)
	if percent > 100 {
		percent = 100
	}
	return percent
}

func (t *bigKeyScanTask) addTotal(n int64) {
	t.mutex.Lock()
	t.total += n
	t.mutex.Unlock()
}

func (t *bigKeyScanTask) incScanned(n int64) {
	t.mutex.Lock()
	t.scanned += n
	t.mutex.Unlock()
}

func (t *bigKeyScanTask) addFound(item bigKeyItem) {
	t.mutex.Lock()
	t.found++
	t.results = insertBigKeyResult(t.results, item, bigKeyResultLimit)
	t.mutex.Unlock()
}

func (t *bigKeyScanTask) markUnavailable(addr string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for _, existing := range t.unavailable {
		if existing == addr {
			return
		}
	}
	t.unavailable = append(t.unavailable, addr)
}

func (t *bigKeyScanTask) finish(err error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.state != "running" {
		return
	}
	switch {
	case t.ctx.Err() != nil:
		t.state = "canceled"
	case err != nil:
		t.state = "failed"
		t.errMsg = err.Error()
	default:
		t.state = "done"
	}
	t.finishedAt = time.Now()
}

// snapshot 返回当前进度的副本，供接口读取。
func (t *bigKeyScanTask) snapshot() bigKeyProgress {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	results := append([]bigKeyItem(nil), t.results...)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Size != results[j].Size {
			return results[i].Size > results[j].Size
		}
		return results[i].Key < results[j].Key
	})

	unavailable := append([]string(nil), t.unavailable...)
	sort.Strings(unavailable)

	// 任务结束后耗时必须冻结，否则打开旧任务会看到一个不断增长的时长
	elapsed := time.Since(t.started)
	if !t.finishedAt.IsZero() {
		elapsed = t.finishedAt.Sub(t.started)
	}

	return bigKeyProgress{
		State:       t.state,
		Scanned:     t.scanned,
		Total:       t.total,
		Percent:     bigKeyPercent(t.scanned, t.total, t.state == "done"),
		Found:       t.found,
		Results:     results,
		Unavailable: unavailable,
		ElapsedMS:   elapsed.Milliseconds(),
		Error:       t.errMsg,
		Options:     t.options,
	}
}

// StartBigKeyScan 启动一次大 key 扫描；同一连接只允许一个任务在跑。
func (b *browserService) StartBigKeyScan(server string, sampleSize, thresholdMB int, full bool) (resp types.JSResp) {
	options := normalizeBigKeyOptions(sampleSize, thresholdMB, full)

	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	b.bigKeyMutex.Lock()
	if b.bigKeyTasks == nil {
		b.bigKeyTasks = map[string]*bigKeyScanTask{}
	}
	if existing, ok := b.bigKeyTasks[server]; ok {
		existing.mutex.Lock()
		running := existing.state == "running"
		existing.mutex.Unlock()
		if running {
			b.bigKeyMutex.Unlock()
			resp.Msg = "scan already running"
			return
		}
	}

	ctx, cancel := context.WithCancel(item.ctx)
	task := newBigKeyScanTask(ctx, cancel, options)
	b.bigKeyTasks[server] = task
	b.bigKeyMutex.Unlock()

	go b.runBigKeyScan(server, task, item)

	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// GetBigKeyScan 返回指定连接最近一次扫描的进度。
func (b *browserService) GetBigKeyScan(server string) (resp types.JSResp) {
	b.bigKeyMutex.Lock()
	task := b.bigKeyTasks[server]
	b.bigKeyMutex.Unlock()

	if task == nil {
		resp.Success = true
		resp.Data = bigKeyProgress{State: "idle", Results: []bigKeyItem{}}
		return
	}

	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// CancelBigKeyScan 取消正在运行的扫描；已结束的任务不受影响。
func (b *browserService) CancelBigKeyScan(server string) (resp types.JSResp) {
	b.bigKeyMutex.Lock()
	task := b.bigKeyTasks[server]
	b.bigKeyMutex.Unlock()

	if task == nil {
		resp.Msg = "no scan task"
		return
	}

	task.cancel()
	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// ExportBigKeys 把扫描结果写成 CSV；路径由调用方决定，仅桌面端使用。
func (b *browserService) ExportBigKeys(path string, rows []bigKeyItem) (resp types.JSResp) {
	path = strings.TrimSpace(path)
	if path == "" {
		resp.Msg = "path is required"
		return
	}

	table := make([][]string, 0, len(rows))
	for _, row := range rows {
		table = append(table, []string{row.Key, row.Type, strconv.FormatInt(row.Size, 10), row.Node, strconv.Itoa(row.DB)})
	}
	if err := writeCSV(path, []string{"key", "type", "size", "node", "db"}, table); err != nil {
		resp.Msg = err.Error()
		return
	}

	resp.Success = true
	resp.Data = map[string]any{"path": path, "count": len(rows)}
	return
}

// writeCSV 把表头与数据写入指定路径，自动创建缺失的父目录。
func writeCSV(path string, header []string, rows [][]string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	if len(header) > 0 {
		_ = writer.Write(header)
	}
	for _, row := range rows {
		_ = writer.Write(row)
	}
	writer.Flush()
	err = writer.Error()
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

// runBigKeyScan 依次扫描每个数据节点：先取 keyspace 决定数据库与总量，再按库扫描。
func (b *browserService) runBigKeyScan(server string, task *bigKeyScanTask, item *connectionItem) {
	targets := b.configMasterTargets(task.ctx, item.client, server)
	if len(targets) == 0 {
		task.finish(errors.New("no reachable node"))
		return
	}
	defer closeConfigTargets(targets)

	for _, target := range targets {
		if task.ctx.Err() != nil {
			break
		}
		sizes := b.keyspaceSizes(task.ctx, target.Client)
		if sizes == nil {
			task.markUnavailable(target.Addr)
			continue
		}
		var nodeKeys int64
		for _, count := range sizes {
			nodeKeys += count
		}
		if task.options.Full {
			task.addTotal(nodeKeys)
		} else {
			task.addTotal(bigKeyNodeTotal(task.options.SampleSize, nodeKeys))
		}
	}

	for _, target := range targets {
		if task.ctx.Err() != nil {
			break
		}
		base, ok := target.Client.(*redis.Client)
		if !ok {
			continue
		}
		b.scanBigKeysOnNode(task, target.Addr, base.Options())
	}

	task.finish(nil)
}

// keyspaceSizes 读取节点的数据库键数量；读取失败返回 nil，由调用方标记为未响应。
func (b *browserService) keyspaceSizes(ctx context.Context, client redis.UniversalClient) map[int]int64 {
	info, err := client.Info(ctx, "keyspace").Result()
	if err != nil {
		return nil
	}
	return parseKeyspaceSizes(info)
}

// scanBigKeysOnNode 扫描单个节点上的所有非空数据库。
func (b *browserService) scanBigKeysOnNode(task *bigKeyScanTask, addr string, base *redis.Options) {
	probe := redis.NewClient(configNodeOptionsWithDB(addr, base, 0))
	defer probe.Close()

	info, err := probe.Info(task.ctx, "keyspace").Result()
	if err != nil {
		task.markUnavailable(addr)
		return
	}
	sizes := parseKeyspaceSizes(info)
	if len(sizes) == 0 {
		return
	}

	dbs := make([]int, 0, len(sizes))
	for db := range sizes {
		dbs = append(dbs, db)
	}
	sort.Ints(dbs)

	budget := task.options.SampleSize
	for index, db := range dbs {
		if task.ctx.Err() != nil {
			return
		}
		quota := 0
		if !task.options.Full {
			quota = bigKeyBudget(budget, len(dbs)-index)
			if quota <= 0 {
				return
			}
		}

		scanned, err := b.scanBigKeysOnDB(task, addr, base, db, quota)
		if err != nil {
			task.markUnavailable(addr)
			return
		}
		if !task.options.Full {
			budget -= scanned
			if budget <= 0 {
				return
			}
		}
	}
}

// scanBigKeysOnDB 扫描单个数据库；quota 为 0 表示全量扫描。
// 类型与内存查询按批流水线发送，避免每个键两次往返把远程实例的扫描拖到几十秒。
func (b *browserService) scanBigKeysOnDB(task *bigKeyScanTask, addr string, base *redis.Options, db, quota int) (int, error) {
	client := redis.NewClient(configNodeOptionsWithDB(addr, base, db))
	defer client.Close()

	cursor := uint64(0)
	scanned := 0
	for {
		if task.ctx.Err() != nil {
			return scanned, nil
		}

		batch := bigKeyScanBatch
		if quota > 0 && quota-scanned < batch {
			batch = quota - scanned
		}
		if batch <= 0 {
			return scanned, nil
		}

		keys, next, err := client.Scan(task.ctx, cursor, "*", int64(batch)).Result()
		if err != nil {
			return scanned, err
		}
		if len(keys) == 0 {
			cursor = next
			if cursor == 0 {
				return scanned, nil
			}
			continue
		}

		scanned += len(keys)
		task.incScanned(int64(len(keys)))

		typeCmds := make([]*redis.StatusCmd, len(keys))
		typePipe := client.Pipeline()
		for i, key := range keys {
			typeCmds[i] = typePipe.Type(task.ctx, key)
		}
		_, _ = typePipe.Exec(task.ctx)

		candidates := make([]string, 0, len(keys))
		candidateTypes := make([]string, 0, len(keys))
		for i, key := range keys {
			keyType, err := typeCmds[i].Result()
			if err != nil || keyType == "none" {
				continue
			}
			candidates = append(candidates, key)
			candidateTypes = append(candidateTypes, keyType)
		}

		if len(candidates) > 0 {
			usageCmds := make([]*redis.IntCmd, len(candidates))
			usagePipe := client.Pipeline()
			for i, key := range candidates {
				usageCmds[i] = usagePipe.MemoryUsage(task.ctx, key, 5)
			}
			_, _ = usagePipe.Exec(task.ctx)

			for i, key := range candidates {
				size, err := usageCmds[i].Result()
				if err != nil {
					continue
				}
				if size >= task.threshold {
					task.addFound(bigKeyItem{Key: key, Node: addr, DB: db, Type: candidateTypes[i], Size: size})
				}
			}
		}

		if quota > 0 && scanned >= quota {
			return scanned, nil
		}

		cursor = next
		if cursor == 0 {
			return scanned, nil
		}
		// 全量模式下逐批让出一点时间，避免长时间占满单节点 CPU
		time.Sleep(bigKeyRateLimitSleep)
	}
}
