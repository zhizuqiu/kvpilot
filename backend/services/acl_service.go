package services

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

// aclUserInfo 是 ACL 用户列表中的一行摘要。
type aclUserInfo struct {
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Flags     []string `json:"flags"`
	Passwords int      `json:"passwords"`
	Commands  string   `json:"commands"`
	Keys      string   `json:"keys"`
	Channels  string   `json:"channels"`
	Selectors int      `json:"selectors"`
	// Nodes/TotalNodes 表示该用户实际存在于多少个集群节点上
	Nodes      int `json:"nodes"`
	TotalNodes int `json:"totalNodes"`
}

var aclFlagNames = map[string]bool{
	"on": true, "off": true, "nopass": true, "resetpass": true,
	"allkeys": true, "resetkeys": true, "allchannels": true, "resetchannels": true,
	"clearselectors": true, "sanitize-payload": true, "skip-sanitize-payload": true,
}

// aclShardResult 保存某个分片上的执行结果，用于跨节点聚合。
type aclShardResult struct {
	Addr  string
	Value interface{}
	Err   error
}

// aclAllShards 在所有分片上执行命令并按节点地址排序返回，保证读取顺序稳定。
// 单机/哨兵模式只有一个结果。
//
// 注意：go-redis 的 ForEachShard 是并发执行的（每个节点一个 goroutine），
// 回调内写共享切片必须加锁，否则会出现数据竞争并丢结果。
func (b *browserService) aclAllShards(ctx context.Context, client redis.UniversalClient, args ...interface{}) []aclShardResult {
	if cc, ok := client.(*redis.ClusterClient); ok {
		results := make([]aclShardResult, 0, 3)
		var mu sync.Mutex
		_ = cc.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
			value, err := shard.Do(ctx, args...).Result()
			mu.Lock()
			results = append(results, aclShardResult{Addr: shard.Options().Addr, Value: value, Err: err})
			mu.Unlock()
			return nil
		})
		sort.Slice(results, func(i, j int) bool { return results[i].Addr < results[j].Addr })
		return results
	}
	addr := ""
	if c, ok := client.(*redis.Client); ok {
		addr = c.Options().Addr
	}
	value, err := client.Do(ctx, args...).Result()
	return []aclShardResult{{Addr: addr, Value: value, Err: err}}
}

// aclFirst 按节点地址排序后取第一个成功返回的结果，保证读取稳定。
func (b *browserService) aclFirst(ctx context.Context, client redis.UniversalClient, args ...interface{}) (interface{}, error) {
	results := b.aclAllShards(ctx, client, args...)
	var firstErr error
	for _, result := range results {
		if result.Err == nil {
			return result.Value, nil
		}
		if firstErr == nil {
			firstErr = result.Err
		}
	}
	if firstErr == nil {
		firstErr = redis.Nil
	}
	return nil, firstErr
}

// aclDo 执行 ACL 写命令：集群模式下对每个主分片执行，保证多节点一致；
// 单机/哨兵模式直接在当前连接执行。
func (b *browserService) aclDo(ctx context.Context, client redis.UniversalClient, args ...interface{}) error {
	if cc, ok := client.(*redis.ClusterClient); ok {
		return cc.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
			return shard.Do(ctx, args...).Err()
		})
	}
	return client.Do(ctx, args...).Err()
}

// aclRead 读取 ACL 信息：集群模式下固定取地址最小的分片，避免命令被随机路由到
// 不同节点导致"刚写入的用户读不到"。
// ForEachShard 并发执行，因此先收集全部结果再排序挑选，不能依赖回调执行顺序。
func (b *browserService) aclRead(ctx context.Context, client redis.UniversalClient, args ...interface{}) (interface{}, error) {
	if cc, ok := client.(*redis.ClusterClient); ok {
		results := make([]aclShardResult, 0, 3)
		var mu sync.Mutex
		loopErr := cc.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
			value, err := shard.Do(ctx, args...).Result()
			mu.Lock()
			results = append(results, aclShardResult{Addr: shard.Options().Addr, Value: value, Err: err})
			mu.Unlock()
			return nil
		})
		if len(results) == 0 {
			if loopErr != nil {
				return nil, loopErr
			}
			return nil, redis.Nil
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Addr < results[j].Addr })
		for _, result := range results {
			if result.Err == nil {
				return result.Value, nil
			}
		}
		return nil, results[0].Err
	}
	return client.Do(ctx, args...).Result()
}

// AclOverview 返回当前连接身份与全部 ACL 用户摘要。
// 集群模式下会聚合所有节点的用户，并标注每个用户实际存在于多少个节点，
// 避免因命令路由到不同节点导致列表"时有时无"。
func (b *browserService) AclOverview(name string) (resp types.JSResp) {
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	client, ctx := item.client, item.ctx

	whoamiResults := b.aclAllShards(ctx, client, "ACL", "WHOAMI")
	whoami := ""
	for _, r := range whoamiResults {
		if r.Err == nil {
			whoami = toText(r.Value)
			break
		}
	}

	listResults := b.aclAllShards(ctx, client, "ACL", "LIST")
	if len(listResults) == 0 {
		resp.Msg = "get acl list fail"
		return
	}

	totalNodes := len(listResults)
	users, firstErr := aggregateAclUsers(listResults)

	if len(users) == 0 && firstErr != nil {
		resp.Msg = "get acl list fail:" + firstErr.Error()
		return
	}

	resp.Success = true
	resp.Data = map[string]any{
		"whoami": whoami,
		"users":  users,
		"nodes":  totalNodes,
	}
	return
}

// aggregateAclUsers 合并各分片的 ACL LIST 结果：同名用户只保留一份并统计出现节点数，
// 分片报错时记录首个错误但不影响其余分片的数据。
func aggregateAclUsers(results []aclShardResult) ([]aclUserInfo, error) {
	totalNodes := len(results)
	users := make([]aclUserInfo, 0, 8)
	index := map[string]int{}
	var firstErr error
	for _, result := range results {
		if result.Err != nil {
			if firstErr == nil {
				firstErr = result.Err
			}
			continue
		}
		list, _ := result.Value.([]interface{})
		for _, v := range list {
			line, ok := v.(string)
			if !ok {
				continue
			}
			user, ok := parseAclListLine(line)
			if !ok {
				continue
			}
			if idx, exists := index[user.Name]; exists {
				users[idx].Nodes++
				continue
			}
			user.Nodes = 1
			user.TotalNodes = totalNodes
			index[user.Name] = len(users)
			users = append(users, user)
		}
	}

	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	return users, firstErr
}

// aclUserDetail 是规范化后的 ACL 用户详情。
type aclUserDetail struct {
	Flags          []string `json:"flags"`
	Passwords      int      `json:"passwords"`
	PasswordHashes []string `json:"passwordHashes"`
	Commands       string   `json:"commands"`
	Keys           string   `json:"keys"`
	Channels       string   `json:"channels"`
	Selectors      int      `json:"selectors"`
	// Nodes/TotalNodes 表示该用户实际存在于多少个集群节点上
	Nodes      int `json:"nodes"`
	TotalNodes int `json:"totalNodes"`
}

// AclUser 返回指定用户的详细信息（ACL GETUSER）。
// 集群模式下逐节点读取，返回第一个匹配结果并标注存在节点数。
func (b *browserService) AclUser(name, user string) (resp types.JSResp) {
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	client, ctx := item.client, item.ctx

	results := b.aclAllShards(ctx, client, "acl", "getuser", user)
	var chosen interface{}
	found := 0
	for _, result := range results {
		if result.Err != nil || result.Value == nil {
			continue
		}
		if chosen == nil {
			chosen = result.Value
		}
		found++
	}
	detail, ok := buildAclUserDetail(chosen, found, len(results))
	if !ok {
		// chosen 为 nil（命令无返回）与返回结构为空都归为"用户不存在"
		resp.Msg = "user not found"
		return
	}

	resp.Success = true
	resp.Data = detail
	return
}

// buildAclUserDetail 把 ACL GETUSER 的原始返回规整成详情结构。
// ok 为 false 表示返回内容为空（用户已不存在或返回结构不可识别）。
func buildAclUserDetail(value interface{}, found, total int) (detail aclUserDetail, ok bool) {
	m := toStringMap(value)
	if len(m) == 0 {
		return detail, false
	}

	hashes := toSlice(m["passwords"])
	// selectors 是对象数组（每个 selector 是一个 map），不能按字符串过滤计数，
	// 否则详情里永远显示 0 个 selector。
	selectors := 0
	if list, ok := m["selectors"].([]interface{}); ok {
		selectors = len(list)
	}
	return aclUserDetail{
		Flags:          toSlice(m["flags"]),
		Passwords:      len(hashes),
		PasswordHashes: hashes,
		Commands:       toText(m["commands"]),
		Keys:           toText(m["keys"]),
		Channels:       toText(m["channels"]),
		Selectors:      selectors,
		Nodes:          found,
		TotalNodes:     total,
	}, true
}

// AclCategories 返回 ACL 的全部命令类别，供前端规则编辑器使用。
func (b *browserService) AclCategories(name string) (resp types.JSResp) {
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	rawAny, err := b.aclFirst(item.ctx, item.client, "ACL", "CAT")
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	raw, _ := rawAny.([]interface{})
	categories := make([]string, 0, len(raw))
	for _, v := range raw {
		if cat, ok := v.(string); ok {
			categories = append(categories, cat)
		}
	}
	resp.Success = true
	resp.Data = categories
	return
}

// AclSetUser 应用一组 ACL 规则（ACL SETUSER），返回更新后的用户详情。
func (b *browserService) AclSetUser(name, user string, rules []string) (resp types.JSResp) {
	if user == "" || len(rules) == 0 {
		resp.Msg = "user and rules are required"
		return
	}
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	if err = b.aclDo(item.ctx, item.client, buildAclSetUserArgs(user, rules)...); err != nil {
		resp.Msg = err.Error()
		return
	}
	return b.AclUser(name, user)
}

// buildAclSetUserArgs 拼装 ACL SETUSER 的完整参数，丢弃空字符串规则。
// 空规则会被服务端当成非法修饰符，导致整条命令失败，必须在客户端先剔除。
func buildAclSetUserArgs(user string, rules []string) []interface{} {
	args := make([]interface{}, 0, len(rules)+3)
	args = append(args, "acl", "setuser", user)
	for _, rule := range rules {
		if strings.TrimSpace(rule) == "" {
			continue
		}
		args = append(args, rule)
	}
	return args
}

// AclDeleteUser 删除用户（ACL DELUSER），禁止删除当前连接所使用的用户与 default。
func (b *browserService) AclDeleteUser(name, user string) (resp types.JSResp) {
	if user == "" {
		resp.Msg = "user is required"
		return
	}
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	whoamiRaw, err := b.aclRead(item.ctx, item.client, "ACL", "WHOAMI")
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	whoami := toText(whoamiRaw)
	if err = validateAclDelete(whoami, user); err != nil {
		resp.Msg = err.Error()
		return
	}
	removed, err := b.aclDeleteUser(item.ctx, item.client, user)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	if removed == 0 {
		resp.Msg = "user not found"
		return
	}
	// 集群下每个分片各删一次，对外仍表达"删除了 1 个用户"
	resp.Success = true
	resp.Data = map[string]any{"deleted": 1}
	return
}

// aclDeleteUser 执行 ACL DELUSER 并返回实际删除数量，
// 集群模式下逐分片累加，用于识别"用户本就不存在"的情况。
func (b *browserService) aclDeleteUser(ctx context.Context, client redis.UniversalClient, user string) (int64, error) {
	if cc, ok := client.(*redis.ClusterClient); ok {
		var total int64
		var mu sync.Mutex // ForEachShard 并发执行，累加必须加锁
		err := cc.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
			removed, err := shard.Do(ctx, "ACL", "DELUSER", user).Int64()
			mu.Lock()
			total += removed
			mu.Unlock()
			return err
		})
		return total, err
	}
	return client.Do(ctx, "ACL", "DELUSER", user).Int64()
}

// validateAclDelete 校验用户是否允许被删除：
// 空用户名、当前连接使用的用户、default 用户都不允许删除。
func validateAclDelete(whoami, user string) error {
	if user == "" {
		return errors.New("user is required")
	}
	if user == whoami {
		return errors.New("cannot delete the user used by the current connection")
	}
	if user == "default" {
		return errors.New("the default user cannot be deleted")
	}
	return nil
}

// AclLog 返回最近的 ACL 拒绝事件。
func (b *browserService) AclLog(name string, count int) (resp types.JSResp) {
	count = clampAclLogCount(count)
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	client, ctx := item.client, item.ctx

	raw, err := b.aclFirst(ctx, client, "acl", "log", count)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	entries := normalizeAclLog(raw)

	resp.Success = true
	resp.Data = entries
	return
}

// clampAclLogCount 把 ACL LOG 的条数限制在 1-200，非法值回退到默认 20。
func clampAclLogCount(count int) int {
	if count <= 0 || count > 200 {
		return 20
	}
	return count
}

// AclDryRun 校验某个用户能否执行给定命令（ACL DRYRUN）。
func (b *browserService) AclDryRun(name, user string, command []string) (resp types.JSResp) {
	if user == "" || len(command) == 0 {
		resp.Msg = "user and command are required"
		return
	}
	item, err := b.getRedisClient(name, -1)
	if err != nil {
		resp.Msg = err.Error()
		return
	}
	client, ctx := item.client, item.ctx

	args := append([]interface{}{"acl", "dryrun", user}, stringsToInterfaces(command)...)
	result, err := b.aclFirst(ctx, client, args...)
	allowed, message, failure := normalizeDryRunResult(result, err)
	if failure != "" {
		resp.Msg = failure
		return
	}

	resp.Success = true
	resp.Data = map[string]any{"allowed": allowed, "message": message}
	return
}

// normalizeDryRunResult 把 ACL DRYRUN 的返回规整成判定结果。
// 权限不足在部分版本以错误返回、在 Redis 7.2 以普通文本返回，两者都属于判定结果；
// 只有真正的执行错误才通过 failure 抛出。
func normalizeDryRunResult(result interface{}, err error) (allowed bool, message, failure string) {
	text := toText(result)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "has no permissions") {
			return false, msg, ""
		}
		return false, "", msg
	}
	return strings.EqualFold(text, "OK"), text, ""
}

// normalizeAclLog 把 ACL LOG 的返回（数组套数组/映射）规整成键值对数组。
func normalizeAclLog(v interface{}) []map[string]string {
	entries, ok := v.([]interface{})
	if !ok {
		return nil
	}
	ret := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		m := map[string]string{}
		switch data := entry.(type) {
		case []interface{}:
			for i := 0; i+1 < len(data); i += 2 {
				key := toText(data[i])
				m[key] = toText(data[i+1])
			}
		case map[interface{}]interface{}:
			for k, val := range data {
				m[toText(k)] = toText(val)
			}
		}
		if len(m) > 0 {
			ret = append(ret, m)
		}
	}
	return ret
}

// parseAclListLine 解析 "user <name> <flags> <passwords> <keys> <channels> <commands>"
func parseAclListLine(line string) (aclUserInfo, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "user" {
		return aclUserInfo{}, false
	}

	user := aclUserInfo{Name: fields[1]}
	commands := make([]string, 0, 8)
	for _, f := range fields[2:] {
		switch {
		case strings.HasPrefix(f, "#"):
			user.Passwords++
		case strings.HasPrefix(f, "~"), strings.HasPrefix(f, "%R~"), strings.HasPrefix(f, "%W~"):
			user.Keys = joinNonEmpty(user.Keys, f, " ")
		case strings.HasPrefix(f, "&"):
			user.Channels = joinNonEmpty(user.Channels, f, " ")
		case strings.HasPrefix(f, "+"), strings.HasPrefix(f, "-"):
			commands = append(commands, f)
		case aclFlagNames[f]:
			user.Flags = append(user.Flags, f)
			if f == "on" {
				user.Enabled = true
			}
		default:
			user.Flags = append(user.Flags, f)
		}
	}
	user.Commands = strings.Join(commands, " ")
	return user, true
}

// toStringMap 兼容 RESP2 的扁平数组与 RESP3 的 map 两种返回。
func toStringMap(v interface{}) map[string]interface{} {
	ret := map[string]interface{}{}
	switch data := v.(type) {
	case map[interface{}]interface{}:
		for k, val := range data {
			if ks, ok := k.(string); ok {
				ret[ks] = val
			}
		}
	case []interface{}:
		for i := 0; i+1 < len(data); i += 2 {
			if ks, ok := data[i].(string); ok {
				ret[ks] = data[i+1]
			}
		}
	}
	return ret
}

// toSlice 把数组值规整成字符串数组，字符串值则按空格切分。
func toSlice(v interface{}) []string {
	switch data := v.(type) {
	case []interface{}:
		ret := make([]string, 0, len(data))
		for _, it := range data {
			if s, ok := it.(string); ok {
				ret = append(ret, s)
			}
		}
		return ret
	case string:
		if strings.TrimSpace(data) == "" {
			return nil
		}
		return strings.Fields(data)
	}
	return nil
}

// toText 把命令返回值转成展示文本。
// ACL LOG 的条目里混有整型字段（例如 count），必须一并转换，否则会变成空字符串。
func toText(v interface{}) string {
	switch data := v.(type) {
	case string:
		return data
	case int:
		return strconv.Itoa(data)
	case int64:
		return strconv.FormatInt(data, 10)
	case float64:
		return strconv.FormatFloat(data, 'f', -1, 64)
	case []interface{}:
		return strings.Join(toSlice(data), " ")
	}
	return ""
}

func joinNonEmpty(current, value, sep string) string {
	if current == "" {
		return value
	}
	return current + sep + value
}

func stringsToInterfaces(items []string) []interface{} {
	ret := make([]interface{}, 0, len(items))
	for _, it := range items {
		ret = append(ret, it)
	}
	return ret
}
