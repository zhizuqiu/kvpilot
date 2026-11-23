package services

import (
	"context"
	"net"
	"testing"
	"time"
	"tinyrdm/backend/types"
)

// startHangingServer 启动一个只接受连接、从不回包的监听器，
// 用来模拟"端口可连但节点无响应"的坏节点。
func startHangingServer(t *testing.T) (addr string, port int, stop func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	conns := make(chan net.Conn, 16)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns <- c // 故意不读不写，让客户端卡在等待响应
		}
	}()

	tcpAddr := ln.Addr().(*net.TCPAddr)
	stop = func() {
		_ = ln.Close()
		close(conns)
		for c := range conns {
			_ = c.Close()
		}
	}
	return tcpAddr.IP.String(), tcpAddr.Port, stop
}

// 验证集群拓扑发现必须有超时：seed 节点可连但不响应时，
// 打开连接不能无限等待，必须在限定时间内返回错误。
func TestCreateClusterClientFailsFastOnHangingSeed(t *testing.T) {
	addr, port, stop := startHangingServer(t)
	defer stop()

	// 缩短发现超时，避免测试耗时过长
	old := clusterDiscoveryTimeout
	clusterDiscoveryTimeout = 400 * time.Millisecond
	defer func() { clusterDiscoveryTimeout = old }()

	svc := &connectionService{ctx: context.Background()}
	cfg := types.ConnectionConfig{
		Name:        "hanging",
		Network:     "tcp",
		Addr:        addr,
		Port:        port,
		ConnTimeout: 1,
		Cluster:     types.ConnectionCluster{Enable: true},
	}

	start := time.Now()
	client, err := svc.createRedisClient(cfg)
	elapsed := time.Since(start)

	if client != nil {
		_ = client.Close()
	}
	if err == nil {
		t.Fatal("节点无响应时应返回错误")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("集群发现耗时 %v，未在超时内返回", elapsed)
	}
	if _, ok := err.(*net.OpError); ok {
		t.Logf("错误类型为网络错误（符合预期）: %v", err)
	}
}

// 验证哨兵地址查询同样有超时，避免哨兵不可达时连接卡死。
func TestCreateSentinelClientFailsFastOnHangingSentinel(t *testing.T) {
	addr, port, stop := startHangingServer(t)
	defer stop()

	old := clusterDiscoveryTimeout
	clusterDiscoveryTimeout = 400 * time.Millisecond
	defer func() { clusterDiscoveryTimeout = old }()

	svc := &connectionService{ctx: context.Background()}
	cfg := types.ConnectionConfig{
		Name:        "hanging-sentinel",
		Network:     "tcp",
		Addr:        addr,
		Port:        port,
		ConnTimeout: 1,
		Sentinel:    types.ConnectionSentinel{Enable: true, Master: "kvpmaster"},
	}

	start := time.Now()
	_, err := svc.createRedisClient(cfg)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("哨兵无响应时应返回错误")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("哨兵查询耗时 %v，未在超时内返回", elapsed)
	}
}
