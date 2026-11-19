#!/usr/bin/env python3
"""为 README 截图准备一份可读的演示数据（不包含任何真实或地理信息）。

用法：
    redis-server --port 6379 --requirepass demo123 --daemonize yes
    python3 scripts/seed-demo-data.py

可通过环境变量覆盖连接参数：
    REDIS_HOST / REDIS_PORT / REDIS_PASSWORD
"""
import json
import os
import random
import socket

HOST = os.getenv("REDIS_HOST", "127.0.0.1")
PORT = int(os.getenv("REDIS_PORT", "6379"))
PASSWORD = os.getenv("REDIS_PASSWORD", "demo123")


class Redis:
    def __init__(self):
        self.sock = socket.create_connection((HOST, PORT))
        self.f = self.sock.makefile("rb")
        self.cmd("AUTH", PASSWORD)

    def cmd(self, *args):
        out = b"*%d\r\n" % len(args)
        for a in args:
            b = str(a).encode()
            out += b"$%d\r\n%s\r\n" % (len(b), b)
        self.sock.sendall(out)
        return self._read()

    def _read(self):
        line = self.f.readline()
        t, body = line[:1], line[1:-2]
        if t == b"+":
            return body.decode()
        if t == b"-":
            raise RuntimeError(body.decode())
        if t == b":":
            return int(body)
        if t == b"$":
            n = int(body)
            if n == -1:
                return None
            data = self.f.read(n + 2)[:-2]
            return data.decode(errors="replace")
        if t == b"*":
            n = int(body)
            return [self._read() for _ in range(n)] if n != -1 else None
        raise RuntimeError(f"unexpected reply: {line!r}")


r = Redis()
r.cmd("FLUSHALL")

random.seed(42)

# 用户资料：hash + 关联 string/set
for i in range(1, 21):
    uid = 1000 + i
    r.cmd("HSET", f"user:{uid}:profile", "name", f"user{i:02d}",
          "email", f"user{i:02d}@example.com",
          "level", random.randint(1, 9), "vip", random.choice(["true", "false"]))
    r.cmd("SET", f"user:{uid}:avatar", f"https://cdn.example.com/avatar/{uid}.png")
    r.cmd("SADD", f"user:{uid}:roles", *random.sample(["admin", "dev", "ops", "guest", "qa"], 2))

# 商品
for i in range(1, 26):
    sku = 3000 + i
    r.cmd("HSET", f"product:sku:{sku}", "title", f"商品 {i:02d}", "price", f"{random.randint(9, 999)}.00",
          "stock", random.randint(0, 500), "status", random.choice(["online", "offline"]))
    r.cmd("SET", f"product:sku:{sku}:cover", f"https://cdn.example.com/p/{sku}.jpg")

# 会话（带 TTL）
for i in range(1, 13):
    r.cmd("SET", f"session:token:{i:04d}", json.dumps({"uid": 1000 + i, "ua": "Mozilla/5.0"}),
          "EX", random.randint(600, 86400))

# 页面缓存
for i in range(1, 31):
    r.cmd("SET", f"cache:page:/feed/{i}", "x" * random.randint(200, 800), "EX", 3600)

# 订单队列
r.cmd("DEL", "queue:orders")
for i in range(12):
    r.cmd("RPUSH", "queue:orders", json.dumps({"order": 90000 + i, "amount": random.randint(10, 900)}))

# 标签集合
r.cmd("DEL", "article:42:tags")
r.cmd("SADD", "article:42:tags", "redis", "kv", "缓存", "性能", "运维")

# 排行榜
r.cmd("DEL", "leaderboard:weekly")
for i, name in enumerate(["alice", "bob", "carol", "dave", "erin", "frank", "grace", "heidi"], start=1):
    r.cmd("ZADD", "leaderboard:weekly", random.randint(100, 9999), name)

# 事件流
r.cmd("DEL", "events:stream")
for i in range(8):
    r.cmd("XADD", "events:stream", "*", "type", random.choice(["login", "logout", "purchase"]),
          "uid", 1000 + i, "ok", "true")

# 计数器与限流
r.cmd("SET", "counter:page:view", 1284937)
r.cmd("SET", "ratelimit:api:10.0.0.5", 42, "EX", 60)

# 大文本（方便展示编辑器）
r.cmd("SET", "config:app:settings", json.dumps({
    "app": "kvpilot-demo", "version": "1.2.8", "features": ["arena", "cluster", "sentinel"],
    "limits": {"max_conn": 200, "timeout_ms": 3000},
}, ensure_ascii=False, indent=2))

# 数据库信息
print("db0 keys:", r.cmd("DBSIZE"))
for key in ["user:1001:profile", "leaderboard:weekly", "events:stream"]:
    print(key, "->", r.cmd("TYPE", key))
