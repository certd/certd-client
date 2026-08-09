package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	storeRepo "github.com/certd/certd-client/internal/store/repo"
	"github.com/certd/certd-client/internal/syncservice"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	defaultCertificateWaitMinutes = 10
)

type certdSetting = syncservice.CertdSetting

type certificateSyncResult struct {
	Succeeded int
	Skipped   int
	Errors    []string
	Canceled  bool
}

func (m *Model) loadCertdSettings() {
	if m.settingsRepo == nil {
		return
	}
	value, err := m.settingsRepo.GetSetting(syncservice.CertdSettingKey)
	if err != nil || value == "" {
		return
	}
	setting, err := syncservice.ParseCertdSetting(value)
	if err != nil {
		m.appendLog("读取 Certd 接口设置失败：" + err.Error())
		return
	}
	m.certdInputs[0].SetValue(setting.BaseURL)
	m.certdInputs[1].SetValue(setting.KeyId)
	m.certdInputs[2].SetValue(setting.KeySecret)
	m.certdInputs[3].SetValue(setting.MachineName)
	if setting.MaxWaitMinutes > 0 {
		m.certdInputs[4].SetValue(strconv.Itoa(setting.MaxWaitMinutes))
	} else {
		m.certdInputs[4].SetValue("")
	}
}

func (m Model) updateCertdSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.certdInputs[m.certdFocus].Blur()
		m.screen = homeScreen
		m.status = "已取消 Certd 接口设置"
	case "tab", "down":
		m.certdInputs[m.certdFocus].Blur()
		m.certdFocus = (m.certdFocus + 1) % len(m.certdInputs)
		m.certdInputs[m.certdFocus].Focus()
	case "shift+tab", "up":
		m.certdInputs[m.certdFocus].Blur()
		m.certdFocus = (m.certdFocus + len(m.certdInputs) - 1) % len(m.certdInputs)
		m.certdInputs[m.certdFocus].Focus()
	case "enter":
		if m.settingsRepo == nil {
			m.status = "设置仓库未初始化"
			m.appendLog(m.status)
			return m, nil
		}
		maxWaitMinutes, err := certificateWaitMinutes(m.certdInputs[4].Value())
		if err != nil {
			m.status = "最长等待时长必须是正整数分钟"
			m.appendLog(m.status)
			return m, nil
		}
		setting := certdSetting{
			BaseURL:        strings.TrimSpace(m.certdInputs[0].Value()),
			KeyId:          strings.TrimSpace(m.certdInputs[1].Value()),
			KeySecret:      m.certdInputs[2].Value(),
			MachineName:    strings.TrimSpace(m.certdInputs[3].Value()),
			MaxWaitMinutes: maxWaitMinutes,
		}
		content, err := json.Marshal(setting)
		if err != nil {
			m.status = "编码 Certd 接口设置失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		if err := m.settingsRepo.SaveSetting(syncservice.CertdSettingKey, string(content)); err != nil {
			m.status = "保存 Certd 接口设置失败：" + err.Error()
			m.appendLog(m.status)
			return m, nil
		}
		m.certdInputs[m.certdFocus].Blur()
		m.screen = homeScreen
		m.status = "Certd 接口设置已保存"
		m.appendLog(m.status)
	default:
		var cmd tea.Cmd
		m.certdInputs[m.certdFocus], cmd = m.certdInputs[m.certdFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func certificateWaitMinutes(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultCertificateWaitMinutes, nil
	}
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes < 1 {
		return 0, fmt.Errorf("invalid max wait minutes")
	}
	return minutes, nil
}

// syncCertificatesService adapts the application service result to Bubble Tea.
func (m Model) syncCertificatesService(ctx context.Context, apps []storeRepo.TargetApp, progress chan string) tea.Cmd {
	return func() tea.Msg {
		result := certificateSyncResult{}
		if m.settingsRepo == nil {
			result.Errors = append(result.Errors, "设置仓库未初始化")
			return certificateSyncCompletedMsg{result: result}
		}
		if m.siteRepo == nil {
			result.Errors = append(result.Errors, "站点仓库未初始化")
			return certificateSyncCompletedMsg{result: result}
		}
		if m.providers == nil {
			result.Errors = append(result.Errors, "应用 Provider 未注册")
			return certificateSyncCompletedMsg{result: result}
		}
		value, err := m.settingsRepo.GetSetting(syncservice.CertdSettingKey)
		if err != nil {
			result.Errors = append(result.Errors, "读取 Certd 接口设置失败："+err.Error())
			return certificateSyncCompletedMsg{result: result}
		}
		setting, err := syncservice.ParseCertdSetting(value)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			return certificateSyncCompletedMsg{result: result}
		}
		service := syncservice.New(m.siteRepo, m.providers, nil)
		syncResult := service.Run(ctx, apps, setting.SyncConfig(func(message string) {
			publishSyncProgress(progress, message)
		}))
		return certificateSyncCompletedMsg{result: certificateSyncResult{
			Succeeded: syncResult.Succeeded,
			Skipped:   syncResult.Skipped,
			Errors:    syncResult.Errors,
			Canceled:  syncResult.Canceled,
		}}
	}
}

func certificateSyncNotificationTitle(machineName string, failureCount int) string {
	return syncservice.NotificationTitle(machineName, failureCount)
}

func publishSyncProgress(progress chan string, message string) {
	if progress == nil {
		return
	}
	select {
	case progress <- message:
	default:
		select {
		case <-progress:
		default:
		}
		select {
		case progress <- message:
		default:
		}
	}
}

func (m *Model) readSyncProgress() {
	if m.syncProgressCh == nil {
		return
	}
	for {
		select {
		case message := <-m.syncProgressCh:
			m.appendLog(message)
		default:
			return
		}
	}
}

func syncProgressTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return syncProgressTickMsg{} })
}
