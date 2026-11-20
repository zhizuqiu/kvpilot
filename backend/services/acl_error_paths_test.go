package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newDeadService 构造一个连接不可达的 browserService，
// 用于覆盖各接口的错误分支，不需要真实 Redis，也不访问网络。
func newDeadService(t *testing.T) (*browserService, string) {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		MaxRetries:  -1, // 关闭重试，避免错误路径测试变慢
		PoolSize:    1,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	svc := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	name := "kvpilot_dead"
	svc.connMap[name] = &connectionItem{client: client, ctx: ctx, db: 0}
	return svc, name
}

// 验证连接不可达时，所有 ACL 接口都返回失败且带错误信息，
// 而不是 panic 或假装成功。
func TestAclEndpointsWithUnreachableServer(t *testing.T) {
	svc, name := newDeadService(t)

	cases := []struct {
		api string
		run func() (bool, string)
	}{
		{"overview", func() (bool, string) {
			r := svc.AclOverview(name)
			return r.Success, r.Msg
		}},
		{"user", func() (bool, string) {
			r := svc.AclUser(name, "default")
			return r.Success, r.Msg
		}},
		{"categories", func() (bool, string) {
			r := svc.AclCategories(name)
			return r.Success, r.Msg
		}},
		{"setuser", func() (bool, string) {
			r := svc.AclSetUser(name, "app", []string{"reset", "on"})
			return r.Success, r.Msg
		}},
		{"deleteuser", func() (bool, string) {
			r := svc.AclDeleteUser(name, "app")
			return r.Success, r.Msg
		}},
		{"log", func() (bool, string) {
			r := svc.AclLog(name, 10)
			return r.Success, r.Msg
		}},
		{"dryrun", func() (bool, string) {
			r := svc.AclDryRun(name, "app", []string{"get", "k"})
			return r.Success, r.Msg
		}},
	}

	for _, c := range cases {
		t.Run(c.api, func(t *testing.T) {
			success, msg := c.run()
			if success {
				t.Fatalf("%s 在连接不可达时不应返回成功", c.api)
			}
			if strings.TrimSpace(msg) == "" {
				t.Fatalf("%s 失败时必须给出错误信息", c.api)
			}
		})
	}
}

// 验证连接名不存在时，接口返回带连接名的错误，
// 这是前端"连接已被删除但标签页还在"时的兜底路径。
func TestAclEndpointsWithUnknownConnection(t *testing.T) {
	// Connection() 单例在首次使用时按 HOME 解析存储路径，
	// 这里指向临时目录，避免读写用户真实的 connections.yaml。
	t.Setenv("HOME", t.TempDir())

	svc := &browserService{ctx: context.Background(), connMap: map[string]*connectionItem{}}

	for _, c := range []struct {
		api string
		run func() (bool, string)
	}{
		{"overview", func() (bool, string) {
			r := svc.AclOverview("kvpilot_missing")
			return r.Success, r.Msg
		}},
		{"user", func() (bool, string) {
			r := svc.AclUser("kvpilot_missing", "default")
			return r.Success, r.Msg
		}},
		{"categories", func() (bool, string) {
			r := svc.AclCategories("kvpilot_missing")
			return r.Success, r.Msg
		}},
		{"setuser", func() (bool, string) {
			r := svc.AclSetUser("kvpilot_missing", "app", []string{"on"})
			return r.Success, r.Msg
		}},
		{"deleteuser", func() (bool, string) {
			r := svc.AclDeleteUser("kvpilot_missing", "app")
			return r.Success, r.Msg
		}},
		{"log", func() (bool, string) {
			r := svc.AclLog("kvpilot_missing", 10)
			return r.Success, r.Msg
		}},
		{"dryrun", func() (bool, string) {
			r := svc.AclDryRun("kvpilot_missing", "app", []string{"get", "k"})
			return r.Success, r.Msg
		}},
	} {
		t.Run(c.api, func(t *testing.T) {
			success, msg := c.run()
			if success {
				t.Fatalf("%s 在连接不存在时不应返回成功", c.api)
			}
			if !strings.Contains(msg, "no match connection") {
				t.Fatalf("%s 错误信息应包含 no match connection, got %q", c.api, msg)
			}
		})
	}
}

// 验证 aclFirst 在所有分片都失败时返回首个错误，避免调用方拿到 nil 错误却无数据。
func TestAclFirstReturnsErrorWhenAllShardsFail(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	t.Cleanup(func() { _ = client.Close() })

	svc := &browserService{ctx: context.Background(), connMap: map[string]*connectionItem{}}
	if _, err := svc.aclFirst(context.Background(), client, "ACL", "WHOAMI"); err == nil {
		t.Fatal("全部分片失败时应返回错误")
	}
}

// 验证集群客户端完全没有可用分片时，AclOverview 返回明确的失败信息，
// 而不是成功返回空列表让前端显示"没有用户"。
func TestAclOverviewWithoutShards(t *testing.T) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        []string{"127.0.0.1:1"},
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		MaxRetries:   -1,
		MaxRedirects: 1,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	svc := &browserService{ctx: ctx, connMap: map[string]*connectionItem{}}
	name := "kvpilot_dead_cluster"
	svc.connMap[name] = &connectionItem{client: client, ctx: ctx, db: 0}

	resp := svc.AclOverview(name)
	if resp.Success {
		t.Fatalf("无可用分片时不应返回成功, data=%+v", resp.Data)
	}
	if resp.Msg == "" {
		t.Fatal("失败时必须给出错误信息")
	}
}
