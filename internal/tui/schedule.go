package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/certd/certd-client/internal/schedule"
	tea "github.com/charmbracelet/bubbletea"
)

// loadSchedule 从 settings 表读取定时配置，填充 Cron 输入框与启用状态。
// 未配置时保持默认：启用且表达式为空（按启动时刻每天执行一次）。
func (m *Model) loadSchedule() {
	m.scheduleEnabled = true
	m.scheduleInput.SetValue("")
	if m.settingsRepo == nil {
		return
	}
	value, err := m.settingsRepo.GetSetting(schedule.SettingKey)
	if err != nil || value == "" {
		return
	}
	setting, err := schedule.Parse(value)
	if err != nil {
		m.logInfo("读取定时设置失败：" + err.Error())
		return
	}
	m.scheduleInput.SetValue(setting.Cron)
	m.scheduleEnabled = setting.Enabled
}

// updateScheduleSettings 处理定时设置屏幕的键盘事件：
// Esc 返回、e 切换启用、回车校验并保存，其余按键转发给 Cron 输入框。
func (m Model) updateScheduleSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.scheduleInput.Blur()
		m.screen = m.scheduleReturnScreen
		if m.screen == 0 {
			m.screen = homeScreen
		}
		m.status = "已取消定时设置"
	case "e":
		m.scheduleEnabled = !m.scheduleEnabled
		if m.scheduleEnabled {
			m.status = "定时同步已切换为启用，回车保存生效"
		} else {
			m.status = "定时同步已切换为禁用，回车保存生效"
		}
	case "enter":
		if m.settingsRepo == nil {
			m.status = "设置仓库未初始化"
			m.logInfo(m.status)
			return m, nil
		}
		expression := strings.TrimSpace(m.scheduleInput.Value())
		if err := schedule.Validate(expression); err != nil {
			m.status = err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		content, err := schedule.Setting{Cron: expression, Enabled: m.scheduleEnabled}.Marshal()
		if err != nil {
			m.status = "保存定时设置失败：" + err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		if err := m.settingsRepo.SaveSetting(schedule.SettingKey, content); err != nil {
			m.status = "保存定时设置失败：" + err.Error()
			m.logInfo(m.status)
			return m, nil
		}
		m.scheduleInput.Blur()
		m.screen = m.scheduleReturnScreen
		if m.screen == 0 {
			m.screen = homeScreen
		}
		m.status = "定时设置已保存"
		m.logInfo(m.status)
	default:
		var cmd tea.Cmd
		m.scheduleInput, cmd = m.scheduleInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// scheduleSummary 从数据库读取当前定时计划，返回供服务启动确认屏展示的简要描述。
func (m Model) scheduleSummary() string {
	cron := ""
	enabled := true
	if m.settingsRepo != nil {
		if value, err := m.settingsRepo.GetSetting(schedule.SettingKey); err == nil && value != "" {
			if setting, err := schedule.Parse(value); err == nil {
				cron = setting.Cron
				enabled = setting.Enabled
			}
		}
	}
	state := "未启用"
	if enabled {
		state = "已启用"
	}
	if strings.TrimSpace(cron) == "" {
		cron = "默认（每天按当前时刻执行一次）"
	}
	return fmt.Sprintf("%s · %s", state, cron)
}

// viewScheduleSettings 渲染定时设置屏幕：显示启用状态、Cron 输入框和下次执行时间预览。
func (m Model) viewScheduleSettings() string {
	state := "禁用"
	if m.scheduleEnabled {
		state = "启用"
	}
	lines := []string{
		"定时设置",
		"",
		"启用状态：" + state + "（按 e 切换）",
		"Cron 表达式（分 时 日 月 周，留空则按启动时刻每天执行一次）",
		m.scheduleInput.View(),
	}
	expression := strings.TrimSpace(m.scheduleInput.Value())
	if err := schedule.Validate(expression); err != nil {
		lines = append(lines, "", "校验："+err.Error())
	} else if next, ok := schedule.NextRun(expression, time.Now()); ok {
		lines = append(lines, "", schedule.NextRunMessage(next))
	}
	lines = append(lines, "", "回车保存 · e 切换启用 · Esc 返回")
	return strings.Join(lines, "\n")
}
