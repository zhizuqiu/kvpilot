package services

import (
	"os"
	"testing"
)

// 验证 INFO keyspace 解析出每个 db 的键数、过期数与平均 TTL。
func TestParseKeyspaceDetail(t *testing.T) {
	text := "# Keyspace\r\n" +
		"db0:keys=120,expires=10,avg_ttl=3600000\r\n" +
		"db3:keys=5,expires=5,avg_ttl=500\r\n"
	rows := parseKeyspaceDetail("10.0.0.1:6379", text)
	if len(rows) != 2 {
		t.Fatalf("解析数量 = %d, want 2 (%+v)", len(rows), rows)
	}
	if rows[0].DB != 0 || rows[0].Keys != 120 || rows[0].Expires != 10 || rows[0].AvgTTL != 3600000 {
		t.Fatalf("db0 字段错误: %+v", rows[0])
	}
	if rows[1].DB != 3 || rows[1].Keys != 5 || rows[1].Node != "10.0.0.1:6379" {
		t.Fatalf("db3 字段错误: %+v", rows[1])
	}
}

// 验证无 TTL 与带 TTL 的统计口径：TTL 为 -1 表示永不过期，正数表示有过期时间。
func TestSampleTTLClassify(t *testing.T) {
	if !keyWithoutTTL(-1) || keyWithoutTTL(0) || keyWithoutTTL(100) || keyWithoutTTL(-2) {
		t.Fatal("TTL 分类判断错误")
	}
	if keyWithTTL(-1) || keyWithTTL(-2) || !keyWithTTL(1) {
		t.Fatal("带 TTL 判断错误")
	}
}

// 验证前缀提取：优先取第一个分隔符前的片段，无分隔符时使用完整键名。
func TestKeyPrefix(t *testing.T) {
	cases := map[string]string{
		"user:1001:profile": "user",
		"session":           "session",
		":leading":          "",
		"a:b:c":             "a",
	}
	for key, want := range cases {
		if got := keyPrefix(key); got != want {
			t.Fatalf("keyPrefix(%q) = %q, want %q", key, got, want)
		}
	}
}

// 验证前缀统计只保留数量最多的若干项，并按数量倒序。
func TestTopPrefixes(t *testing.T) {
	counts := map[string]int64{"user": 10, "order": 3, "cache": 7, "tmp": 1}
	top := topPrefixes(counts, 3)
	if len(top) != 3 {
		t.Fatalf("数量 = %d, want 3", len(top))
	}
	if top[0].Prefix != "user" || top[0].Count != 10 {
		t.Fatalf("排序错误: %+v", top)
	}
	if top[1].Prefix != "cache" || top[2].Prefix != "order" {
		t.Fatalf("TopN 顺序错误: %+v", top)
	}
}

// 验证真实实例的键空间统计：数据库行、采样分布与 TTL 分类互相一致。
func TestIntegrationKeyspaceStats(t *testing.T) {
	svc, name := newEndpointService(t)
	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") == "1" {
		_, cleanup := seedBigKeys(t, svc.connMap[name].client, 50)
		t.Cleanup(cleanup)
	}

	resp := svc.GetKeyspaceStats(name, 200)
	if !resp.Success {
		t.Fatalf("GetKeyspaceStats 失败: %s", resp.Msg)
	}
	data, ok := resp.Data.(keyspaceData)
	if !ok {
		t.Fatalf("返回类型应为 keyspaceData, got %T", resp.Data)
	}
	if len(data.Nodes) == 0 {
		t.Fatal("节点列表不应为空")
	}
	if len(data.Databases) == 0 {
		t.Log("目标实例没有 key，跳过分布断言")
		return
	}

	for _, row := range data.Databases {
		if row.Node == "" || row.Keys <= 0 {
			t.Fatalf("数据库行不完整: %+v", row)
		}
	}
	if data.Sampled == 0 {
		t.Fatal("有 key 的实例应产生采样结果")
	}
	if data.NoTTL+data.WithTTL > data.Sampled {
		t.Fatalf("TTL 分类超过采样数: noTtl=%d withTtl=%d sampled=%d", data.NoTTL, data.WithTTL, data.Sampled)
	}
	if len(data.Types) == 0 || len(data.Prefixes) == 0 {
		t.Fatalf("采样后应有类型与前缀分布: %+v", data)
	}
	// 探针键没有设置过期时间，用于验证 TTL 分类没有被 go-redis 的时长表示误导
	if os.Getenv("KVPILOT_TEST_REDIS_SEED_BIGKEY") == "1" && data.NoTTL == 0 {
		t.Fatalf("探针键没有 TTL，noTtl 应大于 0: %+v", data)
	}
}
