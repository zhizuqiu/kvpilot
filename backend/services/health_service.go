package services

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

// healthItem 是一条巡检结果。
type healthItem struct {
	Node     string            `json:"node"`
	Rule     string            `json:"rule"`
	Severity string            `json:"severity"` // ok / warn / error
	Values   map[string]string `json:"values,omitempty"`
}

// healthData 是巡检接口的返回体。
type healthData struct {
	Nodes       []string       `json:"nodes"`
	Unavailable []string       `json:"unavailable,omitempty"`
	Items       []healthItem   `json:"items"`
	Counts      map[string]int `json:"counts"`
	Mode        string         `json:"mode"`
	ElapsedMS   int64          `json:"elapsedMs"`
}

// checkValue 读取 INFO 中的字段，缺失时返回空字符串。
func checkValue(info map[string]map[string]string, section, field string) string {
	if sectionInfo, ok := info[section]; ok {
		return sectionInfo[field]
	}
	return ""
}

func checkFloat(value string) float64 {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return number
}

func checkInt(value string) int64 {
	number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return number
}

// evaluateHealth 按只读规则评估单个节点，返回每条规则的结果（包含正常的规则）。
func evaluateHealth(node string, info map[string]map[string]string, config map[string]string, mode string) []healthItem {
	items := make([]healthItem, 0, 12)
	add := func(rule, severity string, values map[string]string) {
		items = append(items, healthItem{Node: node, Rule: rule, Severity: severity, Values: values})
	}

	// 1. 内存碎片率
	ratio := checkFloat(checkValue(info, "Memory", "mem_fragmentation_ratio"))
	severity := "ok"
	if ratio >= 2 {
		severity = "error"
	} else if ratio >= 1.5 {
		severity = "warn"
	}
	add("fragmentation", severity, map[string]string{"ratio": formatFloat(ratio)})

	// 2. maxmemory 使用率
	used := checkInt(checkValue(info, "Memory", "used_memory"))
	maxmemory := checkInt(config["maxmemory"])
	switch {
	case maxmemory <= 0:
		add("maxmemory", "warn", map[string]string{"used": formatBytesHuman(used)})
	case float64(used)/float64(maxmemory) >= 0.95:
		add("maxmemory", "error", map[string]string{
			"used": formatBytesHuman(used), "max": formatBytesHuman(maxmemory),
			"percent": formatPercent(used, maxmemory),
		})
	case float64(used)/float64(maxmemory) >= 0.85:
		add("maxmemory", "warn", map[string]string{
			"used": formatBytesHuman(used), "max": formatBytesHuman(maxmemory),
			"percent": formatPercent(used, maxmemory),
		})
	default:
		add("maxmemory", "ok", map[string]string{
			"used": formatBytesHuman(used), "max": formatBytesHuman(maxmemory),
			"percent": formatPercent(used, maxmemory),
		})
	}

	// 3. 内存淘汰：上限已设置却使用 noeviction 时写满即报错
	policy := config["maxmemory-policy"]
	if maxmemory > 0 && policy == "noeviction" {
		add("policy", "warn", map[string]string{"policy": policy})
	} else {
		add("policy", "ok", map[string]string{"policy": policy})
	}
	evicted := checkInt(checkValue(info, "Stats", "evicted_keys"))
	if evicted > 0 {
		add("evicted", "warn", map[string]string{"count": strconv.FormatInt(evicted, 10)})
	} else {
		add("evicted", "ok", nil)
	}

	// 4. 持久化状态
	rdbStatus := checkValue(info, "Persistence", "rdb_last_bgsave_status")
	aofEnabled := checkValue(info, "Persistence", "aof_enabled") == "1"
	aofStatus := checkValue(info, "Persistence", "aof_last_write_status")
	persistenceSeverity := "ok"
	persistenceValues := map[string]string{"rdb": rdbStatus, "aof": "off"}
	if aofEnabled {
		persistenceValues["aof"] = aofStatus
		if aofStatus != "ok" {
			persistenceSeverity = "error"
		}
	}
	if rdbStatus != "" && rdbStatus != "ok" {
		persistenceSeverity = "error"
	}
	add("persistence", persistenceSeverity, persistenceValues)

	// 5. 复制链路
	role := checkValue(info, "Replication", "role")
	link := checkValue(info, "Replication", "master_link_status")
	lag := checkInt(checkValue(info, "Replication", "master_last_io_seconds_ago"))
	slaves := checkInt(checkValue(info, "Replication", "connected_slaves"))
	replicationSeverity := "ok"
	replicationValues := map[string]string{
		"role":   role,
		"link":   link,
		"lag":    strconv.FormatInt(lag, 10),
		"slaves": strconv.FormatInt(slaves, 10),
	}
	switch {
	case role == "slave" && link != "up":
		replicationSeverity = "error"
	case role == "slave" && lag > 10:
		replicationSeverity = "warn"
	case role == "master" && mode != "standalone" && slaves == 0:
		replicationSeverity = "warn"
	}
	add("replication", replicationSeverity, replicationValues)

	// 6. 阻塞客户端
	blocked := checkInt(checkValue(info, "Clients", "blocked_clients"))
	if blocked > 0 {
		add("blocked", "warn", map[string]string{"count": strconv.FormatInt(blocked, 10)})
	} else {
		add("blocked", "ok", nil)
	}

	// 7. 慢日志配置
	slowlogThreshold := config["slowlog-log-slower-than"]
	if slowlogThreshold == "-1" {
		add("slowlog", "warn", map[string]string{"threshold": slowlogThreshold})
	} else {
		add("slowlog", "ok", map[string]string{"threshold": slowlogThreshold})
	}

	// 8. 未设置密码 / 保护模式关闭
	requirepass := config["requirepass"]
	protectedMode := config["protected-mode"]
	authSeverity := "ok"
	if requirepass == "" {
		authSeverity = "warn"
		if protectedMode == "no" {
			authSeverity = "error"
		}
	}
	add("auth", authSeverity, map[string]string{"protected": protectedMode, "password": boolText(requirepass != "")})

	// 9. 持久化配置：RDB 与 AOF 都没有开启时提示
	saveRule := config["save"]
	appendonly := config["appendonly"]
	persistenceConfigSeverity := "ok"
	if saveRule == "" && appendonly != "yes" {
		persistenceConfigSeverity = "warn"
	}
	add("persistence_config", persistenceConfigSeverity, map[string]string{"save": saveRule, "appendonly": appendonly})

	// 10. 集群状态（仅集群模式）
	if mode == "cluster" {
		state := checkValue(info, "Cluster", "cluster_state")
		clusterSeverity := "ok"
		if state != "" && state != "ok" {
			clusterSeverity = "error"
		}
		add("cluster", clusterSeverity, map[string]string{"state": state})
	}

	return items
}

func boolText(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatPercent(used, max int64) string {
	if max <= 0 {
		return "0"
	}
	return strconv.FormatInt(used*100/max, 10)
}

func formatBytesHuman(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatInt(bytes, 10) + "B"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 1, 64) + string(units[exp]) + "B"
}

// CheckHealth 对连接涉及的数据节点执行只读巡检。
func (b *browserService) CheckHealth(server string) (resp types.JSResp) {
	started := time.Now()

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

	mode := ""
	if topo := b.Topology(server); topo.Success {
		if data, ok := topo.Data.(topologyData); ok {
			mode = data.Mode
		}
	}

	configKeys := []string{
		"maxmemory", "maxmemory-policy", "appendonly", "save",
		"protected-mode", "slowlog-log-slower-than", "requirepass",
	}

	data := healthData{
		Nodes:  make([]string, 0, len(targets)),
		Items:  make([]healthItem, 0, len(targets)*10),
		Counts: map[string]int{"ok": 0, "warn": 0, "error": 0},
		Mode:   mode,
	}

	for _, target := range targets {
		client, ok := target.Client.(*redis.Client)
		if !ok {
			continue
		}

		infoText, err := client.Info(item.ctx).Result()
		if err != nil {
			data.Unavailable = append(data.Unavailable, target.Addr)
			continue
		}
		info := b.parseInfo(infoText)

		config := map[string]string{}
		pipe := client.Pipeline()
		cmds := make(map[string]*redis.MapStringStringCmd, len(configKeys))
		for _, key := range configKeys {
			cmds[key] = pipe.ConfigGet(item.ctx, key)
		}
		_, _ = pipe.Exec(item.ctx)
		for key, cmd := range cmds {
			if values, err := cmd.Result(); err == nil {
				config[key] = values[key]
			}
		}
		// 密码只用于判断是否为空，具体值不保留
		if config["requirepass"] != "" {
			config["requirepass"] = "set"
		}

		data.Nodes = append(data.Nodes, target.Addr)
		data.Items = append(data.Items, evaluateHealth(target.Addr, info, config, mode)...)
	}

	sort.Strings(data.Nodes)
	sort.Strings(data.Unavailable)
	severityRank := func(severity string) int {
		switch severity {
		case "error":
			return 0
		case "warn":
			return 1
		default:
			return 2
		}
	}
	sort.Slice(data.Items, func(i, j int) bool {
		if data.Items[i].Severity != data.Items[j].Severity {
			return severityRank(data.Items[i].Severity) < severityRank(data.Items[j].Severity)
		}
		if data.Items[i].Node != data.Items[j].Node {
			return data.Items[i].Node < data.Items[j].Node
		}
		return data.Items[i].Rule < data.Items[j].Rule
	})
	for _, item := range data.Items {
		data.Counts[item.Severity]++
	}

	data.ElapsedMS = time.Since(started).Milliseconds()
	resp.Success = true
	resp.Data = data
	return
}
