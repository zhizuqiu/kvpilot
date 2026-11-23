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
	Merged bool          `json:"merged"`
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

// configTargets 返回需要操作的节点列表。
// 集群下包含所有主从节点，因为 CONFIG 不会随复制传播。
func (b *browserService) configTargets(ctx context.Context, client redis.UniversalClient) ([]configTarget, bool) {
	cc, ok := client.(*redis.ClusterClient)
	if !ok {
		addr := ""
		if c, ok := client.(*redis.Client); ok {
			addr = c.Options().Addr
		}
		return []configTarget{{Addr: addr, Client: client}}, false
	}

	text, err := cc.Do(ctx, "CLUSTER", "NODES").Text()
	if err != nil {
		return nil, false
	}
	nodes := parseClusterNodes(text, "")
	addrs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		addrs = append(addrs, n.Addr)
	}
	if len(addrs) == 0 {
		return nil, false
	}

	base := cc.Options()
	targets := make([]configTarget, 0, len(addrs))
	for _, addr := range addrs {
		opt := &redis.Options{
			Addr:         addr,
			Username:     base.Username,
			Password:     base.Password,
			DB:           0,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
			MaxRetries:   1,
			Protocol:     base.Protocol,
			TLSConfig:    base.TLSConfig,
			Dialer:       base.Dialer,
		}
		c := redis.NewClient(opt)
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

	targets, merged := b.configTargets(ctx, item.client)
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

	addrs := make([]string, 0, len(views))
	for addr := range views {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	resp.Success = true
	resp.Data = configData{
		Params: mergeConfigValues(views),
		Nodes:  addrs,
		Merged: merged,
	}
	return
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

	targets, _ := b.configTargets(ctx, item.client)
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
