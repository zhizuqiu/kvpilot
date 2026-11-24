package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
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

// 验证读取失败的节点会出现在未响应列表里，而不是被静默丢弃。
func TestBuildConfigDataMarksUnavailable(t *testing.T) {
	views := map[string]map[string]string{
		"10.0.0.1:6379": {"maxmemory": "0"},
	}
	targets := []configTarget{
		{Addr: "10.0.0.1:6379"},
		{Addr: "10.0.0.2:6379"},
	}

	data := buildConfigData(views, targets, true)
	if len(data.Nodes) != 1 || data.Nodes[0] != "10.0.0.1:6379" {
		t.Fatalf("Nodes = %v, 应只包含读取成功的节点", data.Nodes)
	}
	if len(data.Unavailable) != 1 || data.Unavailable[0] != "10.0.0.2:6379" {
		t.Fatalf("Unavailable = %v, 应保留读取失败的节点", data.Unavailable)
	}
	if data.Merged {
		t.Fatal("存在未响应节点时不应标记为完整视图")
	}
}

// 验证全部节点可读时仍为完整视图，且未响应列表为空。
func TestBuildConfigDataAllReadable(t *testing.T) {
	views := map[string]map[string]string{
		"10.0.0.1:6379": {"maxmemory": "0"},
		"10.0.0.2:6379": {"maxmemory": "0"},
	}
	targets := []configTarget{
		{Addr: "10.0.0.2:6379"},
		{Addr: "10.0.0.1:6379"},
	}

	data := buildConfigData(views, targets, true)
	if len(data.Unavailable) != 0 {
		t.Fatalf("Unavailable = %v, want 空", data.Unavailable)
	}
	if !data.Merged {
		t.Fatal("全部可读时应保持完整视图")
	}
	if len(data.Nodes) != 2 || data.Nodes[0] != "10.0.0.1:6379" {
		t.Fatalf("Nodes = %v, 应按地址排序", data.Nodes)
	}
}

// 验证分片超限导致的降级标记不会因为节点全部可读而被覆盖。
func TestBuildConfigDataKeepsIncompleteFlag(t *testing.T) {
	views := map[string]map[string]string{
		"10.0.0.1:6379": {"maxmemory": "0"},
	}
	targets := []configTarget{{Addr: "10.0.0.1:6379"}}

	data := buildConfigData(views, targets, false)
	if data.Merged {
		t.Fatal("拓扑本身不完整时应保持 merged=false")
	}
	if len(data.Unavailable) != 0 {
		t.Fatalf("Unavailable = %v, want 空", data.Unavailable)
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

// 验证单节点客户端会快速失败：拨号次数与超时受控，
// 否则一个不可达节点就能把整页配置加载拖到几十秒。
func TestConfigNodeOptionsFastFail(t *testing.T) {
	opt := configNodeOptions("10.0.0.2:6379", &redis.Options{
		Username: "u",
		Password: "p",
		Protocol: 2,
	})

	if opt.DialTimeout != configNodeDialTimeout {
		t.Fatalf("DialTimeout = %v, want %v", opt.DialTimeout, configNodeDialTimeout)
	}
	if opt.DialerRetries != 2 {
		t.Fatalf("DialerRetries = %d, 不可达节点不应长时间重试", opt.DialerRetries)
	}
	if opt.Addr != "10.0.0.2:6379" || opt.Username != "u" || opt.Password != "p" || opt.Protocol != 2 {
		t.Fatalf("应继承连接凭据与协议: %+v", opt)
	}
}

// 验证种子节点读取失败时会继续尝试下一个，并返回解析出的全部地址。
func TestClusterAddrsFromSeedsFallsBack(t *testing.T) {
	text := "id1 10.0.0.1:6379@16379 myself,master - 0 0 1 connected 0-5460\n" +
		"id2 10.0.0.2:6379@16379 master - 0 0 2 connected 5461-10922\n"
	var calls []string
	read := func(ctx context.Context, addr string) (string, error) {
		calls = append(calls, addr)
		if addr == "10.0.0.9:6379" {
			return "", errors.New("dial tcp: no route to host")
		}
		return text, nil
	}

	addrs, ok := clusterAddrsFromSeeds(context.Background(),
		[]string{"10.0.0.9:6379", "10.0.0.1:6379", "10.0.0.2:6379"}, read)
	if !ok {
		t.Fatal("应能从一个可达种子拿到节点列表")
	}
	if len(addrs) != 2 || addrs[0] != "10.0.0.1:6379" || addrs[1] != "10.0.0.2:6379" {
		t.Fatalf("addrs = %v", addrs)
	}
	// 拿到结果后不应继续访问后续种子
	if len(calls) != 2 {
		t.Fatalf("种子访问次数 = %d, want 2 (%v)", len(calls), calls)
	}
}

// 验证所有种子都不可读时返回失败，由调用方给出"无可用节点"。
func TestClusterAddrsFromSeedsAllFail(t *testing.T) {
	read := func(ctx context.Context, addr string) (string, error) {
		return "", errors.New("connection refused")
	}
	addrs, ok := clusterAddrsFromSeeds(context.Background(), []string{"10.0.0.1:6379"}, read)
	if ok || addrs != nil {
		t.Fatalf("全部失败时应返回 false, got ok=%v addrs=%v", ok, addrs)
	}
}

// 验证返回空节点列表的种子视为不可用，会继续尝试下一个种子。
func TestClusterAddrsFromSeedsSkipsEmptyList(t *testing.T) {
	text := "id1 10.0.0.1:6379@16379 myself,master - 0 0 1 connected 0-5460\n"
	read := func(ctx context.Context, addr string) (string, error) {
		if addr == "10.0.0.1:6379" {
			return "", nil
		}
		return text, nil
	}
	addrs, ok := clusterAddrsFromSeeds(context.Background(),
		[]string{"10.0.0.1:6379", "10.0.0.2:6379"}, read)
	if !ok || len(addrs) != 1 || addrs[0] != "10.0.0.1:6379" {
		t.Fatalf("应跳过空列表的种子, got ok=%v addrs=%v", ok, addrs)
	}
}

// 验证重复的种子只访问一次，避免同一节点被反复拨号。
func TestClusterAddrsFromSeedsDeduplicates(t *testing.T) {
	var calls int
	read := func(ctx context.Context, addr string) (string, error) {
		calls++
		return "", errors.New("connection refused")
	}
	clusterAddrsFromSeeds(context.Background(),
		[]string{"10.0.0.1:6379", "10.0.0.1:6379", "10.0.0.1:6379"}, read)
	if calls != 1 {
		t.Fatalf("重复种子访问次数 = %d, want 1", calls)
	}
}

// 验证扫描类任务只保留主节点，并关闭被剔除节点的连接。
func TestFilterMasterTargets(t *testing.T) {
	closed := map[string]int{}
	makeTarget := func(addr string) configTarget {
		return configTarget{Addr: addr, close: func() { closed[addr]++ }}
	}
	targets := []configTarget{
		makeTarget("10.0.0.1:6379"),
		makeTarget("10.0.0.2:6379"),
		makeTarget("10.0.0.3:6379"),
	}
	masters := map[string]bool{"10.0.0.1:6379": true, "10.0.0.3:6379": true}

	kept := filterMasterTargets(targets, masters)
	if len(kept) != 2 || kept[0].Addr != "10.0.0.1:6379" || kept[1].Addr != "10.0.0.3:6379" {
		t.Fatalf("kept = %+v", kept)
	}
	if closed["10.0.0.2:6379"] != 1 {
		t.Fatalf("被剔除节点应关闭连接: %+v", closed)
	}
	if closed["10.0.0.1:6379"] != 0 || closed["10.0.0.3:6379"] != 0 {
		t.Fatalf("保留节点不应被关闭: %+v", closed)
	}

	// 没有任何地址匹配时保持原样，避免拓扑异常导致任务无法执行
	same := filterMasterTargets(targets, map[string]bool{"10.9.9.9:6379": true})
	if len(same) != 3 {
		t.Fatalf("无匹配时应原样返回, got %d", len(same))
	}
}
