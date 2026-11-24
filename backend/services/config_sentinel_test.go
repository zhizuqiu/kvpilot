package services

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"tinyrdm/backend/storage"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
	"github.com/vrischmann/userdir"
)

// newSentinelConfigService 注册一个真实的哨兵连接，并返回指向主节点的 browserService。
//
// 配置比较需要 Connection().getConnection(name) 命中哨兵信息，因此这里把全局连接
// 注册表替换为指向临时 HOME 的实例：既走真实的 SENTINEL MASTERS / REPLICAS 分支，
// 又不会把测试连接写入本机真实的 connections.yaml。
func newSentinelConfigService(t *testing.T) (*browserService, string) {
	t.Helper()

	sentinelAddr := os.Getenv("KVPILOT_TEST_REDIS_SENTINEL_ADDR")
	if sentinelAddr == "" {
		t.Skip("未设置 KVPILOT_TEST_REDIS_SENTINEL_ADDR，跳过哨兵配置测试")
	}
	masterName := os.Getenv("KVPILOT_TEST_REDIS_SENTINEL_MASTER")
	if masterName == "" {
		masterName = "mymaster"
	}
	password := os.Getenv("KVPILOT_TEST_REDIS_SENTINEL_PASSWORD")
	if password == "" {
		password = os.Getenv("KVPILOT_TEST_REDIS_PASSWORD")
	}

	host, portText, err := net.SplitHostPort(sentinelAddr)
	if err != nil {
		t.Fatalf("哨兵地址格式错误: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("哨兵端口格式错误: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	// 存储层只做单层 Mkdir，先把平台对应的配置根目录建出来
	if err := os.MkdirAll(userdir.GetConfigHome(), 0o700); err != nil {
		t.Fatalf("创建临时配置目录失败: %v", err)
	}
	prev := connection
	connection = &connectionService{conns: storage.NewConnections()}
	t.Cleanup(func() { connection = prev })

	name := "kvpilot_sentinel_test"
	cfg := types.ConnectionConfig{
		Name:         name,
		Network:      "tcp",
		Addr:         host,
		Port:         port,
		Password:     password,
		KeySeparator: ":",
		Sentinel: types.ConnectionSentinel{
			Enable:   true,
			Master:   masterName,
			Password: password,
		},
	}
	if resp := Connection().SaveConnection("", cfg); !resp.Success {
		t.Fatalf("注册哨兵连接失败: %s", resp.Msg)
	}

	// 与 createRedisClient 的哨兵分支一致：先问出主节点，再把客户端指向主节点
	sentinel := redis.NewSentinelClient(&redis.Options{
		Addr:        sentinelAddr,
		Password:    password,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
		Protocol:    2,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	masterAddr, err := sentinel.GetMasterAddrByName(ctx, masterName).Result()
	cancel()
	sentinel.Close()
	if err != nil {
		t.Fatalf("获取哨兵主节点失败: %v", err)
	}
	if len(masterAddr) < 2 {
		t.Fatalf("哨兵返回的主节点地址不完整: %v", masterAddr)
	}

	client := redis.NewClient(&redis.Options{
		Addr:        net.JoinHostPort(masterAddr[0], masterAddr[1]),
		Password:    password,
		DialTimeout: 3 * time.Second,
		ReadTimeout: 3 * time.Second,
	})
	t.Cleanup(func() { _ = client.Close() })

	svcCtx := context.Background()
	svc := &browserService{ctx: svcCtx, connMap: map[string]*connectionItem{}}
	svc.connMap[name] = &connectionItem{client: client, ctx: svcCtx, db: 0}
	return svc, name
}

// 验证哨兵模式下配置读取走 SENTINEL REPLICAS：数据节点覆盖主从、排除哨兵。
func TestIntegrationConfigSentinelOverview(t *testing.T) {
	svc, name := newSentinelConfigService(t)

	topoResp := svc.Topology(name)
	if !topoResp.Success {
		t.Fatalf("Topology 失败: %s", topoResp.Msg)
	}
	topo, ok := topoResp.Data.(topologyData)
	if !ok {
		t.Fatalf("拓扑返回类型错误: %T", topoResp.Data)
	}
	if topo.Mode != "sentinel" {
		t.Fatalf("拓扑模式 = %q, want sentinel", topo.Mode)
	}

	sentinelAddrs := map[string]bool{}
	dataAddrs := map[string]bool{}
	for _, n := range topo.Nodes {
		switch n.Role {
		case "sentinel":
			sentinelAddrs[n.Addr] = true
		case "master", "replica":
			dataAddrs[n.Addr] = true
		}
	}
	if len(sentinelAddrs) == 0 {
		t.Fatal("拓扑应包含哨兵节点")
	}
	if len(dataAddrs) < 2 {
		t.Fatalf("哨兵应提供主从至少两个数据节点, got %v", dataAddrs)
	}

	resp := svc.ConfigOverview(name)
	if !resp.Success {
		t.Fatalf("ConfigOverview 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(configData)
	if !ok {
		t.Fatalf("配置返回类型错误: %T", resp.Data)
	}
	if !data.Merged {
		t.Fatal("哨兵模式应返回完整视图")
	}

	// 配置覆盖的节点必须与拓扑中的主从节点一致，且不含哨兵
	got := map[string]bool{}
	for _, addr := range data.Nodes {
		if sentinelAddrs[addr] {
			t.Fatalf("配置节点不应包含哨兵地址: %s", addr)
		}
		got[addr] = true
	}
	for addr := range dataAddrs {
		if !got[addr] {
			t.Fatalf("配置未覆盖数据节点 %s, got %v", addr, data.Nodes)
		}
	}

	// 逐节点取值依赖节点列表，退出哨兵路径后仍应完整
	var maxmemory *configParam
	for i := range data.Params {
		if data.Params[i].Key == "maxmemory" {
			maxmemory = &data.Params[i]
		}
	}
	if maxmemory == nil {
		t.Fatal("缺少 maxmemory 参数")
	}
	if maxmemory.Nodes != len(data.Nodes) {
		t.Fatalf("maxmemory 节点数 = %d, want %d", maxmemory.Nodes, len(data.Nodes))
	}
}

// 验证哨兵模式下写入覆盖主节点与从节点，而不是只写当前主节点。
func TestIntegrationConfigSentinelSet(t *testing.T) {
	svc, name := newSentinelConfigService(t)

	before := svc.ConfigOverview(name)
	if !before.Success {
		t.Fatalf("读取初始配置失败: %s", before.Msg)
	}
	var original string
	for _, p := range before.Data.(configData).Params {
		if p.Key == "maxmemory" {
			original = p.Value
		}
	}
	if original == "" {
		t.Fatal("未读到 maxmemory 原值")
	}
	t.Cleanup(func() { svc.ConfigSet(name, "maxmemory", original) })

	resp := svc.ConfigSet(name, "maxmemory", "91mb")
	if !resp.Success {
		t.Fatalf("ConfigSet 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("返回类型不正确: %T", resp.Data)
	}
	applied, _ := data["applied"].(int)
	total, _ := data["nodes"].(int)
	if total < 2 {
		t.Fatalf("哨兵模式应写入主从多个节点, got %d", total)
	}
	if applied != total {
		t.Fatalf("应写入全部数据节点: applied=%d nodes=%d results=%+v", applied, total, data["results"])
	}

	results, _ := data["results"].([]configNodeResult)
	written := map[string]bool{}
	for _, r := range results {
		if !r.Set {
			t.Fatalf("节点 %s 写入失败: %s", r.Addr, r.Msg)
		}
		written[r.Addr] = true
	}

	topoResp := svc.Topology(name)
	if !topoResp.Success {
		t.Fatalf("Topology 失败: %s", topoResp.Msg)
	}
	for _, n := range topoResp.Data.(topologyData).Nodes {
		if n.Role != "master" && n.Role != "replica" {
			continue
		}
		if !written[n.Addr] {
			t.Fatalf("数据节点 %s 未被写入: %+v", n.Addr, results)
		}
	}
}
