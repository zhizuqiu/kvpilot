package services

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

// configParam 是一个 Redis 参数在各节点上的合并视图。
type configParam struct {
	Key   string `json:"key"`
	Value string `json:"value"` // 代表性值
	Type  string `json:"type"`  // bool / int / memory / string
	// Values 仅在节点间不一致时携带，addr -> 值
	Values     map[string]string `json:"values,omitempty"`
	Consistent bool              `json:"consistent"`
	Nodes      int               `json:"nodes"`
	Sensitive  bool              `json:"sensitive"`
	Immutable  bool              `json:"immutable"`
	Category   string            `json:"category"`
}

// configData 是配置浏览接口的返回体。
type configData struct {
	Params []configParam `json:"params"`
	Nodes  []string      `json:"nodes"`
	// Unavailable 是本次读取失败的节点地址：这些节点的值没有参与比较，
	// 前端需要明确提示，否则页面会把"没读到"显示成"一致"。
	Unavailable []string `json:"unavailable,omitempty"`
	Merged      bool     `json:"merged"`
}

// configNodeResult 记录某个节点上的写入结果。
type configNodeResult struct {
	Addr string `json:"addr"`
	Set  bool   `json:"set"`
	// Rewrite: persisted（已写入配置文件）/ memory-only（无配置文件）/ failed（其它改写失败）
	Rewrite string `json:"rewrite"`
	Msg     string `json:"msg,omitempty"`
}

// sensitiveConfigKeys 读取时脱敏且禁止编辑，避免把客户端锁在门外。
var sensitiveConfigKeys = map[string]bool{
	"requirepass":               true,
	"masterauth":                true,
	"tls-key-file-pass":         true,
	"tls-client-key-file-pass":  true,
	"tls-client-cert-file-pass": true,
}

// immutableConfigKeys 运行期无法通过 CONFIG SET 修改的参数。
var immutableConfigKeys = map[string]bool{
	"databases": true, "daemonize": true, "logfile": true, "pidfile": true,
	"port": true, "bind": true, "dir": true, "dbfilename": true,
	"cluster-enabled": true, "cluster-config-file": true, "unixsocket": true,
	"supervised": true, "always-show-logo": true, "set-proc-title": true,
	"proc-title-template": true, "replicaof": true, "slaveof": true,
	"io-threads": true, "tls-port": true, "tls-cert-file": true, "tls-key-file": true,
	"tls-ca-cert-file": true, "tls-ca-cert-dir": true, "aclfile": true,
}

// configCategoryRules 按前缀归类，顺序敏感：更具体的前缀放在前面。
var configCategoryRules = []struct {
	Prefix   string
	Category string
}{
	{"maxmemory", "memory"},
	{"activedefrag", "memory"},
	{"lazyfree", "memory"},
	{"save", "persistence"},
	{"append", "persistence"},
	{"aof", "persistence"},
	{"rdb", "persistence"},
	{"auto-aof", "persistence"},
	{"stop-writes", "persistence"},
	{"dir", "persistence"},
	{"dbfilename", "persistence"},
	{"repl", "replication"},
	{"replica", "replication"},
	{"slave", "replication"},
	{"min-replicas", "replication"},
	{"cluster", "cluster"},
	{"slowlog", "slowlog"},
	{"latency", "slowlog"},
	{"requirepass", "security"},
	{"masterauth", "security"},
	{"acl", "security"},
	{"protected-mode", "security"},
	{"rename-command", "security"},
	{"tls", "security"},
	{"user", "security"},
	{"bind", "network"},
	{"port", "network"},
	{"unixsocket", "network"},
	{"tcp", "network"},
	{"timeout", "network"},
	{"maxclients", "clients"},
	{"client", "clients"},
	{"notify-keyspace", "clients"},
	{"lua", "lua"},
	{"busy-reply", "lua"},
	{"hz", "performance"},
	{"dynamic-hz", "performance"},
	{"io-threads", "performance"},
	{"jemalloc", "performance"},
	{"appendfsync", "persistence"},
	{"log", "logging"},
	{"syslog", "logging"},
	{"supervised", "logging"},
	{"pidfile", "logging"},
}

// configCategory 返回参数所属分类 key，前端负责翻译。
func configCategory(key string) string {
	for _, rule := range configCategoryRules {
		if strings.HasPrefix(key, rule.Prefix) {
			return rule.Category
		}
	}
	return "other"
}

// parseConfigPairs 把 CONFIG GET 的返回规整成 key -> value。
// RESP2 返回扁平数组，RESP3 返回 map，两种都要兼容。
func parseConfigPairs(raw interface{}) map[string]string {
	ret := map[string]string{}
	switch data := raw.(type) {
	case []interface{}:
		for i := 0; i+1 < len(data); i += 2 {
			k, ok1 := data[i].(string)
			v, ok2 := data[i+1].(string)
			if ok1 && ok2 {
				ret[k] = v
			}
		}
	case map[interface{}]interface{}:
		for k, v := range data {
			ks, ok1 := k.(string)
			vs, ok2 := v.(string)
			if ok1 && ok2 {
				ret[ks] = vs
			}
		}
	}
	return ret
}

// configValueType 按取值推断展示类型，供前端选择控件。
func configValueType(value string) string {
	switch strings.ToLower(value) {
	case "yes", "no":
		return "bool"
	}
	if _, err := parseConfigInt(value); err == nil {
		return "int"
	}
	if isMemorySize(value) {
		return "memory"
	}
	return "string"
}

func parseConfigInt(value string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
}

// isMemorySize 判断 100mb / 1gb 这类内存写法。
func isMemorySize(value string) bool {
	s := strings.ToLower(strings.TrimSpace(value))
	for _, unit := range []string{"kb", "mb", "gb", "tb"} {
		if strings.HasSuffix(s, unit) {
			_, err := parseConfigInt(strings.TrimSuffix(s, unit))
			return err == nil
		}
	}
	return false
}

// mergeConfigValues 合并各节点的配置视图。
func mergeConfigValues(views map[string]map[string]string) []configParam {
	values := map[string]map[string]string{}
	addrs := make([]string, 0, len(views))
	for addr := range views {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	for _, addr := range addrs {
		for key, value := range views[addr] {
			if values[key] == nil {
				values[key] = map[string]string{}
			}
			values[key][addr] = value
		}
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]configParam, 0, len(keys))
	for _, key := range keys {
		perNode := values[key]
		repr := perNode[addrs[0]]
		consistent := true
		for _, addr := range addrs {
			if v, ok := perNode[addr]; !ok || v != repr {
				consistent = false
				break
			}
		}

		param := configParam{
			Key:        key,
			Value:      repr,
			Type:       configValueType(repr),
			Consistent: consistent,
			Nodes:      len(perNode),
			Sensitive:  sensitiveConfigKeys[key],
			Immutable:  immutableConfigKeys[key],
			Category:   configCategory(key),
		}
		if !consistent {
			param.Values = perNode
		}
		if param.Sensitive {
			param.Value = "***"
			param.Type = "string"
			param.Values = nil
		}
		params = append(params, param)
	}
	return params
}

// configTarget 表示一个待读写的节点。
type configTarget struct {
	Addr   string
	Client redis.UniversalClient
	close  func()
}

// configTopologyAddrs 从拓扑数据里取出数据节点地址（主节点与从节点），去重排序。
func configTopologyAddrs(data topologyData) []string {
	seen := map[string]bool{}
	addrs := make([]string, 0, len(data.Nodes))
	for _, node := range data.Nodes {
		if node.Role != "master" && node.Role != "replica" {
			continue
		}
		if node.Addr == "" || seen[node.Addr] {
			continue
		}
		seen[node.Addr] = true
		addrs = append(addrs, node.Addr)
	}
	sort.Strings(addrs)
	return addrs
}

// configNodeDialTimeout 限制单节点拨号耗时：不可达节点不能拖慢整页加载。
const configNodeDialTimeout = 500 * time.Millisecond

// configNodeOptions 构造单节点配置客户端。
// 拨号最多两轮：一次瞬时抖动不应把节点标记为未响应，
// 同时避免默认的 5 轮重试把整页加载拖到几十秒。
func configNodeOptions(addr string, base *redis.Options) *redis.Options {
	return &redis.Options{
		Addr:          addr,
		Username:      base.Username,
		Password:      base.Password,
		DialTimeout:   configNodeDialTimeout,
		DialerRetries: 2,
		ReadTimeout:   2 * time.Second,
		WriteTimeout:  2 * time.Second,
		MaxRetries:    1,
		Protocol:      base.Protocol,
		TLSConfig:     base.TLSConfig,
		Dialer:        base.Dialer,
	}
}

// configNodeOptionsWithDB 在通用选项上指定数据库，扫描类任务按库独立建连。
func configNodeOptionsWithDB(addr string, base *redis.Options, db int) *redis.Options {
	opt := configNodeOptions(addr, base)
	opt.DB = db
	return opt
}

// configClusterNodeOptions 与 configNodeOptions 同义，只是集群选项里的凭据字段位置不同。
func configClusterNodeOptions(addr string, base *redis.ClusterOptions) *redis.Options {
	return configNodeOptions(addr, &redis.Options{
		Username:  base.Username,
		Password:  base.Password,
		Protocol:  base.Protocol,
		TLSConfig: base.TLSConfig,
		Dialer:    base.Dialer,
	})
}

// clusterSeedReader 读取某个节点返回的 CLUSTER NODES 原文。
type clusterSeedReader func(ctx context.Context, addr string) (string, error)

// clusterAddrsFromSeeds 依次尝试种子节点，第一个能给出节点列表的即为结果。
// 不依赖集群客户端：它会把命令路由到失联节点，等满 ExecTimeout 才返回。
func clusterAddrsFromSeeds(ctx context.Context, seeds []string, read clusterSeedReader) ([]string, bool) {
	tried := map[string]bool{}
	for _, seed := range seeds {
		if seed == "" || tried[seed] {
			continue
		}
		tried[seed] = true

		text, err := read(ctx, seed)
		if err != nil {
			continue
		}
		nodes := parseClusterNodes(text, "")
		if len(nodes) == 0 {
			continue
		}
		addrs := make([]string, 0, len(nodes))
		for _, node := range nodes {
			addrs = append(addrs, node.Addr)
		}
		return addrs, true
	}
	return nil, false
}

// discoverClusterAddrs 用短超时客户端读取集群节点地址。
func discoverClusterAddrs(ctx context.Context, seeds []string, base *redis.ClusterOptions) ([]string, bool) {
	return clusterAddrsFromSeeds(ctx, seeds, func(ctx context.Context, addr string) (string, error) {
		client := redis.NewClient(configClusterNodeOptions(addr, base))
		defer client.Close()
		return client.Do(ctx, "CLUSTER", "NODES").Text()
	})
}

// filterMasterTargets 只保留主节点；副本不参与扫描，避免同一份数据被重复统计。
// 未匹配到任何主节点时原样返回，保证拓扑异常时任务仍可执行。
func filterMasterTargets(targets []configTarget, masters map[string]bool) []configTarget {
	if len(masters) == 0 || len(targets) == 0 {
		return targets
	}
	matched := 0
	for _, target := range targets {
		if masters[target.Addr] {
			matched++
		}
	}
	if matched == 0 {
		return targets
	}

	kept := make([]configTarget, 0, matched)
	for _, target := range targets {
		if masters[target.Addr] {
			kept = append(kept, target)
			continue
		}
		if target.close != nil {
			target.close()
		}
	}
	return kept
}

// configMasterTargets 返回主节点列表，供大 key 与键空间统计等扫描类任务使用。
func (b *browserService) configMasterTargets(ctx context.Context, client redis.UniversalClient, name string) []configTarget {
	targets, _ := b.configTargets(ctx, client, name)
	if len(targets) <= 1 {
		return targets
	}

	resp := b.Topology(name)
	if !resp.Success {
		return targets
	}
	topology, ok := resp.Data.(topologyData)
	if !ok {
		return targets
	}
	masters := map[string]bool{}
	for _, node := range topology.Nodes {
		if node.Role == "master" && node.Addr != "" {
			masters[node.Addr] = true
		}
	}
	return filterMasterTargets(targets, masters)
}

// configTargets 返回需要操作的节点列表。
// CONFIG 不会随复制传播，因此主从/哨兵也要覆盖从节点，集群则覆盖全部节点。
func (b *browserService) configTargets(ctx context.Context, client redis.UniversalClient, name string) ([]configTarget, bool) {
	cc, ok := client.(*redis.ClusterClient)
	if !ok {
		baseAddr := ""
		var baseOpts *redis.Options
		if c, ok := client.(*redis.Client); ok {
			baseOpts = c.Options()
			baseAddr = baseOpts.Addr
		}

		// 借助拓扑信息补齐从节点；拿不到拓扑（或只有自身）时退化为单节点
		addrs := []string{}
		if name != "" {
			if resp := b.Topology(name); resp.Success {
				if data, ok := resp.Data.(topologyData); ok {
					addrs = configTopologyAddrs(data)
				}
			}
		}
		if len(addrs) <= 1 || baseOpts == nil {
			return []configTarget{{Addr: baseAddr, Client: client}}, true
		}

		targets := make([]configTarget, 0, len(addrs))
		for _, addr := range addrs {
			if addr == baseAddr {
				targets = append(targets, configTarget{Addr: addr, Client: client})
				continue
			}
			c := redis.NewClient(configNodeOptions(addr, baseOpts))
			targets = append(targets, configTarget{Addr: addr, Client: c, close: func() { _ = c.Close() }})
		}
		return targets, true
	}

	base := cc.Options()
	// 不用基础集群客户端取节点列表：命令可能被路由到失联节点并阻塞到读超时
	addrs, ok := discoverClusterAddrs(ctx, base.Addrs, base)
	if !ok || len(addrs) == 0 {
		return nil, false
	}

	targets := make([]configTarget, 0, len(addrs))
	for _, addr := range addrs {
		c := redis.NewClient(configClusterNodeOptions(addr, base))
		targets = append(targets, configTarget{Addr: addr, Client: c, close: func() { _ = c.Close() }})
	}
	return targets, true
}

// ConfigOverview 返回各节点配置的合并视图。
func (b *browserService) ConfigOverview(name string) (resp types.JSResp) {
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	ctx := item.ctx

	targets, merged := b.configTargets(ctx, item.client, name)
	if len(targets) == 0 {
		resp.Msg = "no reachable node"
		return
	}
	defer func() {
		for _, t := range targets {
			if t.close != nil {
				t.close()
			}
		}
	}()

	views := map[string]map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t configTarget) {
			defer wg.Done()
			raw, err := t.Client.Do(ctx, "CONFIG", "GET", "*").Result()
			if err != nil {
				return
			}
			pairs := parseConfigPairs(raw)
			mu.Lock()
			views[t.Addr] = pairs
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	if len(views) == 0 {
		resp.Msg = "get config fail"
		return
	}

	resp.Success = true
	resp.Data = buildConfigData(views, targets, merged)
	return
}

// buildConfigData 汇总各节点视图；读取失败的节点保留在 Unavailable 中，
// 并把视图标记为不完整，避免"没读到"被显示成"一致"。
func buildConfigData(views map[string]map[string]string, targets []configTarget, merged bool) configData {
	addrs := make([]string, 0, len(views))
	for addr := range views {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	unavailable := make([]string, 0)
	for _, target := range targets {
		if _, ok := views[target.Addr]; !ok {
			unavailable = append(unavailable, target.Addr)
		}
	}
	sort.Strings(unavailable)

	return configData{
		Params:      mergeConfigValues(views),
		Nodes:       addrs,
		Unavailable: unavailable,
		Merged:      merged && len(unavailable) == 0,
	}
}

// ConfigSet 修改单个参数：写入所有节点，成功后自动尝试 CONFIG REWRITE。
func (b *browserService) ConfigSet(name, key, value string) (resp types.JSResp) {
	key = strings.TrimSpace(key)
	if key == "" {
		resp.Msg = "key is required"
		return
	}
	if sensitiveConfigKeys[key] {
		resp.Msg = "this parameter is read-only in KVPilot, change it via CLI or deployment config"
		return
	}
	if immutableConfigKeys[key] {
		resp.Msg = "this parameter cannot be changed at runtime"
		return
	}

	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	ctx := item.ctx

	targets, _ := b.configTargets(ctx, item.client, name)
	if len(targets) == 0 {
		resp.Msg = "no reachable node"
		return
	}
	defer func() {
		for _, t := range targets {
			if t.close != nil {
				t.close()
			}
		}
	}()

	results := make([]configNodeResult, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(idx int, t configTarget) {
			defer wg.Done()
			result := configNodeResult{Addr: t.Addr}
			if err := t.Client.Do(ctx, "CONFIG", "SET", key, value).Err(); err != nil {
				result.Msg = err.Error()
				results[idx] = result
				return
			}
			result.Set = true
			// 有配置文件时自动落盘；没有配置文件属于正常情况，只需告知用户
			switch err := t.Client.Do(ctx, "CONFIG", "REWRITE").Err(); {
			case err == nil:
				result.Rewrite = "persisted"
			case strings.Contains(err.Error(), "without a config file"):
				result.Rewrite = "memory-only"
			default:
				result.Rewrite = "failed"
				result.Msg = err.Error()
			}
			results[idx] = result
		}(i, target)
	}
	wg.Wait()

	applied, failed, memoryOnly := 0, 0, 0
	for _, r := range results {
		if r.Set {
			applied++
		} else {
			failed++
		}
		if r.Rewrite == "memory-only" {
			memoryOnly++
		}
	}
	if applied == 0 {
		resp.Msg = "apply config fail"
		resp.Data = map[string]any{"results": results}
		return
	}

	resp.Success = true
	resp.Data = map[string]any{
		"results":    results,
		"applied":    applied,
		"failed":     failed,
		"memoryOnly": memoryOnly,
		"nodes":      len(results),
	}
	return
}
