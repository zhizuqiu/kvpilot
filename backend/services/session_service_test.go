package services

import (
	"testing"
)

// 验证 CLIENT LIST 文本解析：字段类型、空 name、命令中的 | 分隔符、Node 归属。
func TestParseClientList(t *testing.T) {
	text := "id=12 addr=10.0.0.8:51234 laddr=10.0.0.1:6379 fd=9 name= age=120 idle=3 flags=N db=2 sub=0 psub=0 ssub=0 multi=-1 watch=0 qbuf=26 qbuf-free=20448 argv-mem=10 multi-mem=0 tot-net-in=0 tot-net-out=0 rbs=1024 rbp=0 obl=0 oll=0 omem=0 tot-mem=0 events=r cmd=client|list user=default redir=-1 resp=2\n" +
		"id=13 addr=10.0.0.9:40000 laddr=10.0.0.1:6379 fd=10 name=worker age=300 idle=120 flags=S db=0 sub=0 psub=0 ssub=0 multi=-1 watch=0 qbuf=0 qbuf-free=0 argv-mem=0 multi-mem=0 tot-net-in=0 tot-net-out=0 rbs=0 rbp=0 obl=0 oll=0 omem=0 tot-mem=0 events=r cmd=psync user=default redir=-1 resp=2\n" +
		"\n"
	list := parseClientList("10.0.0.1:6379", text)
	if len(list) != 2 {
		t.Fatalf("解析数量 = %d, want 2", len(list))
	}

	first := list[0]
	if first.Node != "10.0.0.1:6379" || first.ID != 12 || first.DB != 2 || first.Age != 120 || first.Idle != 3 {
		t.Fatalf("首条会话字段错误: %+v", first)
	}
	if first.Addr != "10.0.0.8:51234" || first.Cmd != "client|list" || first.User != "default" {
		t.Fatalf("首条会话文本字段错误: %+v", first)
	}
	if first.Name != "" {
		t.Fatalf("空 name 应为空字符串, got %q", first.Name)
	}
	if first.Protected {
		t.Fatal("普通连接不应标记为受保护")
	}

	second := list[1]
	if second.Name != "worker" || !second.Protected {
		t.Fatalf("从库链路应带 name 并标记受保护: %+v", second)
	}
}

// 验证无法解析的行被跳过，不会产生零值会话。
func TestParseClientListSkipsInvalidLines(t *testing.T) {
	text := "garbage line without fields\n" +
		"id=abc addr=1.1.1.1:1 flags=N db=0 age=1 idle=0\n" +
		"id=7 addr=2.2.2.2:2 flags=N db=0 age=9 idle=0\n"
	list := parseClientList("node:1", text)
	if len(list) != 1 || list[0].ID != 7 {
		t.Fatalf("应只保留可解析的会话, got %+v", list)
	}
}

// 验证受保护标记：主从/哨兵链路（S/M/O）不可关闭，普通与发布订阅连接可关闭。
func TestClientFlagsProtected(t *testing.T) {
	cases := map[string]bool{
		"N":  false,
		"P":  false,
		"":   false,
		"S":  true,
		"M":  true,
		"O":  true,
		"s":  true,
		"Sm": true,
		"Px": false,
		"NU": false,
	}
	for flags, want := range cases {
		if got := clientFlagsProtected(flags); got != want {
			t.Fatalf("flags %q 保护判断 = %v, want %v", flags, got, want)
		}
	}
}

// 验证概览统计：总数、空闲阈值计数、节点与未响应列表、来源 IP 聚合排序。
func TestBuildSessionData(t *testing.T) {
	views := map[string][]sessionItem{
		"10.0.0.1:6379": {
			{Node: "10.0.0.1:6379", ID: 2, Addr: "10.0.0.2:50001", Age: 100, Idle: 100},
			{Node: "10.0.0.1:6379", ID: 1, Addr: "10.0.0.2:50000", Age: 5, Idle: 0},
		},
		"10.0.0.3:6379": {
			{Node: "10.0.0.3:6379", ID: 9, Addr: "10.0.0.4:50002", Age: 80, Idle: 61},
		},
	}

	data := buildSessionData(views, []string{"10.0.0.5:6379"})
	if data.Total != 3 {
		t.Fatalf("总数 = %d, want 3", data.Total)
	}
	if data.Idle != 2 {
		t.Fatalf("空闲数 = %d, want 2（idle >= 60）", data.Idle)
	}
	if len(data.Nodes) != 2 || data.Nodes[0] != "10.0.0.1:6379" || data.Nodes[1] != "10.0.0.3:6379" {
		t.Fatalf("节点列表错误: %v", data.Nodes)
	}
	if len(data.Unavailable) != 1 || data.Unavailable[0] != "10.0.0.5:6379" {
		t.Fatalf("未响应列表错误: %v", data.Unavailable)
	}
	if len(data.Sessions) != 3 || data.Sessions[0].ID != 1 {
		t.Fatalf("会话应按节点与 id 排序: %+v", data.Sessions)
	}
	if len(data.Sources) != 2 || data.Sources[0].Addr != "10.0.0.2" || data.Sources[0].Count != 2 {
		t.Fatalf("来源聚合错误: %+v", data.Sources)
	}
}

// 验证连接名的 URL 编码会被还原，避免界面出现 %%E4%%B8%%B4 这类内容。
func TestParseClientListDecodesName(t *testing.T) {
	text := "id=21 addr=1.1.1.1:1 name=%E4%B8%B4%E6%97%B6 flags=N db=0 age=3 idle=0\n"
	list := parseClientList("node:1", text)
	if len(list) != 1 {
		t.Fatalf("解析数量 = %d, want 1", len(list))
	}
	if list[0].Name != "临时" {
		t.Fatalf("连接名 = %q, want 临时", list[0].Name)
	}
}
