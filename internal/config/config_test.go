package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestDefaultConfig_默认值完整 ---
func TestDefaultConfig_默认值完整(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "sqlite", cfg.Storage.Type)
	assert.Equal(t, "", cfg.Storage.Path)
	assert.Equal(t, 30, cfg.Storage.RetentionDays)
	assert.Equal(t, 10240, cfg.Storage.MaxInputSize)
	assert.Equal(t, 10240, cfg.Storage.MaxOutputSize)
	assert.Equal(t, 5000, cfg.Collector.TimeoutMs)
	assert.Equal(t, 1, cfg.Collector.BatchSize)
	assert.True(t, cfg.Collector.AsyncWrite)
	assert.Equal(t, "127.0.0.1", cfg.Dashboard.Host)
	assert.Equal(t, 8080, cfg.Dashboard.Port)
	assert.Equal(t, 1000, cfg.Dashboard.RefreshIntervalMs)
	assert.Equal(t, "5m", cfg.Stats.AggregationInterval)
	assert.False(t, cfg.Alerts.Enabled)
	assert.Equal(t, "json", cfg.Export.DefaultFormat)
	assert.Equal(t, "warn", cfg.Logging.Level)
	assert.Equal(t, "", cfg.Logging.Path)
}

// --- TestDBPath_环境变量优先 ---
func TestDBPath_环境变量优先(t *testing.T) {
	t.Setenv("AGENT_INSIGHT_DB_PATH", "/custom/path/insight.db")
	cfg := DefaultConfig()
	cfg.Storage.Path = "/config/path/insight.db"
	path, err := DBPath(cfg)
	require.NoError(t, err)
	assert.Equal(t, "/custom/path/insight.db", path)
}

// --- TestDBPath_配置文件路径 ---
func TestDBPath_配置文件路径(t *testing.T) {
	t.Setenv("AGENT_INSIGHT_DB_PATH", "")
	cfg := DefaultConfig()
	cfg.Storage.Path = "/config/path/insight.db"
	path, err := DBPath(cfg)
	require.NoError(t, err)
	assert.Equal(t, "/config/path/insight.db", path)
}

// --- TestDBPath_默认路径 ---
func TestDBPath_默认路径(t *testing.T) {
	t.Setenv("AGENT_INSIGHT_DB_PATH", "")
	cfg := DefaultConfig()
	path, err := DBPath(cfg)
	require.NoError(t, err)
	cwd, _ := os.Getwd()
	expected := filepath.Join(cwd, ".agent-insight", "insight.db")
	assert.Equal(t, expected, path)
}

// --- TestParseAggregationInterval_合法值 ---
func TestParseAggregationInterval_合法值(t *testing.T) {
	d, err := ParseAggregationInterval("5m")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, d)

	d, err = ParseAggregationInterval("1h")
	require.NoError(t, err)
	assert.Equal(t, 1*time.Hour, d)
}

// --- TestParseAggregationInterval_非法值 ---
func TestParseAggregationInterval_非法值(t *testing.T) {
	_, err := ParseAggregationInterval("invalid")
	assert.Error(t, err)
}

// --- TestConfigDir_可获取 ---
func TestConfigDir_可获取(t *testing.T) {
	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Contains(t, dir, ".agent-insight")
}

// --- TestConfigPath_可获取 ---
func TestConfigPath_可获取(t *testing.T) {
	path, err := ConfigPath()
	require.NoError(t, err)
	assert.Contains(t, path, "config.yaml")
}
