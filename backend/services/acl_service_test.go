package services

import (
	"errors"
	"reflect"
	"testing"
)

// 验证 ACL LIST 单行解析：flags、密码个数、key 模式、频道模式与命令规则必须分开归类，
// 否则前端编辑器会把 &频道 误当成 key 规则。
func TestParseAclListLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want aclUserInfo
		ok   bool
	}{
		{
			name: "default 用户：nopass 与全权限",
			line: "user default on nopass sanitize-payload ~* &* +@all",
			want: aclUserInfo{
				Name:     "default",
				Enabled:  true,
				Flags:    []string{"on", "nopass", "sanitize-payload"},
				Keys:     "~*",
				Channels: "&*",
				Commands: "+@all",
			},
			ok: true,
		},
		{
			name: "多密码用户：读写分离 key、频道、命令与状态标志",
			line: "user app off #a1b2 #c3d4 ~cache:* %R~read:* %W~write:* &chan:* +get +set -flushall resetkeys",
			want: aclUserInfo{
				Name:      "app",
				Enabled:   false,
				Flags:     []string{"off", "resetkeys"},
				Passwords: 2,
				Keys:      "~cache:* %R~read:* %W~write:*",
				Channels:  "&chan:*",
				Commands:  "+get +set -flushall",
			},
			ok: true,
		},
		{
			name: "只有名字的用户：所有规则为空",
			line: "user empty",
			want: aclUserInfo{Name: "empty"},
			ok:   true,
		},
		{
			name: "非 user 开头的行被丢弃",
			line: "something else",
			ok:   false,
		},
		{
			name: "字段不足的行被丢弃，避免越界",
			line: "user",
			ok:   false,
		},
		{
			name: "空行被丢弃",
			line: "",
			ok:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseAclListLine(c.line)
			if ok != c.ok {
				t.Fatalf("parseAclListLine(%q) ok = %v, want %v", c.line, ok, c.ok)
			}
			if !c.ok {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseAclListLine(%q)\n got %+v\nwant %+v", c.line, got, c.want)
			}
		})
	}
}

// 验证跨分片聚合：同名用户只保留一行并统计出现节点数，列表按用户名稳定排序。
func TestAggregateAclUsersAcrossShards(t *testing.T) {
	results := []aclShardResult{
		{
			Addr: "192.168.1.118:16370",
			Value: []interface{}{
				"user default on nopass ~* &* +@all",
				"user app on ~cache:* +get",
			},
		},
		{
			Addr: "192.168.1.118:16371",
			Value: []interface{}{
				"user default on nopass ~* &* +@all",
			},
		},
	}

	users, err := aggregateAclUsers(results)
	if err != nil {
		t.Fatalf("不应返回错误, got %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("用户数 = %d, want 2（同名用户需合并）", len(users))
	}
	if users[0].Name != "app" || users[1].Name != "default" {
		t.Fatalf("用户未按名称升序排列: %s, %s", users[0].Name, users[1].Name)
	}
	if users[1].Nodes != 2 || users[1].TotalNodes != 2 {
		t.Fatalf("default 应存在于 2/2 个节点, got %d/%d", users[1].Nodes, users[1].TotalNodes)
	}
	if users[0].Nodes != 1 || users[0].TotalNodes != 2 {
		t.Fatalf("app 应只存在于 1/2 个节点, got %d/%d", users[0].Nodes, users[0].TotalNodes)
	}
}

// 验证部分节点失败时仍返回其余节点的数据，并保留首个错误；
// 集群里单节点 ACL 读取失败不能让整个列表变空。
func TestAggregateAclUsersPartialFailure(t *testing.T) {
	results := []aclShardResult{
		{Addr: "192.168.1.118:16370", Value: []interface{}{"user app on +get"}},
		{Addr: "192.168.1.118:16371", Err: errors.New("connection refused")},
	}

	users, err := aggregateAclUsers(results)
	if err == nil {
		t.Fatal("应返回失败分片的错误")
	}
	if len(users) != 1 {
		t.Fatalf("应保留成功分片的用户, got %d: %+v", len(users), users)
	}
	if users[0].TotalNodes != 2 {
		t.Fatalf("TotalNodes 应统计全部分片, got %d", users[0].TotalNodes)
	}
	if users[0].Nodes != 1 {
		t.Fatalf("app 只出现 1 次, got %d", users[0].Nodes)
	}
}

// 验证全部分片失败时返回空列表与错误，由调用方转成失败响应。
func TestAggregateAclUsersAllFailed(t *testing.T) {
	results := []aclShardResult{
		{Addr: "a", Err: errors.New("boom")},
		{Addr: "b", Err: errors.New("boom2")},
	}

	users, err := aggregateAclUsers(results)
	if err == nil {
		t.Fatal("全部分片失败时应返回错误")
	}
	if len(users) != 0 {
		t.Fatalf("不应产生用户, got %+v", users)
	}
}

// 验证返回结构中混入的非字符串元素被忽略，不会 panic。
func TestAggregateAclUsersIgnoresNonString(t *testing.T) {
	results := []aclShardResult{
		{Addr: "a", Value: []interface{}{123, nil, "user app on +get"}},
	}

	users, err := aggregateAclUsers(results)
	if err != nil {
		t.Fatalf("不应返回错误, got %v", err)
	}
	if len(users) != 1 || users[0].Name != "app" {
		t.Fatalf("应只解析出 app, got %+v", users)
	}
}

// 验证 ACL LOG 的两种 RESP 形态都能规整成键值对：
// RESP2 返回数组套数组，RESP3 返回 map。
func TestNormalizeAclLog(t *testing.T) {
	resp2 := []interface{}{
		[]interface{}{
			"count", int64(3),
			"reason", "command",
			"context", "toplevel",
		},
		[]interface{}{"count", int64(1)},
	}
	got := normalizeAclLog(resp2)
	if len(got) != 2 {
		t.Fatalf("条目数 = %d, want 2", len(got))
	}
	if got[0]["reason"] != "command" || got[0]["context"] != "toplevel" {
		t.Fatalf("RESP2 解析不正确: %+v", got[0])
	}
	if got[0]["count"] != "3" {
		t.Fatalf("整数值应转成字符串, got %q", got[0]["count"])
	}

	resp3 := []interface{}{
		map[interface{}]interface{}{"reason": "auth", "object": "app"},
	}
	got = normalizeAclLog(resp3)
	if len(got) != 1 || got[0]["reason"] != "auth" || got[0]["object"] != "app" {
		t.Fatalf("RESP3 解析不正确: %+v", got)
	}
}

// 验证 ACL LOG 的空条目与非数组返回值不会产生空行或被当成错误。
func TestNormalizeAclLogEdgeCases(t *testing.T) {
	if got := normalizeAclLog(nil); got != nil {
		t.Fatalf("nil 输入应返回 nil, got %+v", got)
	}
	if got := normalizeAclLog("unexpected"); got != nil {
		t.Fatalf("非数组输入应返回 nil, got %+v", got)
	}

	got := normalizeAclLog([]interface{}{
		[]interface{}{},
		[]interface{}{"count"},
		"plain",
		[]interface{}{"reason", "command"},
	})
	if len(got) != 1 || got[0]["reason"] != "command" {
		t.Fatalf("空条目与非数组条目应被跳过, got %+v", got)
	}
}

// 验证 ACL GETUSER 的 RESP2 扁平数组与 RESP3 map 都能转成 map，
// 否则详情页在 RESP2 连接下会全是空值。
func TestToStringMap(t *testing.T) {
	resp2 := []interface{}{
		"flags", []interface{}{"on"},
		"commands", "+@all",
	}
	got := toStringMap(resp2)
	if got["commands"] != "+@all" {
		t.Fatalf("RESP2 扁平数组解析不正确: %+v", got)
	}

	resp3 := map[interface{}]interface{}{"commands": "+@all", "keys": "~*"}
	got = toStringMap(resp3)
	if got["keys"] != "~*" {
		t.Fatalf("RESP3 map 解析不正确: %+v", got)
	}

	// 非字符串键与奇数长度数组都应被安全跳过
	got = toStringMap(map[interface{}]interface{}{1: "x"})
	if len(got) != 0 {
		t.Fatalf("非字符串键应被忽略, got %+v", got)
	}
	got = toStringMap([]interface{}{"flags"})
	if len(got) != 0 {
		t.Fatalf("奇数长度数组应被忽略, got %+v", got)
	}
	if got = toStringMap(nil); len(got) != 0 {
		t.Fatalf("nil 应返回空 map, got %+v", got)
	}
}

// 验证数组值与空格分隔字符串都能规整成字符串数组。
func TestToSlice(t *testing.T) {
	cases := []struct {
		in   interface{}
		want []string
	}{
		{[]interface{}{"a", "b"}, []string{"a", "b"}},
		{"a b c", []string{"a", "b", "c"}},
		{"   ", nil},
		{[]interface{}{1, "a"}, []string{"a"}},
		{nil, nil},
	}
	for _, c := range cases {
		if got := toSlice(c.in); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("toSlice(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// 验证 ACL DRYRUN 等返回值的文本化：字符串直传，数组按空格拼接，其余为空。
func TestToText(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{"OK", "OK"},
		{[]interface{}{"NOPERM", "this", "user"}, "NOPERM this user"},
		// ACL LOG 的 count 是整型，必须转成文本而不是被丢弃
		{int64(3), "3"},
		{int(2), "2"},
		{1.5, "1.5"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := toText(c.in); got != c.want {
			t.Fatalf("toText(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 验证拼接工具不会产生多余分隔符。
func TestJoinNonEmpty(t *testing.T) {
	if got := joinNonEmpty("", "~*", " "); got != "~*" {
		t.Fatalf("当前值为空时不应加分隔符, got %q", got)
	}
	if got := joinNonEmpty("~a", "~b", " "); got != "~a ~b" {
		t.Fatalf("拼接结果不正确, got %q", got)
	}
}
