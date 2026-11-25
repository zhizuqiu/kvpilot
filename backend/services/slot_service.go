package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

const (
	slotDefaultBatchKeys  = 100
	slotMaxBatchKeys      = 1000
	slotDefaultBatchBytes = 1 << 20
	slotDefaultTimeoutMs  = 5000
	slotDefaultIntervalMs = 10
	slotSampleSize        = 1000
	slotLogLimit          = 100
	slotLargeKeyBytes     = 100 << 20
	slotManyKeysThreshold = 100000
	slotMigratingFlag     = "migrating"
	slotImportingFlag     = "importing"
)

// slotRange 是一个槽位区间。
type slotRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// slotMarker 表示 CLUSTER NODES 中的迁移标记（[slot->-id] 或 [slot-<-id]）。
type slotMarker struct {
	Slot    int
	Peer    string
	Outward bool
}

// slotMaster 是槽位管理页中的一个主节点。
type slotMaster struct {
	ID         string      `json:"id"`
	Addr       string      `json:"addr"`
	SlotRanges []slotRange `json:"slotRanges"`
	SlotCount  int         `json:"slotCount"`
	Keys       int64       `json:"keys"`
	UsedMemory int64       `json:"usedMemory"`
	MaxMemory  int64       `json:"maxMemory"`
	Migrating  []int       `json:"migrating,omitempty"`
}

// slotOverviewData 是槽位管理页的返回体。
type slotOverviewData struct {
	Mode           string       `json:"mode"`
	State          string       `json:"state"`
	SlotsAssigned  int          `json:"slotsAssigned"`
	KnownNodes     int          `json:"knownNodes"`
	Masters        []slotMaster `json:"masters"`
	MigratingSlots []int        `json:"migratingSlots,omitempty"`
	Unavailable    []string     `json:"unavailable,omitempty"`
}

// slotWarning 是一条预检提示，前端按 code 本地化。
type slotWarning struct {
	Code   string            `json:"code"`
	Level  string            `json:"level"` // info / warn / error
	Values map[string]string `json:"values,omitempty"`
}

// slotPreviewData 是迁移预检的返回体。
type slotPreviewData struct {
	CanStart       bool          `json:"canStart"`
	Source         slotNodeRef   `json:"source"`
	Target         slotNodeRef   `json:"target"`
	Slot           int           `json:"slot"`
	KeyCount       int64         `json:"keyCount"`
	SampledKeys    int           `json:"sampledKeys"`
	EstimatedBytes int64         `json:"estimatedBytes"`
	LargestKey     *slotKeySize  `json:"largestKey,omitempty"`
	TargetUsed     int64         `json:"targetUsed"`
	TargetMax      int64         `json:"targetMax"`
	Resume         bool          `json:"resume"`
	Warnings       []slotWarning `json:"warnings"`
}

type slotNodeRef struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

type slotKeySize struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

// SlotMigrationOptions 供 API 层接收参数。
type SlotMigrationOptions = slotMigrationOptions

// slotMigrationOptions 是迁移限速参数。
type slotMigrationOptions struct {
	BatchKeys        int `json:"batchKeys"`
	BatchBytes       int `json:"batchBytes"`
	MigrateTimeoutMs int `json:"migrateTimeoutMs"`
	BatchIntervalMs  int `json:"batchIntervalMs"`
}

// slotLogEntry 是一条迁移日志。
type slotLogEntry struct {
	Time   int64             `json:"time"`
	Level  string            `json:"level"`
	Code   string            `json:"code"`
	Values map[string]string `json:"values,omitempty"`
}

// slotProgress 是迁移任务的当前状态。
type slotProgress struct {
	State      string               `json:"state"` // idle/running/paused/completed/failed/canceled
	Step       string               `json:"step"`
	Slot       int                  `json:"slot"`
	Source     slotNodeRef          `json:"source"`
	Target     slotNodeRef          `json:"target"`
	TotalKeys  int64                `json:"totalKeys"`
	MovedKeys  int64                `json:"movedKeys"`
	MovedBytes int64                `json:"movedBytes"`
	ElapsedMS  int64                `json:"elapsedMs"`
	Error      string               `json:"error,omitempty"`
	Log        []slotLogEntry       `json:"log"`
	Recovery   []string             `json:"recovery,omitempty"`
	Options    slotMigrationOptions `json:"options"`
}

type slotMigrationTask struct {
	mutex      sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	state      string
	step       string
	slot       int
	source     slotNodeRef
	target     slotNodeRef
	resume     bool
	totalKeys  int64
	movedKeys  int64
	movedBytes int64
	started    time.Time
	finishedAt time.Time
	errMsg     string
	log        []slotLogEntry
	recovery   []string
	pauseAsked bool
	resumeChan chan struct{}
	options    slotMigrationOptions
}

// parseSlotText 解析 CLUSTER NODES 的槽位字段，返回区间与迁移标记。
func parseSlotText(text string) ([]slotRange, []slotMarker) {
	ranges := make([]slotRange, 0, 4)
	markers := make([]slotMarker, 0)
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "[") {
			inner := strings.Trim(field, "[]")
			slotText, peer, ok := strings.Cut(inner, "-")
			if !ok {
				continue
			}
			slot, err := strconv.Atoi(slotText)
			if err != nil {
				continue
			}
			outward := strings.HasPrefix(peer, ">")
			peer = strings.TrimPrefix(peer, ">")
			peer = strings.TrimPrefix(peer, "<")
			peer = strings.TrimPrefix(peer, "-")
			markers = append(markers, slotMarker{Slot: slot, Peer: peer, Outward: outward})
			continue
		}
		if startText, endText, ok := strings.Cut(field, "-"); ok {
			start, err1 := strconv.Atoi(startText)
			end, err2 := strconv.Atoi(endText)
			if err1 == nil && err2 == nil {
				ranges = append(ranges, slotRange{Start: start, End: end})
			}
			continue
		}
		if slot, err := strconv.Atoi(field); err == nil {
			ranges = append(ranges, slotRange{Start: slot, End: slot})
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start < ranges[j].Start })
	return ranges, markers
}

func slotRangeCount(ranges []slotRange) int {
	total := 0
	for _, r := range ranges {
		if r.End >= r.Start {
			total += r.End - r.Start + 1
		}
	}
	return total
}

func nodeOwnsSlot(ranges []slotRange, slot int) bool {
	for _, r := range ranges {
		if slot >= r.Start && slot <= r.End {
			return true
		}
	}
	return false
}

// clampSlotOptions 收敛参数到安全范围。
func clampSlotOptions(options slotMigrationOptions) slotMigrationOptions {
	if options.BatchKeys <= 0 {
		options.BatchKeys = slotDefaultBatchKeys
	}
	if options.BatchKeys > slotMaxBatchKeys {
		options.BatchKeys = slotMaxBatchKeys
	}
	if options.BatchBytes <= 0 {
		options.BatchBytes = slotDefaultBatchBytes
	}
	if options.MigrateTimeoutMs <= 0 {
		options.MigrateTimeoutMs = slotDefaultTimeoutMs
	}
	if options.MigrateTimeoutMs > 60000 {
		options.MigrateTimeoutMs = 60000
	}
	if options.BatchIntervalMs < 0 {
		options.BatchIntervalMs = 0
	}
	if options.BatchIntervalMs > 1000 {
		options.BatchIntervalMs = 1000
	}
	return options
}

// trimBatchByBytes 按字节上限裁剪本批键，至少保留一个键。
func trimBatchByBytes(sizes []int64, limit int64) int {
	if len(sizes) == 0 {
		return 0
	}
	if limit <= 0 {
		return 1
	}
	var total int64
	for i, size := range sizes {
		total += size
		if total > limit && i > 0 {
			return i
		}
	}
	return len(sizes)
}

// slotNodeClient 为指定节点创建直连客户端，避免集群路由把 MIGRATE 发到别的节点。
func (b *browserService) slotNodeClient(item *connectionItem, addr string) (*redis.Client, error) {
	base := &redis.Options{}
	switch client := item.client.(type) {
	case *redis.Client:
		copied := client.Options()
		base = &redis.Options{
			Username:  copied.Username,
			Password:  copied.Password,
			Protocol:  copied.Protocol,
			TLSConfig: copied.TLSConfig,
			Dialer:    copied.Dialer,
		}
	case *redis.ClusterClient:
		base = configClusterNodeOptions(addr, client.Options())
	default:
		return nil, errors.New("unsupported redis client")
	}
	return redis.NewClient(configNodeOptionsWithDB(addr, base, 0)), nil
}

// clusterNodeView 聚合一次拓扑读取结果，供预检与迁移使用。
type clusterNodeView struct {
	node    topologyNode
	ranges  []slotRange
	markers []slotMarker
}

func (b *browserService) loadClusterView(server string) (string, []clusterNodeView, error) {
	resp := b.Topology(server)
	if !resp.Success {
		return "", nil, errors.New(resp.Msg)
	}
	data, ok := resp.Data.(topologyData)
	if !ok {
		return "", nil, errors.New("invalid topology data")
	}
	if data.Mode != "cluster" {
		return data.Mode, nil, nil
	}
	views := make([]clusterNodeView, 0, len(data.Nodes))
	for _, node := range data.Nodes {
		if node.Role != "master" {
			continue
		}
		ranges, markers := parseSlotText(node.Slots)
		views = append(views, clusterNodeView{node: node, ranges: ranges, markers: markers})
	}
	return data.Mode, views, nil
}

// SlotOverview 返回集群主节点的槽位分布。
func (b *browserService) SlotOverview(server string) (resp types.JSResp) {
	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	mode, views, err := b.loadClusterView(server)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	if mode != "cluster" {
		resp.Success = true
		resp.Data = slotOverviewData{Mode: mode, Masters: []slotMaster{}}
		return
	}

	targets := b.configMasterTargets(item.ctx, item.client, server)
	defer closeConfigTargets(targets)
	targetByAddr := map[string]configTarget{}
	for _, target := range targets {
		targetByAddr[target.Addr] = target
	}

	data := slotOverviewData{
		Mode:           mode,
		Masters:        make([]slotMaster, 0, len(views)),
		MigratingSlots: []int{},
	}
	for _, view := range views {
		master := slotMaster{
			ID:         view.node.ID,
			Addr:       view.node.Addr,
			SlotRanges: view.ranges,
			SlotCount:  slotRangeCount(view.ranges),
		}
		for _, marker := range view.markers {
			master.Migrating = append(master.Migrating, marker.Slot)
			data.MigratingSlots = append(data.MigratingSlots, marker.Slot)
		}

		if target, ok := targetByAddr[view.node.Addr]; ok {
			if client, ok := target.Client.(*redis.Client); ok {
				if info, err := client.Info(item.ctx, "keyspace").Result(); err == nil {
					for _, count := range parseKeyspaceSizes(info) {
						master.Keys += count
					}
				}
				if memory, err := client.Info(item.ctx, "memory").Result(); err == nil {
					parsed := b.parseInfo(memory)
					master.UsedMemory = checkInt(checkValue(parsed, "Memory", "used_memory"))
					master.MaxMemory = checkInt(checkValue(parsed, "Memory", "maxmemory"))
				}
				if clusterInfo, err := client.Do(item.ctx, "CLUSTER", "INFO").Text(); err == nil {
					for _, line := range strings.Split(clusterInfo, "\n") {
						line = strings.TrimSpace(line)
						if key, value, ok := strings.Cut(line, ":"); ok {
							switch key {
							case "cluster_state":
								data.State = value
							case "cluster_slots_assigned":
								data.SlotsAssigned, _ = strconv.Atoi(value)
							case "cluster_known_nodes":
								data.KnownNodes, _ = strconv.Atoi(value)
							}
						}
					}
				}
			}
		} else {
			data.Unavailable = append(data.Unavailable, view.node.Addr)
		}
		data.Masters = append(data.Masters, master)
	}

	sort.Ints(data.MigratingSlots)
	firstSlot := func(master slotMaster) int {
		if len(master.SlotRanges) == 0 {
			return 1 << 20
		}
		return master.SlotRanges[0].Start
	}
	sort.Slice(data.Masters, func(i, j int) bool {
		left, right := firstSlot(data.Masters[i]), firstSlot(data.Masters[j])
		if left != right {
			return left < right
		}
		return data.Masters[i].Addr < data.Masters[j].Addr
	})

	resp.Success = true
	resp.Data = data
	return
}

// PreviewSlotMigration 校验并估算一次单槽迁移的影响。
func (b *browserService) PreviewSlotMigration(server, sourceID, targetID string, slot int) (resp types.JSResp) {
	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	if slot < 0 || slot > 16383 {
		resp.Msg = "slot out of range"
		return
	}

	mode, views, err := b.loadClusterView(server)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	if mode != "cluster" {
		resp.Msg = "not a cluster connection"
		return
	}

	byID := map[string]clusterNodeView{}
	for _, view := range views {
		byID[view.node.ID] = view
	}
	source, sourceOK := byID[sourceID]
	target, targetOK := byID[targetID]

	data := slotPreviewData{
		Slot:     slot,
		Warnings: make([]slotWarning, 0, 6),
	}
	addWarning := func(code, level string, values map[string]string) {
		data.Warnings = append(data.Warnings, slotWarning{Code: code, Level: level, Values: values})
	}

	switch {
	case !sourceOK || !targetOK:
		addWarning("node_not_found", "error", nil)
	case sourceID == targetID:
		addWarning("same_node", "error", nil)
	case !nodeOwnsSlot(source.ranges, slot):
		addWarning("slot_not_owned", "error", map[string]string{"source": source.node.Addr})
	case nodeOwnsSlot(target.ranges, slot):
		addWarning("slot_already_on_target", "error", map[string]string{"target": target.node.Addr})
	}

	if !sourceOK || !targetOK {
		data.CanStart = false
		resp.Success = true
		resp.Data = data
		return
	}
	data.Source = slotNodeRef{ID: source.node.ID, Addr: source.node.Addr}
	data.Target = slotNodeRef{ID: target.node.ID, Addr: target.node.Addr}

	// 已存在的迁移标记：同源同目标视为续传，其它情况禁止
	var sourceMarker, targetMarker *slotMarker
	for i := range source.markers {
		if source.markers[i].Slot == slot {
			sourceMarker = &source.markers[i]
		}
	}
	for i := range target.markers {
		if target.markers[i].Slot == slot {
			targetMarker = &target.markers[i]
		}
	}
	if sourceMarker != nil || targetMarker != nil {
		same := sourceMarker != nil && sourceMarker.Outward && sourceMarker.Peer == targetID &&
			targetMarker != nil && !targetMarker.Outward && targetMarker.Peer == sourceID
		if same {
			data.Resume = true
			addWarning("resume", "info", nil)
		} else {
			addWarning("existing_migration", "error", nil)
		}
	}

	sourceClient, err := b.slotNodeClient(item, source.node.Addr)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	defer sourceClient.Close()
	targetClient, err := b.slotNodeClient(item, target.node.Addr)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	defer targetClient.Close()

	// 集群状态
	if info, err := sourceClient.Do(item.ctx, "CLUSTER", "INFO").Text(); err == nil {
		for _, line := range strings.Split(info, "\n") {
			line = strings.TrimSpace(line)
			if key, value, ok := strings.Cut(line, ":"); ok {
				switch key {
				case "cluster_state":
					if value != "ok" {
						addWarning("cluster_state", "error", map[string]string{"state": value})
					}
				case "cluster_slots_assigned":
					if assigned, _ := strconv.Atoi(value); assigned != 16384 {
						addWarning("slots_incomplete", "error", map[string]string{"assigned": value})
					}
				}
			}
		}
	} else {
		addWarning("source_unreachable", "error", nil)
	}

	// 目标可达性与容量
	if memory, err := targetClient.Info(item.ctx, "memory").Result(); err == nil {
		parsed := b.parseInfo(memory)
		data.TargetUsed = checkInt(checkValue(parsed, "Memory", "used_memory"))
		data.TargetMax = checkInt(checkValue(parsed, "Memory", "maxmemory"))
	} else {
		addWarning("target_unreachable", "error", nil)
	}

	// 键数量与体积采样
	if count, err := sourceClient.Do(item.ctx, "CLUSTER", "COUNTKEYSINSLOT", slot).Int64(); err == nil {
		data.KeyCount = count
	} else {
		addWarning("permission_denied", "error", map[string]string{"msg": err.Error()})
	}

	if data.KeyCount > 0 {
		keys, err := sourceClient.Do(item.ctx, "CLUSTER", "GETKEYSINSLOT", slot, slotSampleSize).StringSlice()
		if err == nil && len(keys) > 0 {
			pipe := sourceClient.Pipeline()
			typeCmds := make([]*redis.StatusCmd, len(keys))
			sizeCmds := make([]*redis.IntCmd, len(keys))
			for i, key := range keys {
				typeCmds[i] = pipe.Type(item.ctx, key)
				sizeCmds[i] = pipe.MemoryUsage(item.ctx, key, 5)
			}
			_, _ = pipe.Exec(item.ctx)

			var sampleBytes int64
			largest := &slotKeySize{}
			for i, key := range keys {
				size, err := sizeCmds[i].Result()
				if err != nil {
					continue
				}
				sampleBytes += size
				if size > largest.Size {
					keyType, _ := typeCmds[i].Result()
					largest = &slotKeySize{Key: key, Size: size, Type: keyType}
				}
			}
			data.SampledKeys = len(keys)
			data.LargestKey = largest
			if len(keys) > 0 {
				data.EstimatedBytes = sampleBytes * data.KeyCount / int64(len(keys))
			}
			if largest.Size >= slotLargeKeyBytes {
				addWarning("large_key", "warn", map[string]string{
					"key":  largest.Key,
					"size": formatBytesHuman(largest.Size),
				})
			}
		}
	}
	if data.KeyCount >= slotManyKeysThreshold {
		addWarning("many_keys", "warn", map[string]string{"count": strconv.FormatInt(data.KeyCount, 10)})
	}
	if data.TargetMax > 0 && data.EstimatedBytes > 0 &&
		data.TargetUsed+data.EstimatedBytes*2 > data.TargetMax {
		addWarning("target_memory", "warn", map[string]string{"free": formatBytesHuman(data.TargetMax - data.TargetUsed)})
	}

	data.CanStart = true
	for _, warning := range data.Warnings {
		if warning.Level == "error" {
			data.CanStart = false
			break
		}
	}

	resp.Success = true
	resp.Data = data
	return
}

// StartSlotMigration 启动一次单槽迁移。
func (b *browserService) StartSlotMigration(server, sourceID, targetID string, slot int, options slotMigrationOptions) (resp types.JSResp) {
	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	previewResp := b.PreviewSlotMigration(server, sourceID, targetID, slot)
	if !previewResp.Success {
		resp.Msg = previewResp.Msg
		return
	}
	preview := previewResp.Data.(slotPreviewData)
	if !preview.CanStart {
		for _, warning := range preview.Warnings {
			if warning.Level == "error" {
				resp.Msg = "precheck failed: " + warning.Code
				return
			}
		}
		resp.Msg = "precheck failed"
		return
	}

	b.slotMutex.Lock()
	if b.slotTasks == nil {
		b.slotTasks = map[string]*slotMigrationTask{}
	}
	if existing, ok := b.slotTasks[server]; ok {
		existing.mutex.Lock()
		busy := existing.state == "running" || existing.state == "paused"
		existing.mutex.Unlock()
		if busy {
			b.slotMutex.Unlock()
			resp.Msg = "migration already running"
			return
		}
	}

	ctx, cancel := context.WithCancel(item.ctx)
	task := &slotMigrationTask{
		ctx:        ctx,
		cancel:     cancel,
		state:      "running",
		step:       "preparing",
		slot:       slot,
		source:     preview.Source,
		target:     preview.Target,
		resume:     preview.Resume,
		started:    time.Now(),
		resumeChan: make(chan struct{}, 1),
		options:    clampSlotOptions(options),
		log:        make([]slotLogEntry, 0, 32),
	}
	b.slotTasks[server] = task
	b.slotMutex.Unlock()

	go b.runSlotMigration(item, task)
	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// GetSlotMigration 返回最近一次迁移任务的进度。
func (b *browserService) GetSlotMigration(server string) (resp types.JSResp) {
	b.slotMutex.Lock()
	task := b.slotTasks[server]
	b.slotMutex.Unlock()
	if task == nil {
		resp.Success = true
		resp.Data = slotProgress{State: "idle", Log: []slotLogEntry{}}
		return
	}
	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// PauseSlotMigration 在当前批次结束后暂停迁移。
func (b *browserService) PauseSlotMigration(server string) (resp types.JSResp) {
	task := b.getSlotTask(server)
	if task == nil {
		resp.Msg = "no migration task"
		return
	}
	task.mutex.Lock()
	if task.state != "running" {
		task.mutex.Unlock()
		resp.Msg = "migration is not running"
		return
	}
	task.pauseAsked = true
	task.mutex.Unlock()

	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// ResumeSlotMigration 继续已暂停的迁移。
func (b *browserService) ResumeSlotMigration(server string) (resp types.JSResp) {
	task := b.getSlotTask(server)
	if task == nil {
		resp.Msg = "no migration task"
		return
	}
	task.mutex.Lock()
	if task.state != "paused" {
		task.mutex.Unlock()
		resp.Msg = "migration is not paused"
		return
	}
	task.state = "running"
	task.pauseAsked = false
	task.mutex.Unlock()

	select {
	case task.resumeChan <- struct{}{}:
	default:
	}
	resp.Success = true
	resp.Data = task.snapshot()
	return
}

// CancelSlotMigration 仅在尚未搬键时允许取消；已搬键只能暂停或继续。
func (b *browserService) CancelSlotMigration(server string) (resp types.JSResp) {
	task := b.getSlotTask(server)
	if task == nil {
		resp.Msg = "no migration task"
		return
	}
	task.mutex.Lock()
	if task.state != "running" && task.state != "paused" {
		task.mutex.Unlock()
		resp.Msg = "migration is not active"
		return
	}
	moved := task.movedKeys
	task.mutex.Unlock()
	if moved > 0 {
		resp.Msg = "keys already moved; only pause or resume is allowed"
		return
	}

	task.cancel()
	resp.Success = true
	resp.Data = task.snapshot()
	return
}

func (b *browserService) getSlotTask(server string) *slotMigrationTask {
	b.slotMutex.Lock()
	defer b.slotMutex.Unlock()
	return b.slotTasks[server]
}

func (t *slotMigrationTask) snapshot() slotProgress {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	elapsed := time.Since(t.started)
	if !t.finishedAt.IsZero() {
		elapsed = t.finishedAt.Sub(t.started)
	}
	logs := append([]slotLogEntry(nil), t.log...)
	recovery := append([]string(nil), t.recovery...)
	progress := slotProgress{
		State:      t.state,
		Step:       t.step,
		Slot:       t.slot,
		Source:     t.source,
		Target:     t.target,
		TotalKeys:  t.totalKeys,
		MovedKeys:  t.movedKeys,
		MovedBytes: t.movedBytes,
		ElapsedMS:  elapsed.Milliseconds(),
		Error:      t.errMsg,
		Log:        logs,
		Recovery:   recovery,
		Options:    t.options,
	}
	return progress
}

func (t *slotMigrationTask) setState(state, step string) {
	t.mutex.Lock()
	t.state = state
	t.step = step
	t.mutex.Unlock()
}

func (t *slotMigrationTask) addLog(level, code string, values map[string]string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.log = append(t.log, slotLogEntry{
		Time:   time.Since(t.started).Milliseconds(),
		Level:  level,
		Code:   code,
		Values: values,
	})
	if len(t.log) > slotLogLimit {
		t.log = t.log[len(t.log)-slotLogLimit:]
	}
}

func (t *slotMigrationTask) fail(message string, recovery []string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.state = "failed"
	t.errMsg = message
	t.recovery = recovery
	t.finishedAt = time.Now()
}

func (t *slotMigrationTask) complete(step string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.state = "completed"
	t.step = step
	t.finishedAt = time.Now()
}

// failOrCancel 统一处理执行期错误：主动取消且尚未搬键时清除标记并按取消结束。
func (t *slotMigrationTask) failOrCancel(err error, source, target *redis.Client, recovery []string) {
	if t.ctx.Err() != nil {
		t.mutex.Lock()
		moved := t.movedKeys
		t.mutex.Unlock()
		if moved == 0 {
			clearSlotFlags(context.Background(), source, target, t.slot)
			t.canceled()
			return
		}
		t.fail("migration interrupted", recovery)
		return
	}
	t.fail(err.Error(), recovery)
}

func (t *slotMigrationTask) canceled() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.state = "canceled"
	t.finishedAt = time.Now()
}

// runSlotMigration 执行单槽迁移的全部步骤。
func (b *browserService) runSlotMigration(item *connectionItem, task *slotMigrationTask) {
	sourceClient, err := b.slotNodeClient(item, task.source.Addr)
	if err != nil {
		task.fail(err.Error(), nil)
		return
	}
	defer sourceClient.Close()
	targetClient, err := b.slotNodeClient(item, task.target.Addr)
	if err != nil {
		task.fail(err.Error(), nil)
		return
	}
	defer targetClient.Close()

	ctx := task.ctx
	migrateArgs := []string{"MIGRATE", hostOf(task.target.Addr), portOf(task.target.Addr), "", "0", strconv.Itoa(task.options.MigrateTimeoutMs)}
	// 目标节点的认证信息来自连接配置
	if opts := targetClient.Options(); opts.Username != "" {
		migrateArgs = append(migrateArgs, "AUTH2", opts.Username, opts.Password)
	} else if opts := targetClient.Options(); opts.Password != "" {
		migrateArgs = append(migrateArgs, "AUTH", opts.Password)
	}
	migrateArgs = append(migrateArgs, "REPLACE")

	recovery := []string{
		fmt.Sprintf("CLUSTER SETSLOT %d MIGRATING %s", task.slot, task.target.ID),
		fmt.Sprintf("CLUSTER SETSLOT %d IMPORTING %s", task.slot, task.source.ID),
		fmt.Sprintf("CLUSTER GETKEYSINSLOT %d %d", task.slot, task.options.BatchKeys),
	}

	if !task.resume {
		task.setState("running", "marking")
		if err := sourceClient.Do(ctx, "CLUSTER", "SETSLOT", task.slot, slotMigratingFlag, task.target.ID).Err(); err != nil {
			task.failOrCancel(err, sourceClient, targetClient, recovery)
			return
		}
		task.addLog("info", "set_migrating", nil)
		if err := targetClient.Do(ctx, "CLUSTER", "SETSLOT", task.slot, slotImportingFlag, task.source.ID).Err(); err != nil {
			task.failOrCancel(err, sourceClient, targetClient, recovery)
			return
		}
		task.addLog("info", "set_importing", nil)
	} else {
		task.addLog("info", "resume_migration", nil)
	}

	total, err := sourceClient.Do(ctx, "CLUSTER", "COUNTKEYSINSLOT", task.slot).Int64()
	if err != nil {
		task.failOrCancel(err, sourceClient, targetClient, recovery)
		return
	}
	task.mutex.Lock()
	task.totalKeys = total
	task.mutex.Unlock()
	task.addLog("info", "total_keys", map[string]string{"count": strconv.FormatInt(total, 10)})

	task.setState("running", "migrating")
	for {
		select {
		case <-ctx.Done():
			// 应用关闭或主动取消：未搬键时清除标记，已搬键时视为中断
			task.mutex.Lock()
			moved := task.movedKeys
			task.mutex.Unlock()
			if moved == 0 {
				clearSlotFlags(context.Background(), sourceClient, targetClient, task.slot)
				task.canceled()
			} else {
				task.fail("migration interrupted", recovery)
			}
			return
		default:
		}

		if task.isPauseAsked() {
			task.setState("paused", "migrating")
			task.addLog("info", "paused", nil)
			select {
			case <-ctx.Done():
				continue
			case <-task.resumeChan:
				task.addLog("info", "resumed", nil)
			}
		}

		keys, err := sourceClient.Do(ctx, "CLUSTER", "GETKEYSINSLOT", task.slot, task.options.BatchKeys).StringSlice()
		if err != nil {
			task.failOrCancel(err, sourceClient, targetClient, recovery)
			return
		}
		if len(keys) == 0 {
			break
		}

		// 按字节上限裁剪本批，避免单次 MIGRATE 长时间阻塞源节点
		sizes := make([]int64, len(keys))
		pipe := sourceClient.Pipeline()
		sizeCmds := make([]*redis.IntCmd, len(keys))
		for i, key := range keys {
			sizeCmds[i] = pipe.MemoryUsage(ctx, key, 5)
		}
		_, _ = pipe.Exec(ctx)
		var batchBytes int64
		for i := range keys {
			if size, err := sizeCmds[i].Result(); err == nil {
				sizes[i] = size
			}
		}
		count := trimBatchByBytes(sizes, int64(task.options.BatchBytes))
		keys = keys[:count]
		for i := 0; i < count; i++ {
			batchBytes += sizes[i]
		}

		args := append(append([]string{}, migrateArgs...), "KEYS")
		args = append(args, keys...)
		if err := sourceClient.Do(ctx, stringSliceToAny(args)...).Err(); err != nil {
			task.failOrCancel(err, sourceClient, targetClient, recovery)
			return
		}

		task.mutex.Lock()
		task.movedKeys += int64(len(keys))
		task.movedBytes += batchBytes
		progressTotal := task.movedKeys
		task.mutex.Unlock()
		task.addLog("info", "keys_moved", map[string]string{
			"count": strconv.Itoa(len(keys)),
			"total": strconv.FormatInt(progressTotal, 10),
		})

		if task.options.BatchIntervalMs > 0 {
			select {
			case <-ctx.Done():
				continue
			case <-time.After(time.Duration(task.options.BatchIntervalMs) * time.Millisecond):
			}
		}
	}

	task.setState("running", "committing")
	if err := targetClient.Do(ctx, "CLUSTER", "SETSLOT", task.slot, "NODE", task.target.ID).Err(); err != nil {
		task.failOrCancel(err, sourceClient, targetClient, recovery)
		return
	}
	// 同时通知源节点，避免其视图长时间停留在旧归属与 MIGRATING 标记
	if err := sourceClient.Do(ctx, "CLUSTER", "SETSLOT", task.slot, "NODE", task.target.ID).Err(); err != nil {
		task.failOrCancel(err, sourceClient, targetClient, recovery)
		return
	}
	task.addLog("info", "slot_committed", nil)

	task.setState("running", "verifying")
	if err := verifySlotOwner(ctx, sourceClient, targetClient, task.slot, task.source.ID, task.target.ID); err != nil {
		task.failOrCancel(err, sourceClient, targetClient, recovery)
		return
	}
	task.addLog("info", "verify_ok", nil)
	task.complete("done")
}

func (t *slotMigrationTask) isPauseAsked() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.pauseAsked
}

func clearSlotFlags(ctx context.Context, source, target *redis.Client, slot int) {
	_ = source.Do(ctx, "CLUSTER", "SETSLOT", slot, "STABLE").Err()
	_ = target.Do(ctx, "CLUSTER", "SETSLOT", slot, "STABLE").Err()
}

// verifySlotOwner 轮询源节点与目标节点视图，等待槽位归属在两边都切换完成。
func verifySlotOwner(ctx context.Context, source, target *redis.Client, slot int, sourceID, targetID string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		targetOwns := nodeOwnsSlotInView(ctx, target, slot, targetID)
		sourceStillOwns := nodeOwnsSlotInView(ctx, source, slot, sourceID)
		if targetOwns && !sourceStillOwns {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("slot ownership not updated")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// nodeOwnsSlotInView 判断某个节点视图中指定节点是否拥有该槽位。
func nodeOwnsSlotInView(ctx context.Context, client *redis.Client, slot int, nodeID string) bool {
	text, err := client.Do(ctx, "CLUSTER", "NODES").Text()
	if err != nil {
		return false
	}
	for _, node := range parseClusterNodesRaw(text, "") {
		if node.ID != nodeID {
			continue
		}
		ranges, _ := parseSlotText(node.Slots)
		return nodeOwnsSlot(ranges, slot)
	}
	return false
}

func hostOf(addr string) string {
	host, _, err := netSplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func portOf(addr string) string {
	_, port, err := netSplitHostPort(addr)
	if err != nil {
		return "6379"
	}
	return port
}

func netSplitHostPort(addr string) (string, string, error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, "", errors.New("invalid addr")
	}
	return addr[:idx], addr[idx+1:], nil
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}
