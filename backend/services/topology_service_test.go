package services

import (
	"reflect"
	"strings"
	"testing"
)

// 验证 INFO replication 中 slaveN 行的键值解析，这是主从拓扑还原的基础。
func TestParseSlaveLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want map[string]string
	}{
		{
			name: "完整行",
			line: "ip=192.168.1.118,port=16381,state=online,offset=322,lag=0",
			want: map[string]string{
				"ip": "192.168.1.118", "port": "16381",
				"state": "online", "offset": "322", "lag": "0",
			},
		},
		{
			name: "字段周围有空格时需裁剪",
			line: "ip=10.0.0.2, port=6379 ,state=online",
			want: map[string]string{"ip": "10.0.0.2", "port": "6379", "state": "online"},
		},
		{
			name: "缺少等号的片段被忽略，不产生空键",
			line: "ip=10.0.0.2,broken,port=6379",
			want: map[string]string{"ip": "10.0.0.2", "port": "6379"},
		},
		{
			name: "空行返回空映射",
			line: "",
			want: map[string]string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseSlaveLine(c.line); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseSlaveLine(%q) = %v, want %v", c.line, got, c.want)
			}
		})
	}
}

// 验证以主节点视角生成主从拓扑：当前节点在前，从节点带偏移与延迟。
func TestReplicationTopology(t *testing.T) {
	info := map[string]map[string]string{
		"Replication": {
			"role":               "master",
			"master_repl_offset": "3553",
			"connected_slaves":   "2",
			"slave0":             "ip=192.168.1.118,port=16381,state=online,offset=322,lag=0",
			"slave1":             "ip=192.168.1.118,port=16382,state=online,offset=300,lag=1",
		},
	}

	nodes := replicationTopology(info, "192.168.1.118:16380")

	if len(nodes) != 3 {
		t.Fatalf("节点数 = %d, want 3", len(nodes))
	}
	if nodes[0].Role != "master" || !nodes[0].Self || nodes[0].Offset != 3553 {
		t.Fatalf("首个节点应为自身主节点且带偏移量, got %+v", nodes[0])
	}
	if nodes[1].Role != "replica" || nodes[1].Addr != "192.168.1.118:16381" || nodes[1].Master != "192.168.1.118:16380" {
		t.Fatalf("从节点信息不正确, got %+v", nodes[1])
	}
	if nodes[2].Lag != 1 {
		t.Fatalf("从节点延迟未解析, got %+v", nodes[2])
	}
}

// 验证从节点信息不完整时跳过，不会生成空地址节点导致前端画出孤立点。
func TestReplicationTopologySkipsIncompleteSlave(t *testing.T) {
	info := map[string]map[string]string{
		"Replication": {
			"connected_slaves": "2",
			"slave0":           "ip=192.168.1.118,port=16381,state=online",
			"slave1":           "ip=,port=",
		},
	}

	nodes := replicationTopology(info, "192.168.1.118:16380")

	if len(nodes) != 2 {
		t.Fatalf("只应保留主节点与 1 个有效从节点, got %d 个: %+v", len(nodes), nodes)
	}
}

// 验证当前节点是从节点时，向上补出主节点且自身标记为 Self；
// 此处 conn 传 nil，覆盖"无法回连主节点时只展示已知两个节点"的降级路径。
func TestReplicaTopologyWithoutReconnect(t *testing.T) {
	info := map[string]map[string]string{
		"Replication": {
			"role":               "slave",
			"master_host":        "192.168.1.118",
			"master_port":        "16380",
			"master_link_status": "up",
			"slave_repl_offset":  "3553",
		},
	}

	nodes := replicaTopology(nil, info, "192.168.1.118:16381")

	if len(nodes) != 2 {
		t.Fatalf("节点数 = %d, want 2", len(nodes))
	}
	if nodes[0].Role != "master" || nodes[0].Addr != "192.168.1.118:16380" || nodes[0].State != "online" {
		t.Fatalf("主节点还原不正确, got %+v", nodes[0])
	}
	if nodes[1].Role != "replica" || !nodes[1].Self || nodes[1].Master != "192.168.1.118:16380" {
		t.Fatalf("自身从节点还原不正确, got %+v", nodes[1])
	}
}

// 验证主从断链时状态标记为 down，否则界面会把失联节点显示成健康状态。
func TestReplicaTopologyLinkDown(t *testing.T) {
	info := map[string]map[string]string{
		"Replication": {
			"master_host":        "192.168.1.118",
			"master_port":        "16380",
			"master_link_status": "down",
		},
	}

	nodes := replicaTopology(nil, info, "192.168.1.118:16381")

	for _, n := range nodes {
		if n.State != "down" {
			t.Fatalf("断链时应全部标记 down, got %+v", n)
		}
	}
}

// CLUSTER NODES 的完整样本：故意把从节点放在前面，用于验证排序与主节点 ID 转地址。
const clusterNodesSample = `1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d 192.168.1.118:16373@26373 slave 07c37dfeb235213a872192d90877d0cd55635b91 0 1700000003000 4 connected
07c37dfeb235213a872192d90877d0cd55635b91 192.168.1.118:16370@26370 myself,master - 0 1700000000000 1 connected 0-5460
e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca 192.168.1.118:16371@26371 master - 0 1700000001000 2 connected 5461-10922
239b6398f7f0e1d3f2b3e2a1c9f8d7e6a5b4c3d2 192.168.1.118:16372@26372 master - 0 1700000002000 3 connected 10923-16383
9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c 192.168.1.118:16375@26375 slave e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca 0 1700000004000 5 connected
4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2b1a 192.168.1.118:16374@26374 slave 239b6398f7f0e1d3f2b3e2a1c9f8d7e6a5b4c3d2 0 1700000005000 6 connected`

// 验证集群拓扑：剥离 bus 端口、master ID 转地址、主节点按首槽位排序、从节点跟随其主节点。
func TestParseClusterNodes(t *testing.T) {
	nodes := parseClusterNodes(clusterNodesSample, "192.168.1.118:16370")

	wantOrder := []string{
		"192.168.1.118:16370",
		"192.168.1.118:16371",
		"192.168.1.118:16372",
		"192.168.1.118:16373",
		"192.168.1.118:16375",
		"192.168.1.118:16374",
	}
	if len(nodes) != len(wantOrder) {
		t.Fatalf("节点数 = %d, want %d", len(nodes), len(wantOrder))
	}
	for i, want := range wantOrder {
		if nodes[i].Addr != want {
			t.Fatalf("第 %d 个节点 = %s, want %s（顺序: %v）", i, nodes[i].Addr, want, wantOrder)
		}
	}

	// 地址里的 @bus-port 必须被剥掉，否则前端拼出的地址无法访问
	for _, n := range nodes {
		if strings.Contains(n.Addr, "@") || strings.Contains(n.Addr, ",") {
			t.Fatalf("节点地址未剥离 bus 端口或主机名后缀: %+v", n)
		}
	}

	if !nodes[0].Self {
		t.Fatalf("与 selfAddr 相同的节点应标记 Self, got %+v", nodes[0])
	}
	if nodes[0].SlotStart != 0 || nodes[1].SlotStart != 5461 || nodes[2].SlotStart != 10923 {
		t.Fatalf("主节点首槽位解析不正确: %d %d %d", nodes[0].SlotStart, nodes[1].SlotStart, nodes[2].SlotStart)
	}
	// 从节点的 master 字段应是地址而不是节点 ID
	if nodes[3].Master != "192.168.1.118:16370" {
		t.Fatalf("从节点 master 未转成地址, got %q", nodes[3].Master)
	}
	if nodes[3].State != "online" {
		t.Fatalf("connected 状态的从节点应为 online, got %q", nodes[3].State)
	}
}

// 验证故障与无地址节点被标记为 down，避免把失效节点显示为可用。
func TestParseClusterNodesDownState(t *testing.T) {
	text := "aaa 192.168.1.118:16370@26370 master - 0 1 1 connected 0-5460\n" +
		"bbb 192.168.1.118:16371@26371 master,fail - 0 1 2 disconnected 5461-10922\n" +
		"ccc :0@0 noaddr - 0 1 3 connected\n"

	nodes := parseClusterNodes(text, "")

	if len(nodes) != 3 {
		t.Fatalf("节点数 = %d, want 3", len(nodes))
	}
	if nodes[0].State != "online" {
		t.Fatalf("首节点应为 online, got %q", nodes[0].State)
	}
	if nodes[1].State != "down" {
		t.Fatalf("fail/disconnected 节点应为 down, got %q", nodes[1].State)
	}
	if nodes[2].State != "down" {
		t.Fatalf("noaddr 节点应为 down, got %q", nodes[2].State)
	}
}

// 验证畸形行被丢弃，不会因为字段不足导致越界。
func TestParseClusterNodesSkipsMalformedLine(t *testing.T) {
	text := "aaa 192.168.1.118:16370@26370 master - 0 1 1 connected 0-5460\n" +
		"\n" +
		"bbb 192.168.1.118:16371\n"

	nodes := parseClusterNodes(text, "")

	if len(nodes) != 1 {
		t.Fatalf("只应保留一条合法记录, got %d: %+v", len(nodes), nodes)
	}
}

// 验证故障节点（fail/noaddr/无槽位主节点）统一排到末尾，
// 避免宕机节点因为没有槽位被排到健康主节点中间，干扰主节点顺序阅读。
func TestParseClusterNodesSortsFaultyLast(t *testing.T) {
	text := "aaa 192.168.1.118:16371@26371 master - 0 1 2 connected 5461-10922\n" +
		"bbb 192.168.1.118:16372@26372 master,fail - 0 1 3 disconnected 10923-16383\n" +
		"ccc 192.168.1.118:16370@26370 myself,master - 0 1 1 connected 0-5460\n" +
		"ddd 192.168.1.118:16373@26373 slave ccc 0 1 4 connected\n"

	nodes := parseClusterNodes(text, "192.168.1.118:16370")

	got := make([]string, 0, len(nodes))
	for _, n := range nodes {
		got = append(got, n.Addr)
	}
	want := []string{
		"192.168.1.118:16370", // 健康主节点，按槽位升序
		"192.168.1.118:16371",
		"192.168.1.118:16373", // 健康从节点
		"192.168.1.118:16372", // 故障主节点置尾
	}
	if len(got) != len(want) {
		t.Fatalf("节点数 = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序结果 = %v, want %v", got, want)
		}
	}
}

// 验证槽位串的首槽位提取，包括单槽位与空串。
func TestFirstSlot(t *testing.T) {
	cases := []struct {
		slots string
		want  int
	}{
		{"0-5460 10923-12000", 0},
		{"10923-16383", 10923},
		{"5461", 5461},
		{"", 0},
		{"  ", 0},
	}
	for _, c := range cases {
		if got := firstSlot(c.slots); got != c.want {
			t.Fatalf("firstSlot(%q) = %d, want %d", c.slots, got, c.want)
		}
	}
}

// 验证主从链路状态到展示状态的映射，空状态表示未知而不是异常。
func TestMasterLinkState(t *testing.T) {
	cases := map[string]string{"up": "online", "down": "down", "": ""}
	for in, want := range cases {
		if got := masterLinkState(in); got != want {
			t.Fatalf("masterLinkState(%q) = %q, want %q", in, got, want)
		}
	}
}

// 验证哨兵节点的存活判定：s_down/o_down/disconnected 任一出现即为 down。
func TestSentinelFlags(t *testing.T) {
	cases := []struct {
		flags string
		want  string
	}{
		{"master", "online"},
		{"slave", "online"},
		{"s_down,master,disconnected", "down"},
		{"o_down,slave", "down"},
		{"master,disconnected", "down"},
	}
	for _, c := range cases {
		if got := sentinelFlags("192.168.1.118:16390", c.flags); got != c.want {
			t.Fatalf("sentinelFlags(%q) = %q, want %q", c.flags, got, c.want)
		}
	}
}

// 验证哨兵拓扑的排序：主节点、从节点、哨兵依次排列，同类按地址升序。
func TestSortTopologyNodes(t *testing.T) {
	nodes := []topologyNode{
		{Addr: "192.168.1.118:26392", Role: "sentinel"},
		{Addr: "192.168.1.118:16391", Role: "replica"},
		{Addr: "192.168.1.118:26390", Role: "sentinel"},
		{Addr: "192.168.1.118:16390", Role: "master"},
		{Addr: "192.168.1.118:16380", Role: "replica"},
	}

	got := sortTopologyNodes(nodes)
	want := []string{
		"192.168.1.118:16390",
		"192.168.1.118:16380",
		"192.168.1.118:16391",
		"192.168.1.118:26390",
		"192.168.1.118:26392",
	}
	for i := range want {
		if got[i].Addr != want[i] {
			t.Fatalf("第 %d 个节点 = %s, want %s", i, got[i].Addr, want[i])
		}
	}
}

// 验证重复节点判定，用于避免同一哨兵在拓扑里出现两次。
func TestHasNode(t *testing.T) {
	nodes := []topologyNode{{Addr: "192.168.1.118:16390"}}
	if !hasNode(nodes, "192.168.1.118:16390") {
		t.Fatal("已存在的地址应返回 true")
	}
	if hasNode(nodes, "192.168.1.118:16391") {
		t.Fatal("不存在的地址应返回 false")
	}
}

// 验证多视图合并：迁移标记只在其中一个节点的视图里时，合并后仍保留，
// 且槽位区间不会因为重复上报而重复出现。
func TestMergeClusterViewsUnionsMarkers(t *testing.T) {
	fromA := "aaa 192.168.1.118:16370@26370 myself,master - 0 1 1 connected 0-2729 [100->-bbb]\n" +
		"bbb 192.168.1.118:16371@26371 master - 0 1 2 connected 2730-5460\n"
	fromB := "aaa 192.168.1.118:16370@26370 master - 0 1 1 connected 0-2729\n" +
		"bbb 192.168.1.118:16371@26371 myself,master - 0 1 2 connected 2730-5460\n"

	nodes := mergeClusterViews([]clusterView{
		{Addr: "192.168.1.118:16370", Text: fromA},
		{Addr: "192.168.1.118:16371", Text: fromB},
	}, "192.168.1.118:16370")

	if len(nodes) != 2 {
		t.Fatalf("节点数 = %d, want 2", len(nodes))
	}
	if nodes[0].Addr != "192.168.1.118:16370" || nodes[1].Addr != "192.168.1.118:16371" {
		t.Fatalf("排序结果不正确: %+v", nodes)
	}
	if nodes[0].Slots != "0-2729 [100->-bbb]" {
		t.Fatalf("槽位与标记未正确合并: %q", nodes[0].Slots)
	}
	if strings.Count(nodes[0].Slots, "0-2729") != 1 {
		t.Fatalf("槽位区间重复: %q", nodes[0].Slots)
	}
	if nodes[0].SlotStart != 0 || nodes[1].SlotStart != 2730 {
		t.Fatalf("首槽位计算不正确: %d %d", nodes[0].SlotStart, nodes[1].SlotStart)
	}
	if !nodes[0].Self {
		t.Fatal("自身节点标记丢失")
	}
}

// 验证合并时以节点自身的视图为准：其他节点的陈旧视图不能把在线节点标成离线。
func TestMergeClusterViewsPrefersSelfView(t *testing.T) {
	selfView := "aaa 192.168.1.118:16370@26370 myself,master - 0 1 1 connected 0-2729\n"
	staleView := "aaa 192.168.1.118:16370@26370 master,fail - 0 1 1 disconnected 0-2729\n"

	nodes := mergeClusterViews([]clusterView{
		{Addr: "192.168.1.118:16371", Text: staleView},
		{Addr: "192.168.1.118:16370", Text: selfView},
	}, "192.168.1.118:16370")

	if len(nodes) != 1 {
		t.Fatalf("节点数 = %d, want 1", len(nodes))
	}
	if nodes[0].State != "online" {
		t.Fatalf("应优先采用自身视图的 online 状态, got %q", nodes[0].State)
	}
}

// 验证同一标记在多个视图里重复上报时只保留一份。
func TestMergeClusterViewsDeduplicatesMarkers(t *testing.T) {
	text := "aaa 192.168.1.118:16370@26370 myself,master - 0 1 1 connected 0-100 [5->-bbb]\n"
	nodes := mergeClusterViews([]clusterView{
		{Addr: "192.168.1.118:16370", Text: text},
		{Addr: "192.168.1.118:16371", Text: text},
	}, "192.168.1.118:16370")

	if len(nodes) != 1 {
		t.Fatalf("节点数 = %d, want 1", len(nodes))
	}
	if strings.Count(nodes[0].Slots, "[5->-bbb]") != 1 {
		t.Fatalf("标记应去重: %q", nodes[0].Slots)
	}
}

// 验证合并阈值：分片过多时退回单节点视图，避免每 5 秒向几十个节点发起请求。
func TestShouldMergeClusterShards(t *testing.T) {
	cases := map[int]bool{
		0:                          false,
		1:                          true,
		clusterMergeShardLimit:     true,
		clusterMergeShardLimit + 1: false,
		30:                         false,
	}
	for count, want := range cases {
		if got := shouldMergeClusterShards(count); got != want {
			t.Fatalf("shouldMergeClusterShards(%d) = %v, want %v", count, got, want)
		}
	}
}
