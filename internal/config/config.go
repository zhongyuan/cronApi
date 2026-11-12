// Package config 负责加载、校验并暴露应用配置。
// 配置来源优先级：默认值 < 配置文件(YAML/JSON/TOML) < 环境变量。
// 环境变量使用 CRONAPI_ 前缀并以 _ 分隔嵌套 key，例如 mysql.host 对应 CRONAPI_MYSQL_HOST。
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 是全局配置根。
type Config struct {
	App        AppConfig        `mapstructure:"app"`
	Log        LogConfig        `mapstructure:"log"`
	MySQL      MySQLConfig      `mapstructure:"mysql"`
	Redis      RedisConfig      `mapstructure:"redis"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	WeChat     WeChatConfig     `mapstructure:"wechat"`
	Scheduler  SchedulerConfig  `mapstructure:"scheduler"`
	HTTPClient HTTPClientConfig `mapstructure:"http_client"`
}

type AppConfig struct {
	Name            string        `mapstructure:"name"`
	Env             string        `mapstructure:"env"`
	Addr            string        `mapstructure:"addr"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

func (a AppConfig) IsProd() bool { return a.Env == "prod" }

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type MySQLConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	Charset  string `mapstructure:"charset"`
	// TimeZone 驱动解析 DATETIME 时使用的时区，需与 MySQL 服务端 time_zone 一致。
	// 留空则使用进程本地时区（注意：容器内默认是 UTC，与北京时间相差 8 小时）。
	TimeZone        string        `mapstructure:"timezone"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	SlowThreshold   time.Duration `mapstructure:"slow_threshold"`
}

// DSN 返回 GORM/MySQL 驱动使用的连接串。
//
// 要点：
//   - 用户名/密码做 URL 转义，避免密码含 @ : / % 等字符导致连接串解析失败；
//   - loc 显式指定，保证 DATETIME 按数据库服务端时区解释（而非进程所在机器时区）；
//   - 开启 parseTime，gorm 才能正确映射 time.Time。
func (m MySQLConfig) DSN() string {
	charset := m.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	tz := m.TimeZone
	if tz == "" {
		tz = "Local"
	}
	q := url.Values{}
	q.Set("charset", charset)
	q.Set("parseTime", "true")
	q.Set("loc", tz)
	q.Set("timeout", "10s") // 建连超时：公网抖动时快速失败，而不是长时间挂起
	q.Set("readTimeout", "30s")
	q.Set("writeTimeout", "30s")

	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s",
		url.PathEscape(m.User), // 用户名同样转义，避免含 @ / : 时解析错乱
		escapePassword(m.Password),
		net.JoinHostPort(m.Host, strconv.Itoa(m.Port)),
		m.Database,
		q.Encode(),
	)
}

// escapePassword 对密码做 URL 转义；空密码返回空串。
func escapePassword(pwd string) string {
	if pwd == "" {
		return ""
	}
	// url.UserPassword 会输出 "user:pass"，这里只取冒号后的密码部分
	return strings.TrimPrefix(url.UserPassword("", pwd).String(), ":")
}

type RedisConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Addr         string        `mapstructure:"addr"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	Issuer     string        `mapstructure:"issuer"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type WeChatConfig struct {
	AppID     string        `mapstructure:"app_id"`
	AppSecret string        `mapstructure:"app_secret"`
	Mock      bool          `mapstructure:"mock"`
	Endpoint  string        `mapstructure:"endpoint"`
	Timeout   time.Duration `mapstructure:"timeout"`
}

type SchedulerConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	Timezone       string `mapstructure:"timezone"`
	MaxConcurrency int    `mapstructure:"max_concurrency"`
}

type HTTPClientConfig struct {
	Timeout            time.Duration `mapstructure:"timeout"`
	MaxIdleConns       int           `mapstructure:"max_idle_conns"`
	InsecureSkipVerify bool          `mapstructure:"insecure_skip_verify"`
}

// Load 从指定路径读取配置；path 为空时只使用默认值 + 环境变量。
func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
	}

	v.SetEnvPrefix("CRONAPI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// MustLoad 在加载失败时直接 panic，供 main 使用。
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Validate 做启动阶段的强校验，尽早暴露配置错误。
func (c *Config) Validate() error {
	switch c.App.Env {
	case "dev", "test", "prod":
	default:
		return fmt.Errorf("app.env 非法: %q (可选 dev/test/prod)", c.App.Env)
	}
	if c.App.Addr == "" {
		return fmt.Errorf("app.addr 不能为空")
	}
	if c.App.IsProd() && strings.Contains(c.JWT.Secret, "change-me") {
		return fmt.Errorf("生产环境必须通过 JWT_SECRET 设置强密钥")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 不能为空")
	}
	if c.JWT.AccessTTL <= 0 {
		c.JWT.AccessTTL = 2 * time.Hour
	}
	if c.JWT.RefreshTTL <= 0 {
		c.JWT.RefreshTTL = 168 * time.Hour
	}
	if !c.WeChat.Mock && (c.WeChat.AppID == "" || c.WeChat.AppSecret == "") {
		return fmt.Errorf("wechat.mock=false 时必须配置 app_id / app_secret")
	}
	if c.Scheduler.Timezone != "" {
		if _, err := time.LoadLocation(c.Scheduler.Timezone); err != nil {
			return fmt.Errorf("scheduler.timezone 非法: %w", err)
		}
	}
	if c.Scheduler.MaxConcurrency <= 0 {
		c.Scheduler.MaxConcurrency = 8
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "cronApi")
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.addr", ":8080")
	v.SetDefault("app.read_timeout", 15*time.Second)
	v.SetDefault("app.write_timeout", 30*time.Second)
	v.SetDefault("app.shutdown_timeout", 15*time.Second)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.output", "stdout")

	v.SetDefault("mysql.enabled", false)
	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.charset", "utf8mb4")
	v.SetDefault("mysql.max_idle_conns", 10)
	v.SetDefault("mysql.max_open_conns", 100)
	v.SetDefault("mysql.conn_max_lifetime", time.Hour)
	v.SetDefault("mysql.slow_threshold", 200*time.Millisecond)

	v.SetDefault("redis.enabled", true)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.pool_size", 50)
	v.SetDefault("redis.dial_timeout", 3*time.Second)
	v.SetDefault("redis.read_timeout", 2*time.Second)
	v.SetDefault("redis.write_timeout", 2*time.Second)

	v.SetDefault("jwt.secret", "dev-secret-please-override")
	v.SetDefault("jwt.issuer", "cronApi")
	v.SetDefault("jwt.access_ttl", 2*time.Hour)
	v.SetDefault("jwt.refresh_ttl", 168*time.Hour)

	v.SetDefault("wechat.mock", true)
	v.SetDefault("wechat.endpoint", "https://api.weixin.qq.com")
	v.SetDefault("wechat.timeout", 5*time.Second)

	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.timezone", "Asia/Shanghai")
	v.SetDefault("scheduler.max_concurrency", 8)

	v.SetDefault("http_client.timeout", 15*time.Second)
	v.SetDefault("http_client.max_idle_conns", 200)
	v.SetDefault("http_client.insecure_skip_verify", false)
}
