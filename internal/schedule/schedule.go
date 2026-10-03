// Package schedule 提供定时同步的计划配置与循环执行逻辑，
// 供系统服务、CLI start 和 TUI 定时同步共用，避免在各入口重复实现定时流程。
package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// SettingKey 是定时配置在 settings 表中保存所用的键名。
const SettingKey = "schedule"

// Setting 保存定时同步的 Cron 表达式与启用开关。
type Setting struct {
	Cron    string `json:"cron"`
	Enabled bool   `json:"enabled"`
}

// Parse 解析 settings 中保存的 JSON；空值返回默认启用且未设定时间的配置。
func Parse(value string) (Setting, error) {
	if strings.TrimSpace(value) == "" {
		return Setting{Enabled: true}, nil
	}
	var setting Setting
	if err := json.Unmarshal([]byte(value), &setting); err != nil {
		return Setting{}, fmt.Errorf("解析定时设置失败：%w", err)
	}
	return setting, nil
}

// Marshal 将配置编码为 JSON，便于写入 settings 表。
func (s Setting) Marshal() (string, error) {
	content, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("编码定时设置失败：%w", err)
	}
	return string(content), nil
}

// ResolveExpression 返回有效的 Cron 表达式；未配置时按当前时刻生成每天执行一次的默认计划。
func ResolveExpression(expression string, now time.Time) string {
	if strings.TrimSpace(expression) == "" {
		return fmt.Sprintf("%d %d * * *", now.Minute(), now.Hour())
	}
	return strings.TrimSpace(expression)
}

// Validate 校验 Cron 表达式；空串视为“未设定，按默认每日计划”，合法。
func Validate(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return nil
	}
	if _, err := cron.ParseStandard(expression); err != nil {
		return fmt.Errorf("Cron 表达式无效：%w", err)
	}
	return nil
}

// NextRun 返回给定 Cron 表达式的下一次执行时间；未设定表达式时按启动时刻推导，
// 表达式非法或无下一次时间时 ok 为 false。
func NextRun(expression string, now time.Time) (time.Time, bool) {
	parsed, err := cron.ParseStandard(ResolveExpression(expression, now))
	if err != nil {
		return time.Time{}, false
	}
	next := parsed.Next(now)
	return next, !next.IsZero()
}

// StartSuccessMessage 是定时任务启动成功的提示文案。
func StartSuccessMessage(expression string) string {
	return "定时任务启动成功：" + expression
}

// NextRunMessage 是打印下次执行时间的提示文案。
func NextRunMessage(next time.Time) string {
	return "下次执行时间：" + next.Format(time.DateTime)
}

// StoppedMessage 是定时任务被取消停止时的提示文案。
const StoppedMessage = "定时同步已停止"

// Run 先立即执行一次 onRun，再按 expression 循环执行，直到 ctx 被取消。
// output 用于打印启动、下次执行时间和停止等提示，允许为空。
func Run(ctx context.Context, expression string, onRun func(), output func(string)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if output == nil {
		output = func(string) {}
	}
	schedule, err := cron.ParseStandard(expression)
	if err != nil {
		return fmt.Errorf("解析 Cron 表达式失败：%w", err)
	}
	output(StartSuccessMessage(expression))
	onRun()
	if ctx.Err() != nil {
		output(StoppedMessage)
		return nil
	}
	next := schedule.Next(time.Now())
	if next.IsZero() {
		return fmt.Errorf("Cron 表达式没有下一次执行时间：%s", expression)
	}
	output(NextRunMessage(next))
	for {
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			output(StoppedMessage)
			return nil
		case <-timer.C:
			onRun()
			if ctx.Err() != nil {
				output(StoppedMessage)
				return nil
			}
			next = schedule.Next(time.Now())
			if next.IsZero() {
				return fmt.Errorf("Cron 表达式没有下一次执行时间：%s", expression)
			}
			output(NextRunMessage(next))
		}
	}
}
