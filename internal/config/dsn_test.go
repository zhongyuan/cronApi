package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestDSN_Basic(t *testing.T) {
	m := MySQLConfig{
		Host: "db.example.com", Port: 3306, User: "cronapi",
		Password: "P@ssw0rd!", Database: "cron_api", Charset: "utf8mb4",
		TimeZone: "Asia/Shanghai",
	}
	dsn := m.DSN()

	// 密码含 @ ! 时必须转义，否则驱动会把 @ 当成 user/host 分隔符
	if strings.Count(dsn, "@") != 1 {
		t.Fatalf("密码中的 @ 未转义，DSN: %s", dsn)
	}
	userinfo, rest, ok := strings.Cut(dsn, "@")
	if !ok {
		t.Fatalf("DSN 缺少 @: %s", dsn)
	}
	u, p, ok2 := strings.Cut(userinfo, ":")
	if !ok2 {
		t.Fatalf("DSN userinfo 非法: %s", userinfo)
	}
	if du, err := url.PathUnescape(u); err != nil || du != "cronapi" {
		t.Fatalf("user 解析失败: %q", u)
	}
	if dp, err := url.PathUnescape(p); err != nil || dp != "P@ssw0rd!" {
		t.Fatalf("password 解析失败: %q", p)
	}
	if !strings.HasPrefix(rest, "tcp(db.example.com:3306)/cron_api?") {
		t.Fatalf("host/db 部分错误: %s", rest)
	}

	q, err := url.ParseQuery(rest[strings.IndexByte(rest, '?')+1:])
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Get("loc"); got != "Asia/Shanghai" {
		t.Fatalf("loc = %q, want Asia/Shanghai", got)
	}
	if q.Get("parseTime") != "true" || q.Get("charset") != "utf8mb4" {
		t.Fatalf("缺少必要参数: %v", q)
	}
	if q.Get("timeout") == "" || q.Get("readTimeout") == "" {
		t.Fatalf("公网场景应设置连接/读写超时: %v", q)
	}
}

func TestDSN_NoPassword(t *testing.T) {
	m := MySQLConfig{Host: "127.0.0.1", Port: 3306, User: "root", Database: "cron_api"}
	dsn := m.DSN()
	if !strings.HasPrefix(dsn, "root:@tcp(127.0.0.1:3306)/cron_api?") {
		t.Fatalf("空密码 DSN 格式错误: %s", dsn)
	}
	if !strings.Contains(dsn, "loc=Local") {
		t.Fatalf("未配置时区时应回退为 Local: %s", dsn)
	}
}

func TestDSN_SpecialChars(t *testing.T) {
	m := MySQLConfig{
		Host: "10.0.0.8", Port: 3307, User: "app/user",
		Password: "p@ss:word/with%chars#1?", Database: "cron_api",
	}
	dsn := m.DSN()
	userinfo := dsn[:strings.Index(dsn, "@tcp(")]
	u, p, _ := strings.Cut(userinfo, ":")
	if du, _ := url.PathUnescape(u); du != "app/user" {
		t.Fatalf("用户名转义错误: %q", u)
	}
	if dp, _ := url.PathUnescape(p); dp != "p@ss:word/with%chars#1?" {
		t.Fatalf("密码转义错误: %q -> %q", p, dp)
	}
}
