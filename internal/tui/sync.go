package tui

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	certdSettingKey                   = "certd"
	defaultCertificateWaitMinutes     = 10
	certificatePollingInterval        = 10 * time.Second
	defaultCertificatePollingAttempts = defaultCertificateWaitMinutes * int(time.Minute/certificatePollingInterval)
	otherCertificateRetryInterval     = 6 * time.Second
)

type certdSetting struct {
	BaseURL        string `json:"baseUrl"`
	KeyId          string `json:"keyId"`
	KeySecret      string `json:"keySecret"`
	MachineName    string `json:"machineName"`
	MaxWaitMinutes int    `json:"maxWaitMinutes"`
}

type certificateSyncResult struct {
	Succeeded int
	Skipped   int
	Errors    []string
}

func (m *Model) loadCertdSettings() {
	if m.settingsRepo == nil {
		return
	}
	value, err := m.settingsRepo.GetSetting(certdSettingKey)
	if err != nil || value == "" {
		return
	}
	var setting certdSetting
	if err := json.Unmarshal([]byte(value), &setting); err != nil {
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
		if err := m.settingsRepo.SaveSetting(certdSettingKey, string(content)); err != nil {
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

func certificatePollingAttempts(maxWaitMinutes int) int {
	if maxWaitMinutes < 1 {
		maxWaitMinutes = defaultCertificateWaitMinutes
	}
	attempts := maxWaitMinutes * int(time.Minute/certificatePollingInterval)
	if attempts < 1 {
		return 1
	}
	return attempts
}

func (m Model) syncCertificates(apps []storeRepo.TargetApp, progress chan string) tea.Cmd {
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
		value, err := m.settingsRepo.GetSetting(certdSettingKey)
		if err != nil {
			result.Errors = append(result.Errors, "读取 Certd 接口设置失败："+err.Error())
			return certificateSyncCompletedMsg{result: result}
		}
		var setting certdSetting
		if err := json.Unmarshal([]byte(value), &setting); err != nil {
			result.Errors = append(result.Errors, "解析 Certd 接口设置失败："+err.Error())
			return certificateSyncCompletedMsg{result: result}
		}
		pollingModel := m
		pollingModel.syncAttempts = certificatePollingAttempts(setting.MaxWaitMinutes)
		pollingModel.syncInterval = certificatePollingInterval
		factory := m.certdFactory
		if factory == nil {
			factory = certd.NewClient
		}
		client := factory(certd.Config{BaseURL: setting.BaseURL, KeyId: setting.KeyId, KeySecret: setting.KeySecret})
		for _, app := range apps {
			appLabel := syncAppLabel(app)
			publishSyncProgress(progress, fmt.Sprintf("同步中：开始处理 %s，目录 %s", appLabel, app.RootDir))
			sites, err := m.siteRepo.ListEnabledSites(app.ID)
			if err != nil {
				publishSyncProgress(progress, fmt.Sprintf("同步中：%s 读取站点失败", appLabel))
				result.Errors = append(result.Errors, fmt.Sprintf("%s：读取站点失败：%v", appLabel, err))
				continue
			}
			provider, ok := m.providers.Find(app.AppType)
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("%s：未找到 Provider", appLabel))
				continue
			}
			deployer, ok := provider.(app_provider.CertificateDeployer)
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("%s：Provider 不支持证书部署", appLabel))
				continue
			}
			for _, site := range sites {
				siteLabel := syncSiteLabel(app, site)
				if !site.Https {
					_ = m.siteRepo.UpdateSyncStatus(site.ID, "skipped", "非 HTTPS 站点")
					publishSyncProgress(progress, fmt.Sprintf("同步中：%s 为非 HTTPS 站点，跳过", siteLabel))
					continue
				}
				_ = m.siteRepo.UpdateSyncStatus(site.ID, "syncing", "")
				publishSyncProgress(progress, fmt.Sprintf("同步中：正在请求 %s 证书", siteLabel))
				providerSite := app_provider.Site{
					PrimaryDomain: site.PrimaryDomain, Domains: splitDomains(site.Domains), ConfigPath: site.ConfigPath,
					CertificatePath: site.CertificatePath, PrivateKeyPath: site.PrivateKeyPath, DeploymentName: site.DeploymentName, Https: site.Https,
				}
				localExpiry := time.Time{}
				if inspector, supported := provider.(app_provider.CertificateInspector); supported {
					localExpiry, _ = inspector.LocalCertificateExpiry(providerSite)
				} else {
					localExpiry, _ = certificateExpiry(site.CertificatePath)
				}
				remote, err := pollingModel.fetchCertificate(client, site.Domains, site.PrimaryDomain)
				if err != nil {
					_ = m.siteRepo.UpdateSyncStatus(site.ID, "failed", err.Error())
					publishSyncProgress(progress, fmt.Sprintf("同步中：%s 请求失败：%v", siteLabel, err))
					result.Errors = append(result.Errors, fmt.Sprintf("%s：%v", siteLabel, err))
					continue
				}
				if !localExpiry.IsZero() && !remote.NotAfter.IsZero() && !localExpiry.Before(remote.NotAfter) {
					_ = m.siteRepo.UpdateSyncStatus(site.ID, "synced", "")
					result.Skipped++
					publishSyncProgress(progress, fmt.Sprintf("同步中：%s 本地证书仍有效，跳过部署", siteLabel))
					continue
				}
				if err := deployer.DeployCertificate(providerSite, remote); err != nil {
					_ = m.siteRepo.UpdateSyncStatus(site.ID, "failed", err.Error())
					publishSyncProgress(progress, fmt.Sprintf("同步中：%s 部署失败", siteLabel))
					result.Errors = append(result.Errors, fmt.Sprintf("%s：部署失败：%v", siteLabel, err))
					continue
				}
				if restarter, supported := provider.(app_provider.CertificateRestarter); supported {
					publishSyncProgress(progress, fmt.Sprintf("同步中：%s 正在重启应用以使新证书生效", siteLabel))
					if err := restarter.Restart(app_provider.App{RootDir: app.RootDir, AppType: app.AppType}); err != nil {
						_ = m.siteRepo.UpdateSyncStatus(site.ID, "failed", err.Error())
						publishSyncProgress(progress, fmt.Sprintf("同步中：%s 重启应用失败：%v", siteLabel, err))
						result.Errors = append(result.Errors, fmt.Sprintf("%s：重启服务失败：%v", siteLabel, err))
						continue
					}
					publishSyncProgress(progress, fmt.Sprintf("同步中：%s 应用重启完成", siteLabel))
				}
				if inspector, supported := provider.(app_provider.CertificateInspector); supported {
					activeExpiry, err := inspector.LocalCertificateExpiry(providerSite)
					if err != nil || activeExpiry.Before(remote.NotAfter) {
						if err == nil {
							err = fmt.Errorf("部署后检测到的证书有效期未更新")
						}
						_ = m.siteRepo.UpdateSyncStatus(site.ID, "failed", err.Error())
						result.Errors = append(result.Errors, fmt.Sprintf("%s：部署后验证失败：%v", siteLabel, err))
						continue
					}
				}
				_ = m.siteRepo.UpdateSyncStatus(site.ID, "synced", "")
				result.Succeeded++
				publishSyncProgress(progress, fmt.Sprintf("同步中：%s 部署完成", siteLabel))
			}
		}
		if len(result.Errors) > 0 {
			if err := client.SendDefaultNotification(certificateSyncNotificationTitle(setting.MachineName, len(result.Errors)), strings.Join(result.Errors, "\n")); err != nil {
				result.Errors = append(result.Errors, "发送 Certd 默认通知失败："+err.Error())
			}
		}
		return certificateSyncCompletedMsg{result: result}
	}
}

func syncAppLabel(app storeRepo.TargetApp) string {
	return fmt.Sprintf("应用[%s]", app.AppType)
}

func syncSiteLabel(app storeRepo.TargetApp, site storeRepo.AppSite) string {
	return fmt.Sprintf("%s 站点[Id=%d, %s]", syncAppLabel(app), site.ID, site.PrimaryDomain)
}

func certificateSyncNotificationTitle(machineName string, failureCount int) string {
	if strings.TrimSpace(machineName) == "" {
		machineName = localMachineName()
	}
	return fmt.Sprintf("【Certd Client】 证书同步失败【数量：%d】（%s）", failureCount, machineName)
}

func localMachineName() string {
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	interfaces, _ := net.Interfaces()
	var ipv4 string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, address := range addrs {
			ip := net.ParseIP(strings.Split(address.String(), "/")[0])
			if ip != nil && ip.To4() != nil {
				ipv4 = ip.To4().String()
				break
			}
		}
		if ipv4 != "" {
			break
		}
	}
	if host != "" && ipv4 != "" {
		return host + " " + ipv4
	}
	if host != "" {
		return host
	}
	return "未知主机"
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

func (m Model) fetchCertificate(client *certd.Client, domains, primaryDomain string) (certd.Certificate, error) {
	if domains == "" {
		domains = primaryDomain
	}
	attempts := m.syncAttempts
	if attempts < 1 {
		attempts = 1
	}
	otherAttempts := m.otherRetryAttempts
	if otherAttempts < 1 {
		otherAttempts = 3
	}
	maxAttempts := attempts
	var lastErr error
	pendingObserved := false
	for attempt := 0; attempt < maxAttempts; attempt++ {
		retryInterval := m.otherRetryInterval
		pendingApplication := false
		certificate, err := client.GetCertificate(splitDomains(domains))
		if err == nil && certificate.CertificatePEM != "" || err == nil && certificate.PfxBase64 != "" {
			if certificate.NotAfter.IsZero() && certificate.CertificatePEM != "" {
				certificate.NotAfter, _ = certificateExpiryPEM([]byte(certificate.CertificatePEM))
			}
			if !certificate.NotAfter.IsZero() {
				return certificate, nil
			}
			lastErr = fmt.Errorf("Certd 返回证书但未提供有效期")
		} else {
			if err == nil {
				lastErr = fmt.Errorf("Certd 未返回证书")
			} else {
				lastErr = err
			}
			var apiErr *certd.APIError
			pendingApplication = errors.As(err, &apiErr) && apiErr.Code == certd.ErrCodeOpenCertApplying
			if pendingApplication {
				pendingObserved = true
				retryInterval = m.syncInterval
			} else if pendingObserved {
				return certd.Certificate{}, lastErr
			}
			if pendingApplication && attempt+1 < attempts {
				publishSyncProgress(m.syncProgressCh, fmt.Sprintf("同步中：%v，等待 %d 秒后重新检查（%d/%d）", lastErr, int(m.syncInterval/time.Second), attempt+1, attempts-1))
			}
		}
		if !pendingObserved && maxAttempts > otherAttempts {
			maxAttempts = otherAttempts
		}
		if attempt+1 < maxAttempts {
			if !pendingApplication {
				publishSyncProgress(m.syncProgressCh, fmt.Sprintf("同步中：请求失败：%v，等待 %d 秒后重试（%d/%d）", lastErr, int(retryInterval/time.Second), attempt+1, maxAttempts-1))
			}
			if retryInterval > 0 {
				time.Sleep(retryInterval)
			}
		}
	}
	return certd.Certificate{}, lastErr
}

func certificateExpiry(path string) (time.Time, error) {
	if path == "" {
		return time.Time{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	return certificateExpiryPEM(content)
}

func certificateExpiryPEM(content []byte) (time.Time, error) {
	block, _ := pem.Decode(content)
	if block == nil {
		return time.Time{}, fmt.Errorf("本地证书格式无效")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析本地证书失败：%w", err)
	}
	return certificate.NotAfter, nil
}

func splitDomains(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' })
	return parts
}
