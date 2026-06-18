package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Version 通过 -ldflags 注入
var Version = "0.1.0-dev"

type Config struct {
	Storage   StorageConfig   `yaml:"storage"   json:"storage"`
	Collector CollectorConfig `yaml:"collector" json:"collector"`
	Dashboard DashboardConfig `yaml:"dashboard" json:"dashboard"`
	Stats     StatsConfig     `yaml:"stats"     json:"stats"`
	Alerts    AlertsConfig    `yaml:"alerts"    json:"alerts"`
	Export    ExportConfig    `yaml:"export"    json:"export"`
	Logging   LoggingConfig   `yaml:"logging"   json:"logging"`
}

type StorageConfig struct {
	Type          string `yaml:"type"           json:"type"`
	Path          string `yaml:"path"           json:"path"`
	RetentionDays int    `yaml:"retention_days"  json:"retention_days"`
	MaxInputSize  int    `yaml:"max_input_size"  json:"max_input_size"`
	MaxOutputSize int    `yaml:"max_output_size" json:"max_output_size"`
}

type CollectorConfig struct {
	TimeoutMs  int  `yaml:"timeout_ms"  json:"timeout_ms"`
	BatchSize  int  `yaml:"batch_size"  json:"batch_size"`
	AsyncWrite bool `yaml:"async_write" json:"async_write"`
}

type DashboardConfig struct {
	Host              string `yaml:"host"                json:"host"`
	Port              int    `yaml:"port"                json:"port"`
	RefreshIntervalMs int    `yaml:"refresh_interval_ms" json:"refresh_interval_ms"`
}

type StatsConfig struct {
	AggregationInterval string `yaml:"aggregation_interval" json:"aggregation_interval"`
}

type AlertsConfig struct {
	Enabled  bool           `yaml:"enabled"  json:"enabled"`
	Rules    []RuleConfig   `yaml:"rules"    json:"rules"`
	Channels []ChannelConfig `yaml:"channels" json:"channels"`
}

type RuleConfig struct {
	ID        string `yaml:"id"         json:"id"`
	Threshold int    `yaml:"threshold"  json:"threshold"`
	Window    string `yaml:"window"     json:"window,omitempty"`
	Enabled   bool   `yaml:"enabled"    json:"enabled"`
}

type ChannelConfig struct {
	Type     string `yaml:"type"      json:"type"`
	MinLevel string `yaml:"min_level" json:"min_level"`
	URL      string `yaml:"url"       json:"url,omitempty"`
	Path     string `yaml:"path"      json:"path,omitempty"`
}

type ExportConfig struct {
	DefaultFormat string `yaml:"default_format" json:"default_format"`
}

type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`
	Path  string `yaml:"path"  json:"path"`
}

// DefaultConfig 返回带默认值的配置
func DefaultConfig() *Config {
	return &Config{
		Storage: StorageConfig{
			Type:          "sqlite",
			Path:          "",
			RetentionDays: 30,
			MaxInputSize:  10240,
			MaxOutputSize: 10240,
		},
		Collector: CollectorConfig{
			TimeoutMs:  5000,
			BatchSize:  1,
			AsyncWrite: true,
		},
		Dashboard: DashboardConfig{
			Host:              "127.0.0.1",
			Port:              8080,
			RefreshIntervalMs: 1000,
		},
		Stats: StatsConfig{
			AggregationInterval: "5m",
		},
		Alerts: AlertsConfig{
			Enabled:  false,
			Rules:    nil,
			Channels: nil,
		},
		Export: ExportConfig{
			DefaultFormat: "json",
		},
		Logging: LoggingConfig{
			Level: "warn",
			Path:  "",
		},
	}
}

// ConfigDir 返回配置文件目录
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".agent-insight"), nil
}

// ConfigPath 返回配置文件路径
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DBPath 返回数据库文件路径，优先级：环境变量 > 配置 > 默认
func DBPath(cfg *Config) (string, error) {
	if p := os.Getenv("AGENT_INSIGHT_DB_PATH"); p != "" {
		return p, nil
	}
	if cfg.Storage.Path != "" {
		return cfg.Storage.Path, nil
	}
	dir, err := ConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config dir: %w", err)
	}
	return filepath.Join(dir, "insight.db"), nil
}

// NewViper 创建配置好的 viper 实例
func NewViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("AGENT_INSIGHT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetConfigType("yaml")
	return v
}

// Load 加载配置，优先级：CLI flag > 环境变量 > 配置文件 > 默认值
func Load() (*Config, error) {
	cfg := DefaultConfig()

	v := NewViper()

	configPath, err := ConfigPath()
	if err != nil {
		return cfg, nil
	}

	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return cfg, nil
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}

// Save 保存配置到文件
func Save(cfg *Config) error {
	configDir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	v := NewViper()
	v.Set("storage", cfg.Storage)
	v.Set("collector", cfg.Collector)
	v.Set("dashboard", cfg.Dashboard)
	v.Set("stats", cfg.Stats)
	v.Set("alerts", cfg.Alerts)
	v.Set("export", cfg.Export)
	v.Set("logging", cfg.Logging)

	return v.WriteConfigAs(configPath)
}

// ParseAggregationInterval 解析聚合间隔字符串
func ParseAggregationInterval(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid aggregation interval %q: %w", s, err)
	}
	return d, nil
}
