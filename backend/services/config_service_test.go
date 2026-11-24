package services

import (
	"strings"
	"testing"
)

// 验证 CONFIG GET 的两种返回形态都能规整成 key -> value。
func TestParseConfigPairs(t *testing.T) {
	resp2 := []interface{}{"maxmemory", "0", "maxmemory-policy", "noeviction"}
	got := parseConfigPairs(resp2)
	if got["maxmemory"] != "0" || got["maxmemory-policy"] != "noeviction" {
		t.Fatalf("RESP2 解析不正确: %+v", got)
	}

	resp3 := map[interface{}]interface{}{"appendonly": "no", "timeout": "0"}
	got = parseConfigPairs(resp3)
	if got["appendonly"] != "no" || got["timeout"] != "0" {
		t.Fatalf("RESP3 解析不正确: %+v", got)
	}

	// 奇数长度与非字符串元素应被安全跳过
	got = parseConfigPairs([]interface{}{"a", "b", "c"})
	if len(got) != 1 || got["a"] != "b" {
		t.Fatalf("异常输入处理不正确: %+v", got)
	}
	got = parseConfigPairs([]interface{}{1, 2})
	if len(got) != 0 {
		t.Fatalf("非字符串键值应被忽略: %+v", got)
	}
	if got = parseConfigPairs(nil); len(got) != 0 {
		t.Fatalf("nil 应返回空 map: %+v", got)
	}
}

// 验证参数分类：更具体的前缀优先，未知参数归入 other。
func TestConfigCategory(t *testing.T) {
	cases := map[string]string{
		"maxmemory":               "memory",
		"maxmemory-policy":        "memory",
		"appendonly":              "persistence",
		"appendfsync":             "persistence",
		"save":                    "persistence",
		"repl-backlog-size":       "replication",
		"replica-read-only":       "replication",
		"cluster-node-timeout":    "cluster",
		"slowlog-log-slower-than": "slowlog",
		"requirepass":             "security",
		"tls-port":                "security",
		"bind":                    "network",
		"maxclients":              "clients",
		"lua-time-limit":          "lua",
		"hz":                      "performance",
		"unknown-parameter":       "other",
	}
	for key, want := range cases {
		if got := configCategory(key); got != want {
			t.Fatalf("configCategory(%q) = %q, want %q", key, got, want)
		}
	}
}

// 验证取值类型推断，前端据此选择输入控件。
func TestConfigValueType(t *testing.T) {
	cases := map[string]string{
		"yes":         "bool",
		"no":          "bool",
		"0":           "int",
		"1024":        "int",
		"100mb":       "memory",
		"1GB":         "memory",
		"allkeys-lru": "string",
		"":            "string",
	}
	for value, want := range cases {
		if got := configValueType(value); got != want {
			t.Fatalf("configValueType(%q) = %q, want %q", value, got, want)
		}
	}
}

// 验证多节点合并：不一致时保留每个节点的值，敏感参数脱敏且不暴露差异。
func TestMergeConfigValues(t *testing.T) {
	views := map[string]map[string]string{
		"10.0.0.2:6379": {
			"maxmemory":   "0",
			"timeout":     "0",
			"requirepass": "secret",
		},
		"10.0.0.1:6379": {
			"maxmemory":   "100mb",
			"timeout":     "0",
			"requirepass": "secret",
		},
	}

	params := mergeConfigValues(views)
	byKey := map[string]configParam{}
	for _, p := range params {
		byKey[p.Key] = p
	}

	// 代表性值取地址最小的节点（10.0.0.1）
	if byKey["maxmemory"].Value != "100mb" {
		t.Fatalf("代表值应取首个节点, got %q", byKey["maxmemory"].Value)
	}
	if byKey["maxmemory"].Consistent {
		t.Fatal("maxmemory 节点间不一致，应标记为不一致")
	}
	if len(byKey["maxmemory"].Values) != 2 {
		t.Fatalf("不一致参数应携带各节点取值, got %+v", byKey["maxmemory"].Values)
	}
	if byKey["maxmemory"].Nodes != 2 {
		t.Fatalf("节点数 = %d, want 2", byKey["maxmemory"].Nodes)
	}

	if !byKey["timeout"].Consistent {
		t.Fatal("timeout 各节点一致，不应标记为不一致")
	}

	// 敏感参数：值被脱敏，且不返回逐节点差异
	if byKey["requirepass"].Value != "***" {
		t.Fatalf("敏感参数应脱敏, got %q", byKey["requirepass"].Value)
	}
	if !byKey["requirepass"].Sensitive {
		t.Fatal("requirepass 应标记为敏感")
	}
	if byKey["requirepass"].Values != nil {
		t.Fatalf("敏感参数不应返回逐节点取值: %+v", byKey["requirepass"].Values)
	}
}

// 验证不可变参数被标记，前端据此禁用编辑。
func TestMergeConfigValuesImmutableFlag(t *testing.T) {
	params := mergeConfigValues(map[string]map[string]string{
		"10.0.0.1:6379": {"databases": "16", "maxmemory": "0"},
	})
	byKey := map[string]configParam{}
	for _, p := range params {
		byKey[p.Key] = p
	}
	if !byKey["databases"].Immutable {
		t.Fatal("databases 应标记为不可变")
	}
	if byKey["maxmemory"].Immutable {
		t.Fatal("maxmemory 可运行时修改，不应标记为不可变")
	}
}

// 验证敏感参数与不可变参数在写入前被直接拒绝，不会发起网络请求。
func TestConfigSetValidation(t *testing.T) {
	svc := &browserService{}

	if resp := svc.ConfigSet("any", "requirepass", "newpass"); resp.Success ||
		!strings.Contains(resp.Msg, "read-only") {
		t.Fatalf("敏感参数应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}

	if resp := svc.ConfigSet("any", "databases", "32"); resp.Success ||
		!strings.Contains(resp.Msg, "cannot be changed at runtime") {
		t.Fatalf("不可变参数应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}

	if resp := svc.ConfigSet("any", " ", "1"); resp.Success || resp.Msg != "key is required" {
		t.Fatalf("空参数名应被拒绝, got success=%v msg=%q", resp.Success, resp.Msg)
	}
}

// 验证从拓扑中提取数据节点：包含主从、排除哨兵、去重并排序。
func TestConfigTopologyAddrs(t *testing.T) {
	data := topologyData{Nodes: []topologyNode{
		{Addr: "10.0.0.2:6379", Role: "replica"},
		{Addr: "10.0.0.1:6379", Role: "master"},
		{Addr: "10.0.0.2:6379", Role: "replica"},
		{Addr: "10.0.0.9:26379", Role: "sentinel"},
	}}

	addrs := configTopologyAddrs(data)
	want := []string{"10.0.0.1:6379", "10.0.0.2:6379"}
	if len(addrs) != len(want) {
		t.Fatalf("addrs = %v, want %v", addrs, want)
	}
	for i := range want {
		if addrs[i] != want[i] {
			t.Fatalf("addrs = %v, want %v", addrs, want)
		}
	}
}
