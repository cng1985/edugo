package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Database     DatabaseConfig     `yaml:"database"`
	Auth         AuthConfig         `yaml:"auth"`
	AI           AIConfig           `yaml:"ai"`
	Log          LogConfig          `yaml:"log"`
	Scheduler    SchedulerConfig    `yaml:"scheduler"`
	EventBus     EventBusConfig     `yaml:"eventbus"`
	Organization OrganizationConfig `yaml:"organization"`
	Storage      StorageConfig      `yaml:"storage"`
	Query        QueryConfig        `yaml:"query"`
	Admin        AdminConfig        `yaml:"admin"`
	Seed         SeedConfig         `yaml:"seed"`
	Security     SecurityConfig     `yaml:"security"`
}

type ServerConfig struct {
	Port            int           `yaml:"port"`
	Mode            string        `yaml:"mode"`
	ShutdownTimeout time.Duration `yaml:"shutdownTimeout"`
}

type DatabaseConfig struct {
	Driver          string        `yaml:"driver"`
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"maxOpenConns"`
	MaxIdleConns    int           `yaml:"maxIdleConns"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime"`
	MigrationMode   string        `yaml:"migrationMode"`
}

type AuthConfig struct {
	JWTSecret     string        `yaml:"jwtSecret"`
	JWTExpire     time.Duration `yaml:"jwtExpire"`
	RefreshExpire time.Duration `yaml:"refreshExpire"`
}

type AIProfile struct {
	Model           string `yaml:"model"`
	MaxOutputTokens int    `yaml:"maxOutputTokens"`
}

type AIConfig struct {
	Enabled           bool                `yaml:"enabled"`
	Provider          string              `yaml:"provider"`
	BaseURL           string              `yaml:"baseURL"`
	APIKey            string              `yaml:"apiKey"`
	ConnectTimeout    time.Duration       `yaml:"connectTimeout"`
	RequestTimeout    time.Duration       `yaml:"requestTimeout"`
	StreamIdleTimeout time.Duration       `yaml:"streamIdleTimeout"`
	StreamMaxDuration time.Duration       `yaml:"streamMaxDuration"`
	MaxRetries        int                 `yaml:"maxRetries"`
	Profiles          map[string]AIProfile `yaml:"profiles"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

type SchedulerConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Timezone      string        `yaml:"timezone"`
	CheckInterval time.Duration `yaml:"checkInterval"`
}

type EventBusConfig struct {
	PollInterval time.Duration `yaml:"pollInterval"`
	MaxAttempts  int           `yaml:"maxAttempts"`
}

type OrganizationConfig struct {
	Enabled bool `yaml:"enabled"`
}

type StorageConfig struct {
	Driver    string `yaml:"driver"`
	Root      string `yaml:"root"`
	MaxBytes  int64  `yaml:"maxBytes"`
}

type QueryConfig struct {
	DefaultPageSize int `yaml:"defaultPageSize"`
	MaxPageSize     int `yaml:"maxPageSize"`
	MaxListSize     int `yaml:"maxListSize"`
}

type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type SeedConfig struct {
	Demo bool `yaml:"demo"`
}

type SecurityConfig struct {
	BcryptCost         int `yaml:"bcryptCost"`
	LoginPerMinute     int `yaml:"loginPerMinute"`
	RefreshPerMinute   int `yaml:"refreshPerMinute"`
	AIPerMinute        int `yaml:"aiPerMinute"`
	MaxJSONBodyBytes   int `yaml:"maxJSONBodyBytes"`
}

func Defaults() *Config {
	return &Config{
		Server: ServerConfig{Port: 8080, Mode: "debug", ShutdownTimeout: 30 * time.Second},
		Database: DatabaseConfig{
			Driver: "sqlite", DSN: "data/edugo.db", MaxOpenConns: 1, MaxIdleConns: 1,
			ConnMaxLifetime: time.Hour, MigrationMode: "versioned",
		},
		Auth: AuthConfig{JWTExpire: 24 * time.Hour, RefreshExpire: 168 * time.Hour},
		AI: AIConfig{
			Enabled: false, Provider: "openai-compatible",
			ConnectTimeout: 10 * time.Second, RequestTimeout: 60 * time.Second,
			StreamIdleTimeout: 30 * time.Second, StreamMaxDuration: 5 * time.Minute, MaxRetries: 2,
			Profiles: map[string]AIProfile{
				"tutoring":           {MaxOutputTokens: 2048},
				"questionGeneration": {MaxOutputTokens: 4096},
				"grading":            {MaxOutputTokens: 2048},
				"suggestion":         {MaxOutputTokens: 2048},
				"pathGeneration":     {MaxOutputTokens: 4096},
				"summary":            {MaxOutputTokens: 2048},
				"embedding":          {MaxOutputTokens: 0},
			},
		},
		Log:       LogConfig{Level: "info", Format: "json", Output: "stdout"},
		Scheduler: SchedulerConfig{Enabled: true, Timezone: "Asia/Shanghai", CheckInterval: time.Minute},
		EventBus:  EventBusConfig{PollInterval: time.Second, MaxAttempts: 8},
		Storage:   StorageConfig{Driver: "local", Root: "data/uploads", MaxBytes: 20 << 20},
		Query:     QueryConfig{DefaultPageSize: 20, MaxPageSize: 200, MaxListSize: 1000},
		Security:  SecurityConfig{BcryptCost: 12, LoginPerMinute: 20, RefreshPerMinute: 30, AIPerMinute: 20, MaxJSONBodyBytes: 1 << 20},
	}
}

func Load() (*Config, error) {
	cfg := Defaults()
	path := os.Getenv("EDUGO_CONFIG_PATH")
	if path == "" {
		path = "configs/config.yaml"
	}
	if b, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("解析配置失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取配置失败")
	} else if strings.EqualFold(cfg.Server.Mode, "release") && os.Getenv("EDUGO_CONFIG_PATH") != "" {
		return nil, fmt.Errorf("发布模式缺少配置文件")
	}
	applyEnv(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("EDUGO_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		} else {
			cfg.Server.Port = -1
		}
	}
	if v := os.Getenv("EDUGO_SERVER_MODE"); v != "" {
		cfg.Server.Mode = v
	}
	if v := os.Getenv("EDUGO_DATABASE_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("EDUGO_DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("EDUGO_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("EDUGO_AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("EDUGO_AI_BASE_URL"); v != "" {
		cfg.AI.BaseURL = v
	}
	if v := os.Getenv("EDUGO_AI_ENABLED"); v != "" {
		cfg.AI.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("EDUGO_ADMIN_USERNAME"); v != "" {
		cfg.Admin.Username = v
	}
	if v := os.Getenv("EDUGO_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("EDUGO_SEED_DEMO"); v != "" {
		cfg.Seed.Demo = v == "true" || v == "1"
	}
	if v := os.Getenv("EDUGO_ORGANIZATION_ENABLED"); v != "" {
		cfg.Organization.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("EDUGO_SECURITY_BCRYPT_COST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Security.BcryptCost = n
		}
	}
	chatDefault := os.Getenv("EDUGO_AI_MODEL")
	if chatDefault != "" {
		for name, profile := range cfg.AI.Profiles {
			if name == "embedding" {
				continue
			}
			if profile.Model == "" {
				profile.Model = chatDefault
				cfg.AI.Profiles[name] = profile
			}
		}
	}
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 非法")
	}
	switch strings.ToLower(c.Server.Mode) {
	case "debug", "release", "test":
	default:
		return fmt.Errorf("server.mode 非法")
	}
	switch c.Database.Driver {
	case "sqlite", "mysql", "postgres":
	default:
		return fmt.Errorf("database.driver 仅允许 sqlite、mysql、postgres")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return fmt.Errorf("database.dsn 不能为空")
	}
	if c.Database.MaxOpenConns < 1 || c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("数据库连接池参数非法")
	}
	if c.Database.Driver == "sqlite" && c.Database.MaxOpenConns != 1 {
		return fmt.Errorf("sqlite 轻量部署要求 maxOpenConns=1")
	}
	if _, err := time.LoadLocation(c.Scheduler.Timezone); err != nil || c.Scheduler.Timezone == "" {
		return fmt.Errorf("scheduler.timezone 非法")
	}
	if c.Scheduler.CheckInterval < 100*time.Millisecond {
		return fmt.Errorf("scheduler.checkInterval 过小")
	}
	if c.EventBus.PollInterval <= 0 {
		c.EventBus.PollInterval = time.Second
	}
	if c.EventBus.MaxAttempts <= 0 {
		c.EventBus.MaxAttempts = 8
	}
	if c.Query.DefaultPageSize < 1 || c.Query.MaxPageSize < c.Query.DefaultPageSize || c.Query.MaxListSize < 1 {
		return fmt.Errorf("query 分页参数非法")
	}
	if c.Security.BcryptCost < 4 || c.Security.BcryptCost > 16 {
		return fmt.Errorf("security.bcryptCost 超出范围")
	}
	if c.Auth.JWTExpire <= 0 || c.Auth.RefreshExpire <= 0 {
		return fmt.Errorf("令牌有效期非法")
	}
	release := strings.EqualFold(c.Server.Mode, "release")
	if release {
		if len(c.Auth.JWTSecret) < 32 || isPlaceholderSecret(c.Auth.JWTSecret) {
			return fmt.Errorf("发布环境必须注入足够长度的 JWT 密钥")
		}
		if c.Seed.Demo {
			return fmt.Errorf("发布环境禁止演示种子数据")
		}
		if c.Admin.Password != "" && isWeakPassword(c.Admin.Password) {
			return fmt.Errorf("发布环境禁止示例管理员密码")
		}
	}
	if c.AI.Enabled {
		if strings.TrimSpace(c.AI.APIKey) == "" || strings.TrimSpace(c.AI.BaseURL) == "" {
			return fmt.Errorf("已启用 AI，但缺少密钥或 baseURL")
		}
		if c.AI.Provider != "openai-compatible" {
			return fmt.Errorf("当前仅内置 openai-compatible 适配器")
		}
		hasChat := false
		for name, profile := range c.AI.Profiles {
			if name == "embedding" {
				continue
			}
			if strings.TrimSpace(profile.Model) != "" {
				hasChat = true
			}
		}
		if !hasChat {
			return fmt.Errorf("已启用 AI，但没有可用的聊天 profile")
		}
	}
	return nil
}

func isPlaceholderSecret(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "change-me", "secret", "jwt-secret", "example":
		return true
	default:
		return false
	}
}

func isWeakPassword(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "admin", "admin123", "password", "123456", "admin@123456":
		return true
	default:
		return false
	}
}

func (c *Config) Profile(name string) (AIProfile, bool) {
	p, ok := c.AI.Profiles[name]
	if !ok || strings.TrimSpace(p.Model) == "" {
		return AIProfile{}, false
	}
	return p, true
}
