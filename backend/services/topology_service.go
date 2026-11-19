package services

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
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
	SlotStart int `json:"slotStart"`
	Master string `json:"master,omitempty"`
	Self   bool   `json:"self"`
}

// topologyData 部署拓扑的完整描述。
type topologyData struct {
	Mode    string         `json:"mode"` // standalone / master-replica / sentinel / cluster
	Self    string         `json:"self,omitempty"`
	Version string         `json:"version,omitempty"`
	Nodes   []topologyNode `json:"nodes"`
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
		data.Nodes = b.clusterTopology(ctx, client, data.Self)
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
		Addr:  self,
		Role:  "master",
		State: "online",
		Self:  true,
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

// clusterTopology 解析 CLUSTER NODES 输出，得到集群各节点与槽位归属。
func (b *browserService) clusterTopology(ctx context.Context, client redis.UniversalClient, selfAddr string) []topologyNode {
	text, err := client.Do(ctx, "CLUSTER", "NODES").Text()
	if err != nil {
		return nil
	}

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

	// 固定顺序：主节点按首槽位升序，从节点跟随其主节点之后再按地址排序。
	slotOf := map[string]int{}
	for _, n := range nodes {
		if n.Role == "master" {
			slotOf[n.Addr] = n.SlotStart
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
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
