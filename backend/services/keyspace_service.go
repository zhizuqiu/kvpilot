package services

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
	"tinyrdm/backend/types"

	"github.com/redis/go-redis/v9"
)

const (
	keyspaceDefaultSample = 1000
	keyspacePrefixLimit   = 10
)

// keyspaceDBRow 是单个节点上某个数据库的键数量与过期信息。
type keyspaceDBRow struct {
	Node    string `json:"node"`
	DB      int    `json:"db"`
	Keys    int64  `json:"keys"`
	Expires int64  `json:"expires"`
	AvgTTL  int64  `json:"avgTtl"`
}

type keyspaceTypeRow struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type keyspacePrefixRow struct {
	Prefix string `json:"prefix"`
	Count  int64  `json:"count"`
}

// keyspaceData 是键空间统计接口的返回体。
type keyspaceData struct {
	Nodes       []string            `json:"nodes"`
	Databases   []keyspaceDBRow     `json:"databases"`
	Unavailable []string            `json:"unavailable,omitempty"`
	Sampled     int64               `json:"sampled"`
	NoTTL       int64               `json:"noTtl"`
	WithTTL     int64               `json:"withTtl"`
	Types       []keyspaceTypeRow   `json:"types"`
	Prefixes    []keyspacePrefixRow `json:"prefixes"`
	ElapsedMS   int64               `json:"elapsedMs"`
}

// parseKeyspaceDetail 解析 INFO keyspace，保留每个 db 的键数、过期数与平均 TTL。
func parseKeyspaceDetail(node, info string) []keyspaceDBRow {
	rows := make([]keyspaceDBRow, 0, 4)
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "db") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		db, err := strconv.Atoi(strings.TrimPrefix(name, "db"))
		if err != nil {
			continue
		}

		row := keyspaceDBRow{Node: node, DB: db}
		for _, field := range strings.Split(rest, ",") {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			number, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}
			switch key {
			case "keys":
				row.Keys = number
			case "expires":
				row.Expires = number
			case "avg_ttl":
				row.AvgTTL = number
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// keyWithoutTTL 判断采样键是否永不过期（TTL 返回 -1）。
// go-redis 对 -1/-2 直接返回 time.Duration(-1)/time.Duration(-2)，其余按秒换算。
func keyWithoutTTL(ttl time.Duration) bool {
	return ttl == -1
}

// keyWithTTL 判断采样键是否设置了过期时间。
func keyWithTTL(ttl time.Duration) bool {
	return ttl > 0
}

// keyPrefix 取第一个分隔符前的片段作为前缀，无分隔符时使用完整键名。
func keyPrefix(key string) string {
	if index := strings.Index(key, ":"); index >= 0 {
		return key[:index]
	}
	return key
}

// topPrefixes 返回数量最多的前缀，按数量倒序、同数量按名称排序。
func topPrefixes(counts map[string]int64, limit int) []keyspacePrefixRow {
	rows := make([]keyspacePrefixRow, 0, len(counts))
	for prefix, count := range counts {
		rows = append(rows, keyspacePrefixRow{Prefix: prefix, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Prefix < rows[j].Prefix
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

// GetKeyspaceStats 汇总各节点的数据库规模，并按采样结果给出类型、TTL 与前缀分布。
func (b *browserService) GetKeyspaceStats(server string, sampleSize int) (resp types.JSResp) {
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

	if sampleSize < 0 {
		sampleSize = 0
	}

	ctx := item.ctx
	data := keyspaceData{
		Nodes:     make([]string, 0, len(targets)),
		Databases: make([]keyspaceDBRow, 0, 8),
		Types:     make([]keyspaceTypeRow, 0, 8),
		Prefixes:  make([]keyspacePrefixRow, 0, keyspacePrefixLimit),
	}
	typeCounts := map[string]int64{}
	prefixCounts := map[string]int64{}

	for _, target := range targets {
		base, ok := target.Client.(*redis.Client)
		if !ok {
			continue
		}

		rows, err := b.keyspaceDetail(ctx, target.Addr, base.Options())
		if err != nil {
			data.Unavailable = append(data.Unavailable, target.Addr)
			continue
		}
		data.Nodes = append(data.Nodes, target.Addr)
		data.Databases = append(data.Databases, rows...)

		if sampleSize <= 0 || len(rows) == 0 {
			continue
		}

		budget := sampleSize
		for index, row := range rows {
			quota := bigKeyBudget(budget, len(rows)-index)
			if quota <= 0 {
				break
			}
			sampled, err := sampleKeyspace(ctx, target.Addr, base.Options(), row.DB, quota, typeCounts, prefixCounts, &data.NoTTL, &data.WithTTL)
			if err != nil {
				data.Unavailable = append(data.Unavailable, target.Addr)
				break
			}
			data.Sampled += int64(sampled)
			budget -= sampled
			if budget <= 0 {
				break
			}
		}
	}

	sort.Strings(data.Nodes)
	sort.Strings(data.Unavailable)
	sort.Slice(data.Databases, func(i, j int) bool {
		if data.Databases[i].Node != data.Databases[j].Node {
			return data.Databases[i].Node < data.Databases[j].Node
		}
		return data.Databases[i].DB < data.Databases[j].DB
	})

	for keyType, count := range typeCounts {
		data.Types = append(data.Types, keyspaceTypeRow{Type: keyType, Count: count})
	}
	sort.Slice(data.Types, func(i, j int) bool {
		if data.Types[i].Count != data.Types[j].Count {
			return data.Types[i].Count > data.Types[j].Count
		}
		return data.Types[i].Type < data.Types[j].Type
	})
	data.Prefixes = topPrefixes(prefixCounts, keyspacePrefixLimit)

	data.ElapsedMS = time.Since(started).Milliseconds()
	resp.Success = true
	resp.Data = data
	return
}

// keyspaceDetail 读取单个节点的 INFO keyspace。
func (b *browserService) keyspaceDetail(ctx context.Context, addr string, base *redis.Options) ([]keyspaceDBRow, error) {
	client := redis.NewClient(configNodeOptionsWithDB(addr, base, 0))
	defer client.Close()

	info, err := client.Info(ctx, "keyspace").Result()
	if err != nil {
		return nil, err
	}
	return parseKeyspaceDetail(addr, info), nil
}

// sampleKeyspace 在指定数据库采样，累计类型、前缀与 TTL 分布。
// TYPE 与 TTL 按批流水线发送，远程实例下每个键只需一次往返批次的摊薄成本。
func sampleKeyspace(ctx context.Context, addr string, base *redis.Options, db, quota int, types map[string]int64, prefixes map[string]int64, noTTL, withTTL *int64) (int, error) {
	client := redis.NewClient(configNodeOptionsWithDB(addr, base, db))
	defer client.Close()

	cursor := uint64(0)
	sampled := 0
	for {
		if ctx.Err() != nil {
			return sampled, nil
		}

		batch := bigKeyScanBatch
		if quota-sampled < batch {
			batch = quota - sampled
		}
		if batch <= 0 {
			return sampled, nil
		}

		keys, next, err := client.Scan(ctx, cursor, "*", int64(batch)).Result()
		if err != nil {
			return sampled, err
		}
		if len(keys) == 0 {
			cursor = next
			if cursor == 0 {
				return sampled, nil
			}
			continue
		}

		typeCmds := make([]*redis.StatusCmd, len(keys))
		ttlCmds := make([]*redis.DurationCmd, len(keys))
		pipe := client.Pipeline()
		for i, key := range keys {
			typeCmds[i] = pipe.Type(ctx, key)
			ttlCmds[i] = pipe.TTL(ctx, key)
		}
		_, _ = pipe.Exec(ctx)

		for i, key := range keys {
			keyType, err := typeCmds[i].Result()
			if err != nil || keyType == "none" {
				continue
			}
			types[keyType]++
			prefixes[keyPrefix(key)]++

			if ttl, err := ttlCmds[i].Result(); err == nil {
				switch {
				case keyWithoutTTL(ttl):
					*noTTL = *noTTL + 1
				case keyWithTTL(ttl):
					*withTTL = *withTTL + 1
				}
			}

			sampled++
			if sampled >= quota {
				return sampled, nil
			}
		}

		cursor = next
		if cursor == 0 {
			return sampled, nil
		}
	}
}

// ExportCSV 把任意表格写成 CSV，供键空间统计等结果导出使用。
func (b *browserService) ExportCSV(path string, header []string, rows [][]string) (resp types.JSResp) {
	path = strings.TrimSpace(path)
	if path == "" {
		resp.Msg = "path is required"
		return
	}
	if err := writeCSV(path, header, rows); err != nil {
		resp.Msg = err.Error()
		return
	}
	resp.Success = true
	resp.Data = map[string]any{"path": path, "count": len(rows)}
	return
}
