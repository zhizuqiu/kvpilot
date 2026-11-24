package services

import (
	"testing"
)

// buildCheckInfo 组装巡检所需的 INFO 结构。
func buildCheckInfo(values map[string]map[string]string) map[string]map[string]string {
	info := map[string]map[string]string{}
	for section, fields := range values {
		info[section] = map[string]string{}
		for key, value := range fields {
			info[section][key] = value
		}
	}
	return info
}

func itemByRule(items []healthItem, rule string) *healthItem {
	for i := range items {
		if items[i].Rule == rule {
			return &items[i]
		}
	}
	return nil
}

// 验证健康节点的所有规则均为 ok。
func TestEvaluateHealthAllOK(t *testing.T) {
	info := buildCheckInfo(map[string]map[string]string{
		"Memory":      {"used_memory": "104857600", "mem_fragmentation_ratio": "1.10"},
		"Stats":       {"evicted_keys": "0"},
		"Persistence": {"rdb_last_bgsave_status": "ok", "aof_enabled": "0"},
		"Replication": {"role": "master", "connected_slaves": "1"},
		"Clients":     {"blocked_clients": "0"},
		"Cluster":     {"cluster_state": "ok"},
	})
	config := map[string]string{
		"maxmemory":               "1073741824",
		"maxmemory-policy":        "allkeys-lru",
		"appendonly":              "no",
		"save":                    "3600 1",
		"protected-mode":          "yes",
		"slowlog-log-slower-than": "10000",
		"requirepass":             "set",
	}

	items := evaluateHealth("10.0.0.1:6379", info, config, "cluster")
	for _, item := range items {
		if item.Severity != "ok" {
			t.Fatalf("健康节点规则 %s 应为 ok, got %s (%+v)", item.Rule, item.Severity, item.Values)
		}
	}
	if itemByRule(items, "cluster") == nil {
		t.Fatal("集群模式应包含 cluster 规则")
	}
}

// 验证容量、淘汰、阻塞、慢日志与密码相关的告警能同时命中。
func TestEvaluateHealthWarnings(t *testing.T) {
	info := buildCheckInfo(map[string]map[string]string{
		"Memory":      {"used_memory": "943718400", "mem_fragmentation_ratio": "1.70"},
		"Stats":       {"evicted_keys": "10"},
		"Persistence": {"rdb_last_bgsave_status": "ok", "aof_enabled": "0"},
		"Replication": {"role": "master", "connected_slaves": "0"},
		"Clients":     {"blocked_clients": "3"},
		"Cluster":     {"cluster_state": "ok"},
	})
	config := map[string]string{
		"maxmemory":               "1073741824",
		"maxmemory-policy":        "noeviction",
		"appendonly":              "no",
		"save":                    "",
		"protected-mode":          "yes",
		"slowlog-log-slower-than": "-1",
		"requirepass":             "",
	}

	items := evaluateHealth("10.0.0.1:6379", info, config, "cluster")
	expect := map[string]string{
		"fragmentation":      "warn",
		"maxmemory":          "warn",
		"policy":             "warn",
		"evicted":            "warn",
		"replication":        "warn",
		"blocked":            "warn",
		"slowlog":            "warn",
		"auth":               "warn",
		"persistence_config": "warn",
		"cluster":            "ok",
	}
	for rule, severity := range expect {
		item := itemByRule(items, rule)
		if item == nil {
			t.Fatalf("缺少规则 %s", rule)
		}
		if item.Severity != severity {
			t.Fatalf("规则 %s = %s, want %s (%+v)", rule, item.Severity, severity, item.Values)
		}
	}
}

// 验证严重问题：碎片率过高、容量接近上限、持久化失败、复制中断、空密码且关闭保护模式、集群异常。
func TestEvaluateHealthErrors(t *testing.T) {
	info := buildCheckInfo(map[string]map[string]string{
		"Memory":      {"used_memory": "1105199104", "mem_fragmentation_ratio": "2.50"},
		"Stats":       {"evicted_keys": "0"},
		"Persistence": {"rdb_last_bgsave_status": "err", "aof_enabled": "1", "aof_last_write_status": "err"},
		"Replication": {"role": "slave", "master_link_status": "down", "master_last_io_seconds_ago": "30"},
		"Clients":     {"blocked_clients": "0"},
		"Cluster":     {"cluster_state": "fail"},
	})
	config := map[string]string{
		"maxmemory":               "1073741824",
		"maxmemory-policy":        "allkeys-lru",
		"appendonly":              "yes",
		"save":                    "3600 1",
		"protected-mode":          "no",
		"slowlog-log-slower-than": "10000",
		"requirepass":             "",
	}

	items := evaluateHealth("10.0.0.2:6379", info, config, "cluster")
	expect := map[string]string{
		"fragmentation":      "error",
		"maxmemory":          "error",
		"persistence":        "error",
		"replication":        "error",
		"auth":               "error",
		"cluster":            "error",
		"persistence_config": "ok",
	}
	for rule, severity := range expect {
		item := itemByRule(items, rule)
		if item == nil {
			t.Fatalf("缺少规则 %s", rule)
		}
		if item.Severity != severity {
			t.Fatalf("规则 %s = %s, want %s (%+v)", rule, item.Severity, severity, item.Values)
		}
	}
}

// 验证未设置 maxmemory 时给出告警，而不是除零或误报正常。
func TestEvaluateHealthNoMaxmemory(t *testing.T) {
	info := buildCheckInfo(map[string]map[string]string{
		"Memory":      {"used_memory": "1048576", "mem_fragmentation_ratio": "1.0"},
		"Stats":       {"evicted_keys": "0"},
		"Persistence": {"rdb_last_bgsave_status": "ok"},
		"Replication": {"role": "master", "connected_slaves": "1"},
		"Clients":     {"blocked_clients": "0"},
	})
	config := map[string]string{"maxmemory": "0", "maxmemory-policy": "noeviction", "save": "3600 1", "protected-mode": "yes", "slowlog-log-slower-than": "10000", "requirepass": "set"}

	items := evaluateHealth("10.0.0.3:6379", info, config, "standalone")
	item := itemByRule(items, "maxmemory")
	if item == nil || item.Severity != "warn" {
		t.Fatalf("未设置 maxmemory 应告警: %+v", item)
	}
	if itemByRule(items, "cluster") != nil {
		t.Fatal("非集群模式不应产生 cluster 规则")
	}
}
