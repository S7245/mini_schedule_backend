package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用总配置
type Config struct {
	App        AppConfig        `mapstructure:"app"`
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	CORS       CORSConfig       `mapstructure:"cors"`
	Log        LogConfig        `mapstructure:"log"`
	Pagination PaginationConfig `mapstructure:"pagination"`
}

type AppConfig struct {
	Name  string `mapstructure:"name"`
	Env   string `mapstructure:"env"`
	Debug bool   `mapstructure:"debug"`
}

type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	DBName          string        `mapstructure:"dbname"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	LogLevel        string        `mapstructure:"log_level"`
}

// DSN 返回 GORM 所需的数据源名称
// 优先使用 DATABASE_URL 环境变量（Railway 等云平台原生提供）
func (d *DatabaseConfig) DSN() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		// Railway 提供的 URL 已含 sslmode=require，追加时区
		if strings.Contains(url, "?") {
			return url + "&TimeZone=Asia/Shanghai"
		}
		return url + "?TimeZone=Asia/Shanghai"
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s&TimeZone=Asia/Shanghai",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

type RedisConfig struct {
	Addr         string `mapstructure:"addr"`
	Password     string `mapstructure:"password"`
	DB           int    `mapstructure:"db"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

type JWTConfig struct {
	Secret        string        `mapstructure:"secret"`
	Expire        time.Duration `mapstructure:"expire"`
	RefreshExpire time.Duration `mapstructure:"refresh_expire"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type PaginationConfig struct {
	DefaultPageSize int `mapstructure:"default_page_size"`
	MaxPageSize     int `mapstructure:"max_page_size"`
}

// Load 加载配置，支持配置文件 + 环境变量覆盖
// 环境变量优先级高于配置文件，命名规则：MINI_SCHEDULE_DATABASE_HOST → database.host
func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// 允许环境变量覆盖（前缀 MINI_SCHEDULE_）
	// replacer 将嵌套 key 的 "." 映射为 "_"，使 MINI_SCHEDULE_DATABASE_HOST 能覆盖 database.host
	v.SetEnvPrefix("MINI_SCHEDULE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 显式绑定关键嵌套 key，确保 AutomaticEnv 能正确识别
	keysToBind := []string{
		"server.port",
		"database.host", "database.port", "database.user",
		"database.password", "database.dbname", "database.ssl_mode",
		"redis.addr", "redis.password", "redis.db",
		"jwt.secret", "jwt.expire", "jwt.refresh_expire",
		"app.env", "app.debug",
		"log.level", "log.format",
	}
	for _, key := range keysToBind {
		_ = v.BindEnv(key)
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// PORT 是 Railway 注入的标准端口变量，直接覆盖 server.port
	if port := os.Getenv("PORT"); port != "" {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err == nil {
			cfg.Server.Port = p
		}
	}

	return &cfg, nil
}
