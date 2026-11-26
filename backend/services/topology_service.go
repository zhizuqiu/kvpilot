package services

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

// topologyNode 部署拓扑中的一个节点。
type topologyNode struct {
	ID     string `json:"id,omitempty"`
	Addr   string `json:"addr"`
	Role   string `json:"role"` // master / replica / sentinel
	State  string `json:"state,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Lag    int64  `json:"lag,omitempty"`
	Slots  string `json:"slots,omitempty"`
	// SlotStart 是该节点负责的第一个槽位，用于稳定排序
	SlotStart int    `json:"slotStart"`
	Master    string `json:"master,omitempty"`
	Self      bool   `json:"self"`
}

// topologyData 部署拓扑的完整描述。
type topologyData struct {
	Mode    string         `json:"mode"` // standalone / master-replica / sentinel / cluster
	Self    string         `json:"self,omitempty"`
	Version string         `json:"version,omitempty"`
	Nodes   []topologyNode `json:"nodes"`
	// Merged 表示拓扑是否由全部分片视图合并而来；
	// 为 false 时迁移/导入状态可能不完整（分片过多或取数失败）。
	Merged bool `json:"merged"`
}

// Topology 返回当前连接的部署拓扑：单机、主从、哨兵或集群。
func (b *browserService) Topology(name string) (resp types.JSResp) {
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	client, ctx := item.client, item.ctx

	infoText, err := client.Info(ctx).Result()
	if err != nil {
		resp.Msg = "get server info fail:" + err.Error()
		return
	}
	info := b.parseInfo(infoText)

	conn := Connection().getConnection(name)
	data := topologyData{
		Version: info["Server"]["redis_version"],
		Self:    b.clientSelfAddr(ctx, client),
	}
	if data.Self == "" && conn != nil {
		data.Self = net.JoinHostPort(conn.Addr, strconv.Itoa(conn.Port))
	}

	switch {
	case info["Cluster"]["cluster_enabled"] == "1":
		data.Mode = "cluster"
		if conn != nil {
			// ClusterClient 会把命令随机路由到某个节点，因此以用户连接的地址作为「当前节点」
			data.Self = net.JoinHostPort(conn.Addr, strconv.Itoa(conn.Port))
		}
		nodes, merged := b.clusterTopology(ctx, client, data.Self)
		data.Nodes = nodes
		data.Merged = merged
	case conn != nil && conn.Sentinel.Enable:
		data.Mode = "sentinel"
		data.Nodes = b.sentinelTopology(conn, info, data.Self)
	case info["Replication"]["role"] == "slave":
		data.Mode = "master-replica"
		data.Nodes = replicaTopology(conn, info, data.Self)
	case toInt(info["Replication"]["connected_slaves"]) > 0:
		data.Mode = "master-replica"
		data.Nodes = replicationTopology(info, data.Self)
	default:
		data.Mode = "standalone"
		data.Nodes = []topologyNode{{Addr: data.Self, Role: "master", State: "online", Self: true}}
	}

	resp.Success = true
	resp.Data = data
	return
}

// clientSelfAddr 通过 CLIENT INFO 拿到服务端自身监听地址（laddr）。
func (b *browserService) clientSelfAddr(ctx context.Context, client redis.UniversalClient) string {
	text, err := client.Do(ctx, "CLIENT", "INFO").Text()
	if err != nil {
		return ""
	}
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "laddr=") {
			return strings.TrimPrefix(field, "laddr=")
		}
	}
	return ""
}

// replicationTopology 以当前节点为主节点，结合 INFO replication 生成主从拓扑。
func replicationTopology(info map[string]map[string]string, self string) []topologyNode {
	repl := info["Replication"]
	nodes := []topologyNode{{
		Addr:   self,
		Role:   "master",
		State:  "online",
		Self:   true,
		Offset: toInt64(repl["master_repl_offset"]),
	}}

	slaves := toInt(repl["connected_slaves"])
	for i := 0; i < slaves; i++ {
		fields := parseSlaveLine(repl["slave"+strconv.Itoa(i)])
		if fields["ip"] == "" || fields["port"] == "" {
			continue
		}
		nodes = append(nodes, topologyNode{
			Addr:   net.JoinHostPort(fields["ip"], fields["port"]),
			Role:   "replica",
			State:  fields["state"],
			Offset: toInt64(fields["offset"]),
			Lag:    toInt64(fields["lag"]),
			Master: self,
		})
	}
	return nodes
}

// replicaTopology 当前节点是从节点时，向上补出主节点，并尽量向主节点补齐其余从节点。
func replicaTopology(conn *types.Connection, info map[string]map[string]string, self string) []topologyNode {
	repl := info["Replication"]
	masterAddr := net.JoinHostPort(repl["master_host"], repl["master_port"])
	nodes := []topologyNode{{
		Addr:  masterAddr,
		Role:  "master",
		State: masterLinkState(repl["master_link_status"]),
	}}

	nodes = append(nodes, topologyNode{
		Addr:   self,
		Role:   "replica",
		State:  masterLinkState(repl["master_link_status"]),
		Offset: toInt64(repl["slave_repl_offset"]),
		Master: masterAddr,
		Self:   true,
	})

	// 再连一次主节点，拿到完整从节点列表；失败则只展示已知的两个节点。
	if conn == nil {
		return nodes
	}
	client := redis.NewClient(&redis.Options{
		Addr:        masterAddr,
		Username:    conn.Username,
		Password:    conn.Password,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
		Protocol:    2,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	text, err := client.Info(ctx, "replication").Result()
	if err != nil {
		return nodes
	}
	infoText := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ":"); ok {
			infoText[k] = v
		}
	}
	count := toInt(infoText["connected_slaves"])
	for i := 0; i < count; i++ {
		fields := parseSlaveLine(infoText["slave"+strconv.Itoa(i)])
		if fields["ip"] == "" || fields["port"] == "" {
			continue
		}
		addr := net.JoinHostPort(fields["ip"], fields["port"])
		if addr == self {
			continue
		}
		nodes = append(nodes, topologyNode{
			Addr:   addr,
			Role:   "replica",
			State:  fields["state"],
			Offset: toInt64(fields["offset"]),
			Lag:    toInt64(fields["lag"]),
			Master: masterAddr,
		})
	}
	return nodes
}

// sentinelTopology 通过哨兵节点获取主节点、从节点与其余哨兵。
func (b *browserService) sentinelTopology(conn *types.Connection, info map[string]map[string]string, self string) []topologyNode {
	sentinelAddr := net.JoinHostPort(conn.Addr, strconv.Itoa(conn.Port))
	sentinel := redis.NewSentinelClient(&redis.Options{
		Addr:        sentinelAddr,
		Username:    conn.Username,
		Password:    conn.Password,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
		Protocol:    2,
	})
	defer sentinel.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	nodes := []topologyNode{}
	masterName := conn.Sentinel.Master
	masterAddr := self
	if masterName != "" {
		masters := redis.NewMapStringStringSliceCmd(ctx, "sentinel", "masters")
		if err := sentinel.Process(ctx, masters); err == nil {
			for _, m := range masters.Val() {
				if m["name"] != masterName {
					continue
				}
				masterAddr = net.JoinHostPort(m["ip"], m["port"])
				flags := m["flags"]
				nodes = append(nodes, topologyNode{
					Addr:  masterAddr,
					Role:  "master",
					State: sentinelFlags(masterAddr, flags),
					Self:  masterAddr == self,
				})
				break
			}
		}

		if replicas, err := sentinel.Replicas(ctx, masterName).Result(); err == nil {
			for _, r := range replicas {
				addr := net.JoinHostPort(r["ip"], r["port"])
				nodes = append(nodes, topologyNode{
					Addr:   addr,
					Role:   "replica",
					State:  sentinelFlags(addr, r["flags"]),
					Offset: toInt64(r["slave-repl-offset"]),
					Master: masterAddr,
					Self:   addr == self,
				})
			}
		}

		if peers, err := sentinel.Sentinels(ctx, masterName).Result(); err == nil {
			for _, p := range peers {
				addr := net.JoinHostPort(p["ip"], p["port"])
				nodes = append(nodes, topologyNode{
					Addr:  addr,
					Role:  "sentinel",
					State: sentinelFlags(addr, p["flags"]),
					Self:  addr == self,
				})
			}
		}
	}

	nodes = sortTopologyNodes(nodes)

	// 哨兵不可达时至少展示当前数据节点。
	if len(nodes) == 0 {
		role := info["Replication"]["role"]
		if role == "" {
			role = "master"
		}
		nodes = append(nodes, topologyNode{Addr: self, Role: role, State: "online", Self: true})
	}
	// 当前哨兵本身也作为节点展示
	if !hasNode(nodes, sentinelAddr) {
		nodes = append(nodes, topologyNode{
			Addr:  sentinelAddr,
			Role:  "sentinel",
			State: "online",
			Self:  sentinelAddr == self,
		})
	}
	return nodes
}

// clusterMergeShardLimit 是允许"逐分片合并"的主分片数上限。
// 超过后每次都要向几十个节点请求，代价不划算，退回单节点视图并在界面标注。
const clusterMergeShardLimit = 16

// shouldMergeClusterShards 判断当前分片数是否值得逐分片合并。
func shouldMergeClusterShards(shardCount int) bool {
	return shardCount > 0 && shardCount <= clusterMergeShardLimit
}

// clusterView 是某个节点上报的 CLUSTER NODES 原文。
type clusterView struct {
	Addr string
	Text string
}

// clusterTopology 返回集群各节点与槽位归属。
// 默认向每个主分片各取一份 CLUSTER NODES 再合并：迁移/导入标记只存在于
// 相关节点自身的视图里，单节点随机视图会丢失这些状态。
// 分片数超过上限时退回单节点视图，并通过第二个返回值告知调用方"未合并"。
func (b *browserService) clusterTopology(ctx context.Context, client redis.UniversalClient, selfAddr string) ([]topologyNode, bool) {
	cc, ok := client.(*redis.ClusterClient)
	if !ok {
		text, err := client.Do(ctx, "CLUSTER", "NODES").Text()
		if err != nil {
			return nil, false
		}
		return parseClusterNodes(text, selfAddr), false
	}

	// 先用 CLUSTER SLOTS 统计主节点数量，大集群不做逐分片合并。
	// 注意不能直接用区间条数：槽位碎片化时同一个主节点会返回多条区间。
	slots, err := cc.ClusterSlots(ctx).Result()
	if err != nil {
		return nil, false
	}
	if !shouldMergeClusterShards(countClusterShards(slots)) {
		text, err := cc.Do(ctx, "CLUSTER", "NODES").Text()
		if err != nil {
			return nil, false
		}
		return parseClusterNodes(text, selfAddr), false
	}

	// ForEachShard 并发执行，收集结果必须加锁
	var mu sync.Mutex
	views := make([]clusterView, 0, len(slots))
	_ = cc.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
		text, err := shard.Do(ctx, "CLUSTER", "NODES").Text()
		if err != nil {
			return nil
		}
		mu.Lock()
		views = append(views, clusterView{Addr: shard.Options().Addr, Text: text})
		mu.Unlock()
		return nil
	})
	if len(views) == 0 {
		return nil, false
	}
	return mergeClusterViews(views, selfAddr), true
}

// countClusterShards 统计槽位映射中涉及的不同主节点数量。
func countClusterShards(slots []redis.ClusterSlot) int {
	masters := map[string]bool{}
	for _, slot := range slots {
		for _, node := range slot.Nodes {
			masters[node.Addr] = true
		}
	}
	return len(masters)
}

// mergeClusterViews 合并多个节点上报的 CLUSTER NODES 视图：
// 节点自身视图优先（状态更可信），槽位标记取并集（迁移/导入标记可能只在部分视图里）。
func mergeClusterViews(views []clusterView, selfAddr string) []topologyNode {
	type merged struct {
		base    topologyNode
		ranges  []string
		markers []string
		seen    map[string]bool
	}

	order := make([]string, 0, len(views)*3)
	byID := map[string]*merged{}

	for _, view := range views {
		for _, node := range parseClusterNodesRaw(view.Text, selfAddr) {
			item, ok := byID[node.ID]
			if !ok {
				item = &merged{base: node, seen: map[string]bool{}}
				byID[node.ID] = item
				order = append(order, node.ID)
			}
			// 节点自己上报的视图优先，避免用其他节点的推测状态覆盖
			if view.Addr == node.Addr {
				item.base = node
			}
			for _, token := range strings.Fields(node.Slots) {
				if item.seen[token] {
					continue
				}
				item.seen[token] = true
				if strings.HasPrefix(token, "[") {
					item.markers = append(item.markers, token)
				} else {
					item.ranges = append(item.ranges, token)
				}
			}
		}
	}

	nodes := make([]topologyNode, 0, len(order))
	for _, id := range order {
		item := byID[id]
		tokens := append(append([]string{}, item.ranges...), item.markers...)
		node := item.base
		node.Slots = strings.Join(tokens, " ")
		node.SlotStart = firstSlot(node.Slots)
		nodes = append(nodes, node)
	}
	return finalizeClusterNodes(nodes)
}

// parseClusterNodes 把 CLUSTER NODES 的文本结果转成拓扑节点，并排成稳定顺序。
func parseClusterNodes(text, selfAddr string) []topologyNode {
	return finalizeClusterNodes(parseClusterNodesRaw(text, selfAddr))
}

// parseClusterNodesRaw 只做解析，不做 master ID 转换与排序，供多视图合并后统一收尾。
func parseClusterNodesRaw(text, selfAddr string) []topologyNode {
	nodes := []topologyNode{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}
		addr := parts[1]
		if idx := strings.Index(addr, "@"); idx >= 0 {
			addr = addr[:idx]
		}
		if idx := strings.Index(addr, ","); idx >= 0 {
			addr = addr[:idx]
		}
		flags := parts[2]
		role := "replica"
		if strings.Contains(flags, "master") {
			role = "master"
		}
		if strings.Contains(flags, "sentinel") {
			role = "sentinel"
		}
		state := "online"
		if parts[7] != "connected" || strings.Contains(flags, "fail") || strings.Contains(flags, "noaddr") {
			state = "down"
		}

		node := topologyNode{
			ID:    parts[0],
			Addr:  addr,
			Role:  role,
			State: state,
			Self:  addr == selfAddr,
		}
		if role == "replica" && parts[3] != "-" {
			node.Master = parts[3]
		}
		if len(parts) > 8 {
			node.Slots = strings.Join(parts[8:], " ")
			node.SlotStart = firstSlot(node.Slots)
		}
		nodes = append(nodes, node)
	}

	return nodes
}

// finalizeClusterNodes 把 master ID 转成地址，并排成稳定顺序。
func finalizeClusterNodes(nodes []topologyNode) []topologyNode {
	// CLUSTER NODES 里的 master 字段是节点 ID，转成地址更直观
	idToAddr := map[string]string{}
	for _, n := range nodes {
		idToAddr[n.ID] = n.Addr
	}
	for i := range nodes {
		if nodes[i].Master != "" {
			if addr, ok := idToAddr[nodes[i].Master]; ok {
				nodes[i].Master = addr
			}
		}
	}

	// 固定顺序：先健康节点（主节点按首槽位升序，从节点跟随其主节点），
	// 故障节点（fail/noaddr/无槽位主节点）统一置尾，避免占用健康节点的阅读位置。
	faulty := func(n topologyNode) bool {
		return n.State == "down" || (n.Role == "master" && strings.TrimSpace(n.Slots) == "")
	}
	slotOf := map[string]int{}
	for _, n := range nodes {
		if n.Role == "master" {
			slotOf[n.Addr] = n.SlotStart
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if faulty(a) != faulty(b) {
			return !faulty(a)
		}
		if faulty(a) {
			return a.Addr < b.Addr
		}
		if (a.Role == "master") != (b.Role == "master") {
			return a.Role == "master"
		}
		if a.Role == "master" {
			if a.SlotStart != b.SlotStart {
				return a.SlotStart < b.SlotStart
			}
			return a.Addr < b.Addr
		}
		am, bm := slotOf[a.Master], slotOf[b.Master]
		if am != bm {
			return am < bm
		}
		if a.Master != b.Master {
			return a.Master < b.Master
		}
		return a.Addr < b.Addr
	})
	return nodes
}

// firstSlot 从 "0-5460 10923-12000" 这样的槽位串中取第一个起点。
func firstSlot(slots string) int {
	slots = strings.TrimSpace(slots)
	if slots == "" {
		return 0
	}
	first := strings.Fields(slots)[0]
	if idx := strings.Index(first, "-"); idx >= 0 {
		first = first[:idx]
	}
	v, _ := strconv.Atoi(first)
	return v
}

// parseSlaveLine 解析 INFO replication 中 slaveN 的 ip=..,port=.. 形式。
func parseSlaveLine(line string) map[string]string {
	ret := map[string]string{}
	for _, field := range strings.Split(line, ",") {
		if k, v, ok := strings.Cut(field, "="); ok {
			ret[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return ret
}

func masterLinkState(status string) string {
	if status == "up" {
		return "online"
	}
	if status == "" {
		return ""
	}
	return "down"
}

func sortTopologyNodes(nodes []topologyNode) []topologyNode {
	order := func(role string) int {
		switch role {
		case "master":
			return 0
		case "replica":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if order(nodes[i].Role) != order(nodes[j].Role) {
			return order(nodes[i].Role) < order(nodes[j].Role)
		}
		return nodes[i].Addr < nodes[j].Addr
	})
	return nodes
}

func sentinelFlags(addr, flags string) string {
	switch {
	case strings.Contains(flags, "s_down"), strings.Contains(flags, "o_down"), strings.Contains(flags, "disconnected"):
		return "down"
	default:
		return "online"
	}
}

func hasNode(nodes []topologyNode, addr string) bool {
	for _, n := range nodes {
		if n.Addr == addr {
			return true
		}
	}
	return false
}

func toInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func toInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}
