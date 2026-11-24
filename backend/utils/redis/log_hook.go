package redis

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type execCallback func(string, int64)

type LogHook struct {
	name    string
	cmdExec execCallback
}

func NewHook(name string, cmdExec execCallback) *LogHook {
	return &LogHook{
		name:    name,
		cmdExec: cmdExec,
	}
}

// sensitiveConfigKeys 是日志与命令历史中需要隐藏取值的配置项。
var sensitiveConfigKeys = map[string]bool{
	"requirepass":               true,
	"masterauth":                true,
	"tls-key-file-pass":         true,
	"tls-client-key-pass":       true,
	"tls-client-cert-file-pass": true,
}

func argString(v interface{}) string {
	switch v := v.(type) {
	case nil:
		return "<nil>"
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

// redactArgs 对含密码的命令参数做脱敏，避免命令历史与服务端日志出现明文口令。
// 只处理已知敏感形态，其余命令保持原样以保留排查价值。
func redactArgs(args []interface{}) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = argString(a)
	}

	cmd := strings.ToLower(out[0])
	switch cmd {
	case "auth":
		for i := 1; i < len(out); i++ {
			out[i] = "***"
		}
	case "hello":
		// HELLO [protover [AUTH username password] [SETNAME name]]
		for i := 2; i+2 < len(out); i++ {
			if strings.EqualFold(out[i], "auth") {
				out[i+2] = "***"
				break
			}
		}
	case "config":
		// CONFIG SET key value [key value ...]
		if len(out) >= 4 && strings.EqualFold(out[1], "set") {
			for i := 2; i+1 < len(out); i += 2 {
				if sensitiveConfigKeys[strings.ToLower(out[i])] {
					out[i+1] = "***"
				}
			}
		}
	case "acl":
		// ACL SETUSER user reset on >password <password ...
		if len(out) >= 3 && strings.EqualFold(out[1], "setuser") {
			for i := 2; i < len(out); i++ {
				if strings.HasPrefix(out[i], ">") || strings.HasPrefix(out[i], "<") {
					out[i] = "***"
				}
			}
		}
	case "sentinel":
		// SENTINEL SET master auth-pass password
		if len(out) >= 4 && strings.EqualFold(out[1], "set") {
			for i := 2; i+1 < len(out); i++ {
				if strings.EqualFold(out[i], "auth-pass") {
					out[i+1] = "***"
				}
			}
		}
	case "migrate":
		// MIGRATE host port key db timeout [AUTH password | AUTH2 user password]
		for i := 1; i+1 < len(out); i++ {
			switch {
			case strings.EqualFold(out[i], "auth"):
				out[i+1] = "***"
			case strings.EqualFold(out[i], "auth2"):
				out[i+1] = "***"
				if i+2 < len(out) {
					out[i+2] = "***"
				}
			}
		}
	}
	return out
}

// commandLine 生成用于日志与命令历史的完整命令行（已脱敏）。
func commandLine(args []interface{}) string {
	return strings.Join(redactArgs(args), " ")
}

func appendArg(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case nil:
		return append(b, "<nil>"...)
	case string:
		return append(b, []byte(v)...)
	case []byte:
		return append(b, v...)
	case int:
		return strconv.AppendInt(b, int64(v), 10)
	case int8:
		return strconv.AppendInt(b, int64(v), 10)
	case int16:
		return strconv.AppendInt(b, int64(v), 10)
	case int32:
		return strconv.AppendInt(b, int64(v), 10)
	case int64:
		return strconv.AppendInt(b, v, 10)
	case uint:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint8:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint16:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint32:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint64:
		return strconv.AppendUint(b, v, 10)
	case float32:
		return strconv.AppendFloat(b, float64(v), 'f', -1, 64)
	case float64:
		return strconv.AppendFloat(b, v, 'f', -1, 64)
	case bool:
		if v {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case time.Time:
		return v.AppendFormat(b, time.RFC3339Nano)
	default:
		return append(b, fmt.Sprint(v)...)
	}
}

func (l *LogHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (l *LogHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		t := time.Now()
		err := next(ctx, cmd)
		line := commandLine(cmd.Args())
		log.Println(line)
		if l.cmdExec != nil {
			l.cmdExec(line, time.Since(t).Milliseconds())
		}
		return err
	}
}

func (l *LogHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		t := time.Now()
		err := next(ctx, cmds)
		cost := time.Since(t).Milliseconds()
		lines := make([]string, 0, len(cmds))
		for _, cmd := range cmds {
			line := commandLine(cmd.Args())
			log.Println("pipeline:", line)
			lines = append(lines, line)
		}
		if l.cmdExec != nil {
			l.cmdExec(strings.Join(lines, "\n"), cost)
		}
		return err
	}
}
