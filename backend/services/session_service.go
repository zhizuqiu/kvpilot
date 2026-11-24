package services

import (
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"tinyrdm/backend/types"
)

// sessionIdleThreshold 是概览中"空闲会话"的判定阈值（秒）。
const sessionIdleThreshold = 60

// sessionItem 是一条 CLIENT LIST 记录。
type sessionItem struct {
	Node      string `json:"node"`
	ID        int64  `json:"id"`
	Addr      string `json:"addr"`
	Name      string `json:"name"`
	User      string `json:"user"`
	DB        int    `json:"db"`
	Age       int64  `json:"age"`
	Idle      int64  `json:"idle"`
	Flags     string `json:"flags"`
	Cmd       string `json:"cmd"`
	Protected bool   `json:"protected"`
}

// sessionSource 是按来源地址聚合后的会话数量。
type sessionSource struct {
	Addr  string `json:"addr"`
	Count int    `json:"count"`
}

// sessionData 是会话列表接口的返回体。
type sessionData struct {
	Nodes       []string        `json:"nodes"`
	Unavailable []string        `json:"unavailable,omitempty"`
	Sessions    []sessionItem   `json:"sessions"`
	Total       int             `json:"total"`
	Idle        int             `json:"idle"`
	Sources     []sessionSource `json:"sources"`
}

// parseClientList 把 CLIENT LIST 的文本解析成结构化会话。
// 字段以空格分隔、形如 key=value；无法确定的记录直接跳过，避免产生零值会话。
func parseClientList(node, content string) []sessionItem {
	items := make([]sessionItem, 0, 16)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := map[string]string{}
		for _, part := range strings.Fields(line) {
			if key, value, ok := strings.Cut(part, "="); ok {
				fields[key] = value
			}
		}

		id, err := strconv.ParseInt(fields["id"], 10, 64)
		if err != nil || id <= 0 {
			continue
		}

		item := sessionItem{
			Node:  node,
			ID:    id,
			Addr:  fields["addr"],
			Name:  decodeSessionName(fields["name"]),
			User:  fields["user"],
			DB:    int(parseSessionInt(fields["db"])),
			Age:   parseSessionInt(fields["age"]),
			Idle:  parseSessionInt(fields["idle"]),
			Flags: fields["flags"],
			Cmd:   fields["cmd"],
		}
		item.Protected = clientFlagsProtected(item.Flags)
		items = append(items, item)
	}
	return items
}

// decodeSessionName 还原 CLIENT LIST 中被 URL 编码的连接名；解码失败时保留原文。
func decodeSessionName(name string) string {
	if name == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		return decoded
	}
	return name
}

func parseSessionInt(value string) int64 {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// clientFlagsProtected 判断连接是否属于主从/哨兵链路，这类连接不允许关闭。
// flags 中 S 表示副本、M 表示主节点、O 表示监视模式下的副本。
func clientFlagsProtected(flags string) bool {
	return strings.ContainsAny(strings.ToUpper(strings.TrimSpace(flags)), "OSM")
}

// buildSessionData 汇总各节点的会话，保留读取失败的节点并生成概览统计。
func buildSessionData(views map[string][]sessionItem, unavailable []string) sessionData {
	nodes := make([]string, 0, len(views))
	for node := range views {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	sessions := make([]sessionItem, 0)
	for _, node := range nodes {
		perNode := views[node]
		sort.Slice(perNode, func(i, j int) bool { return perNode[i].ID < perNode[j].ID })
		sessions = append(sessions, perNode...)
	}

	hostCount := map[string]int{}
	total, idle := 0, 0
	for _, session := range sessions {
		total++
		if session.Idle >= sessionIdleThreshold {
			idle++
		}
		host := sessionHost(session.Addr)
		if host != "" {
			hostCount[host]++
		}
	}

	sources := make([]sessionSource, 0, len(hostCount))
	for host, count := range hostCount {
		sources = append(sources, sessionSource{Addr: host, Count: count})
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Count != sources[j].Count {
			return sources[i].Count > sources[j].Count
		}
		return sources[i].Addr < sources[j].Addr
	})

	sortedUnavailable := make([]string, 0, len(unavailable))
	seen := map[string]bool{}
	for _, addr := range unavailable {
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		sortedUnavailable = append(sortedUnavailable, addr)
	}
	sort.Strings(sortedUnavailable)

	return sessionData{
		Nodes:       nodes,
		Unavailable: sortedUnavailable,
		Sessions:    sessions,
		Total:       total,
		Idle:        idle,
		Sources:     sources,
	}
}

// sessionHost 从 addr 中取出主机部分，解析失败时返回原值。
func sessionHost(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// closeConfigTargets 关闭为本次查询临时创建的节点客户端。
func closeConfigTargets(targets []configTarget) {
	for _, target := range targets {
		if target.close != nil {
			target.close()
		}
	}
}

// ListSessions 返回连接涉及的所有节点上的客户端会话。
func (b *browserService) ListSessions(server string) (resp types.JSResp) {
	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	targets, _ := b.configTargets(item.ctx, item.client, server)
	if len(targets) == 0 {
		resp.Msg = "no reachable node"
		return
	}
	defer closeConfigTargets(targets)

	views := make(map[string][]sessionItem, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t configTarget) {
			defer wg.Done()
			text, err := t.Client.Do(item.ctx, "CLIENT", "LIST").Text()
			if err != nil {
				return
			}
			list := parseClientList(t.Addr, text)

			mu.Lock()
			views[t.Addr] = list
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	if len(views) == 0 {
		resp.Msg = "get client list fail"
		return
	}

	unavailable := make([]string, 0)
	for _, target := range targets {
		if _, ok := views[target.Addr]; !ok {
			unavailable = append(unavailable, target.Addr)
		}
	}

	resp.Success = true
	resp.Data = buildSessionData(views, unavailable)
	return
}

// KillSession 关闭指定节点上的一个会话。
// 执行前重新读取该节点的会话，确认目标存在且不是主从/哨兵链路。
func (b *browserService) KillSession(server, node string, id int64, addr string) (resp types.JSResp) {
	addr = strings.TrimSpace(addr)
	if id <= 0 && addr == "" {
		resp.Msg = "session id or addr is required"
		return
	}

	item, err := b.getRedisClient(server, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	targets, _ := b.configTargets(item.ctx, item.client, server)
	if len(targets) == 0 {
		resp.Msg = "no reachable node"
		return
	}
	defer closeConfigTargets(targets)

	var target *configTarget
	for i := range targets {
		if node == "" || targets[i].Addr == node {
			target = &targets[i]
			break
		}
	}
	if target == nil {
		resp.Msg = "node not found"
		return
	}

	text, err := target.Client.Do(item.ctx, "CLIENT", "LIST").Text()
	if err != nil {
		resp.Msg = err.Error()
		return
	}

	var session *sessionItem
	sessions := parseClientList(target.Addr, text)
	for i := range sessions {
		if id > 0 && sessions[i].ID == id || id <= 0 && sessions[i].Addr == addr {
			session = &sessions[i]
			break
		}
	}
	if session == nil {
		resp.Msg = "session not found"
		return
	}
	if session.Protected {
		resp.Msg = "protected session cannot be closed"
		return
	}

	killed, err := target.Client.Do(item.ctx, "CLIENT", "KILL", "ID", strconv.FormatInt(session.ID, 10)).Int64()
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	if killed <= 0 {
		resp.Msg = "session not found"
		return
	}

	resp.Success = true
	resp.Data = map[string]any{"killed": killed}
	return
}
