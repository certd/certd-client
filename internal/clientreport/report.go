// Package clientreport 周期向 Certd 上报客户端心跳，让管理端能展示在线状态与站点统计。
package clientreport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/certd/certd-client/internal/certd"
	"github.com/certd/certd-client/internal/logging"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
	"github.com/certd/certd-client/internal/version"
)

const (
	heartbeatInterval = 10 * time.Minute
	clientSettingKey  = "client"
)

// HeartbeatClient 是心跳上报所需的最小 Certd 客户端能力。
type HeartbeatClient interface {
	Heartbeat(certd.HeartbeatPayload) error
}

// ClientFactory 构造心跳客户端，便于测试注入。
type ClientFactory func(certd.Config) HeartbeatClient

// SettingsStore 读写客户端设置。
type SettingsStore interface {
	GetSetting(key string) (string, error)
	SaveSetting(key, value string) error
}

// AppStore 读取已登记应用，用于统计站点数量。
type AppStore interface {
	List() ([]storeRepo.TargetApp, error)
}

type Reporter struct {
	mu        sync.Mutex
	settings  SettingsStore
	apps      AppStore
	newClient ClientFactory
	logger    logging.Log
}

func New(settings SettingsStore, apps AppStore, logger logging.Log) *Reporter {
	return &Reporter{
		settings: settings,
		apps:     apps,
		newClient: func(config certd.Config) HeartbeatClient {
			return certd.NewClient(config)
		},
		logger: logger,
	}
}

// Run 立即上报一次心跳，之后每 10 分钟上报一次，直到 ctx 被取消。
func (r *Reporter) Run(ctx context.Context) {
	r.Report(ctx)
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Report(ctx)
		}
	}
}

// Report 上报一次心跳；任何一步失败只记录日志，不影响后续上报。
func (r *Reporter) Report(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	if r.settings == nil {
		return
	}
	setting, err := r.loadCertdSetting()
	if err != nil {
		r.loggerError("心跳：读取 Certd 接口设置失败：%v", err)
		return
	}
	if setting.BaseURL == "" || setting.KeyId == "" || setting.KeySecret == "" {
		return
	}
	clientId, err := r.ensureClientId()
	if err != nil {
		r.loggerError("心跳：获取客户端标识失败：%v", err)
		return
	}
	stats, err := r.collectStats()
	if err != nil {
		r.loggerError("心跳：读取应用列表失败：%v", err)
		return
	}
	client := r.newClient(certd.Config{BaseURL: setting.BaseURL, KeyId: setting.KeyId, KeySecret: setting.KeySecret})
	payload := certd.HeartbeatPayload{
		ClientId:        clientId,
		MachineName:     machineName(setting.MachineName),
		Version:         version.String(),
		Os:              runtime.GOOS,
		AppCount:        stats.appCount,
		SiteCount:       stats.siteCount,
		HttpsSiteCount:  stats.httpsSiteCount,
		SyncedSiteCount: stats.syncedSiteCount,
		FailedSiteCount: stats.failedSiteCount,
	}
	r.loggerInfo("正在上报心跳：机器 %s，版本 %s，应用 %d，站点 %d，HTTPS %d，已同步 %d，异常 %d",
		payload.MachineName, payload.Version, payload.AppCount, payload.SiteCount,
		payload.HttpsSiteCount, payload.SyncedSiteCount, payload.FailedSiteCount)
	if err := client.Heartbeat(payload); err != nil {
		r.loggerError("心跳上报失败：%v", err)
		return
	}
	r.loggerInfo("心跳上报成功：站点 %d，HTTPS %d", stats.siteCount, stats.httpsSiteCount)
}

func (r *Reporter) loadCertdSetting() (syncservice.CertdSetting, error) {
	value, err := r.settings.GetSetting(syncservice.CertdSettingKey)
	if err != nil {
		return syncservice.CertdSetting{}, err
	}
	if value == "" {
		return syncservice.CertdSetting{}, nil
	}
	return syncservice.ParseCertdSetting(value)
}

func (r *Reporter) ensureClientId() (string, error) {
	value, err := r.settings.GetSetting(clientSettingKey)
	if err != nil {
		return "", err
	}
	if value != "" {
		var setting clientSetting
		if err := json.Unmarshal([]byte(value), &setting); err == nil && setting.ClientId != "" {
			return setting.ClientId, nil
		}
	}
	clientId, err := generateClientId()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(clientSetting{ClientId: clientId})
	if err != nil {
		return "", err
	}
	if err := r.settings.SaveSetting(clientSettingKey, string(data)); err != nil {
		return "", err
	}
	return clientId, nil
}

type clientSetting struct {
	ClientId string `json:"clientId"`
}

type clientStats struct {
	appCount        int
	siteCount       int
	httpsSiteCount  int
	syncedSiteCount int
	failedSiteCount int
}

func (r *Reporter) collectStats() (clientStats, error) {
	var stats clientStats
	if r.apps == nil {
		return stats, nil
	}
	apps, err := r.apps.List()
	if err != nil {
		return stats, err
	}
	for _, app := range apps {
		if !app.Enabled {
			continue
		}
		stats.appCount++
		stats.siteCount += app.SiteCount
		stats.httpsSiteCount += app.HttpsSiteCount
		stats.syncedSiteCount += app.SyncedSiteCount
		stats.failedSiteCount += app.FailedSiteCount
	}
	return stats, nil
}

func (r *Reporter) loggerInfo(format string, args ...any) {
	if r.logger != nil {
		r.logger.Info(format, args...)
	}
}

func (r *Reporter) loggerError(format string, args ...any) {
	if r.logger != nil {
		r.logger.Error(format, args...)
	}
}

func generateClientId() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func machineName(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	host, _ := os.Hostname()
	return strings.TrimSpace(host)
}
