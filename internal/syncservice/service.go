// Package syncservice coordinates certificate synchronization for local applications.
package syncservice

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
)

const (
	defaultWaitMinutes          = 10
	pollingInterval             = 10 * time.Second
	otherRetryInterval          = 6 * time.Second
	otherRetryAttempts          = 3
	certificateFetchConcurrency = 3
)

const CertdSettingKey = "certd"

type SiteStore interface {
	ListEnabledSites(appId uint) ([]storeRepo.AppSite, error)
	UpdateSyncStatus(siteId uint, status, syncError string) error
}

type SiteScanStore interface {
	SiteStore
	ListSites(appId uint) ([]storeRepo.AppSite, error)
	SyncSites(appId uint, sites []storeRepo.AppSite) error
}

type AppStore interface {
	List() ([]storeRepo.TargetApp, error)
}

type MissingAppDisabler interface {
	DisableMissingApps() ([]storeRepo.TargetApp, error)
}

type SettingsStore interface {
	GetSetting(key string) (string, error)
}

type ProviderRegistry interface {
	Find(appType string) (app_provider.Provider, bool)
}

type CertificateClient interface {
	GetCertificate(domains []string) (certd.Certificate, error)
	SendDefaultNotification(title, content string) error
}

type ClientFactory func(certd.Config) CertificateClient

type Config struct {
	Certd          certd.Config
	MachineName    string
	MaxWaitMinutes int
	Progress       func(string)
}

type CertdSetting struct {
	BaseURL        string `json:"baseUrl"`
	KeyId          string `json:"keyId"`
	KeySecret      string `json:"keySecret"`
	MachineName    string `json:"machineName"`
	MaxWaitMinutes int    `json:"maxWaitMinutes"`
}

func ParseCertdSetting(value string) (CertdSetting, error) {
	var setting CertdSetting
	if err := json.Unmarshal([]byte(value), &setting); err != nil {
		return CertdSetting{}, fmt.Errorf("解析 Certd 接口设置失败：%w", err)
	}
	return setting, nil
}

func (s CertdSetting) SyncConfig(progress func(string)) Config {
	return Config{
		Certd:          certd.Config{BaseURL: s.BaseURL, KeyId: s.KeyId, KeySecret: s.KeySecret},
		MachineName:    s.MachineName,
		MaxWaitMinutes: s.MaxWaitMinutes,
		Progress:       progress,
	}
}

type Result struct {
	Succeeded int
	Skipped   int
	Errors    []string
	Canceled  bool
}

type Service struct {
	sites     SiteStore
	providers ProviderRegistry
	newClient ClientFactory
	sleep     func(time.Duration)
}

func New(sites SiteStore, providers ProviderRegistry, factory ClientFactory) *Service {
	if factory == nil {
		factory = func(config certd.Config) CertificateClient { return certd.NewClient(config) }
	}
	return &Service{sites: sites, providers: providers, newClient: factory, sleep: time.Sleep}
}

func (s *Service) RunConfigured(ctx context.Context, apps AppStore, settings SettingsStore, progress func(string)) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return Result{Canceled: true}
	}
	if apps == nil {
		return Result{Errors: []string{"应用仓库未初始化"}}
	}
	if settings == nil {
		return Result{Errors: []string{"设置仓库未初始化"}}
	}
	if s.sites == nil {
		return Result{Errors: []string{"站点仓库未初始化"}}
	}
	if s.providers == nil {
		return Result{Errors: []string{"应用 Provider 未注册"}}
	}
	if disabler, ok := apps.(MissingAppDisabler); ok {
		disabled, disableErr := disabler.DisableMissingApps()
		if disableErr != nil {
			return Result{Errors: []string{"检查已登记应用失败：" + disableErr.Error()}}
		}
		if len(disabled) > 0 {
			publishProgress(progress, fmt.Sprintf("已禁用 %d 个不存在的应用目录", len(disabled)))
		}
	}
	value, err := settings.GetSetting(CertdSettingKey)
	if err != nil {
		return Result{Errors: []string{"读取 Certd 接口设置失败：" + err.Error()}}
	}
	setting, err := ParseCertdSetting(value)
	if err != nil {
		return Result{Errors: []string{err.Error()}}
	}
	registeredApps, err := apps.List()
	if err != nil {
		return Result{Errors: []string{"读取已登记应用失败：" + err.Error()}}
	}
	activeApps := make([]storeRepo.TargetApp, 0, len(registeredApps))
	for _, app := range registeredApps {
		if app.Enabled {
			activeApps = append(activeApps, app)
		}
	}
	if len(activeApps) == 0 {
		return Result{Errors: []string{"暂无已启用应用，无法同步证书"}}
	}
	config := setting.SyncConfig(progress)
	publishProgress(progress, "============ 站点扫描 =============")
	scanErrors, scanSummary, canceled := s.scanRegisteredSites(ctx, activeApps, progress)
	if canceled {
		return Result{Canceled: true}
	}
	publishProgress(progress, fmt.Sprintf("站点扫描完成：扫描 %d 个应用，发现 %d 个站点，禁用 %d 个，HTTPS %d 个，新增 %d 个，失败 %d 个", len(activeApps), scanSummary.siteCount, scanSummary.disabledCount, scanSummary.httpsCount, scanSummary.newCount, len(scanErrors)))
	publishProgress(progress, "============ 证书同步 =============")
	client := s.newClient(config.Certd)
	result := s.runWithClient(ctx, activeApps, config, client, false)
	result.Errors = append(scanErrors, result.Errors...)
	if len(result.Errors) > 0 && !result.Canceled {
		if err := s.notify(client, config, result.Errors); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	}
	return result
}

func (s *Service) Run(ctx context.Context, apps []storeRepo.TargetApp, config Config) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return Result{Canceled: true}
	}
	if s.sites == nil {
		return Result{Errors: []string{"站点仓库未初始化"}}
	}
	if s.providers == nil {
		return Result{Errors: []string{"应用 Provider 未注册"}}
	}
	return s.runWithClient(ctx, apps, config, s.newClient(config.Certd), true)
}

func (s *Service) runWithClient(ctx context.Context, apps []storeRepo.TargetApp, config Config, client CertificateClient, notify bool) Result {
	result := Result{}
	for _, app := range apps {
		if s.syncApp(ctx, app, client, config, &result) {
			result.Canceled = true
			break
		}
	}
	if notify && len(result.Errors) > 0 && !result.Canceled {
		if err := s.notify(client, config, result.Errors); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	}
	return result
}

func (s *Service) notify(client CertificateClient, config Config, failures []string) error {
	if client == nil || len(failures) == 0 {
		return nil
	}
	if err := client.SendDefaultNotification(NotificationTitle(config.MachineName, len(failures)), strings.Join(failures, "\n")); err != nil {
		return fmt.Errorf("发送 Certd 默认通知失败：%w", err)
	}
	return nil
}

func (s *Service) syncApp(ctx context.Context, app storeRepo.TargetApp, client CertificateClient, config Config, result *Result) bool {
	if ctx.Err() != nil {
		return true
	}
	appLabel := AppLabel(app)
	s.publish(config, fmt.Sprintf("同步中：开始处理 %s，目录 %s", appLabel, app.RootDir))
	sites, err := s.sites.ListEnabledSites(app.ID)
	if err != nil {
		s.fail(result, fmt.Sprintf("%s：读取站点失败：%v", appLabel, err))
		s.publish(config, fmt.Sprintf("同步中：%s 读取站点失败：%v", appLabel, err))
		return false
	}
	provider, ok := s.providers.Find(app.AppType)
	if !ok {
		s.fail(result, fmt.Sprintf("%s：未找到 Provider", appLabel))
		return false
	}
	deployer, ok := provider.(app_provider.CertificateDeployer)
	if !ok {
		s.fail(result, fmt.Sprintf("%s：Provider 不支持证书部署", appLabel))
		return false
	}

	pending := make([]pendingCertificateSite, 0, len(sites))
	for _, site := range sites {
		if ctx.Err() != nil {
			return true
		}
		siteLabel := SiteLabel(app, site)
		if !site.Https {
			s.updateStatus(site.ID, "skipped", "非 HTTPS 站点")
			s.publish(config, fmt.Sprintf("同步中：%s 为非 HTTPS 站点，跳过", siteLabel))
			continue
		}
		s.updateStatus(site.ID, "syncing", "")
		providerSite := toProviderSite(site)
		pending = append(pending, pendingCertificateSite{
			record: site, providerSite: providerSite, localExpiry: localCertificateExpiry(provider, providerSite), label: siteLabel,
		})
	}
	fetched, canceled := s.fetchCertificates(ctx, client, config, pending)
	if canceled {
		return true
	}
	deployed := make([]deployedSite, 0, len(fetched))
	for _, item := range fetched {
		if ctx.Err() != nil {
			return true
		}
		site := item.record
		siteLabel := item.label
		if item.err != nil {
			if ctx.Err() != nil {
				return true
			}
			s.updateStatus(site.ID, "failed", item.err.Error())
			s.fail(result, fmt.Sprintf("%s：%v", siteLabel, item.err))
			s.publish(config, fmt.Sprintf("同步中：%s 请求失败：%v", siteLabel, item.err))
			continue
		}
		if !item.localExpiry.IsZero() && !item.certificate.NotAfter.IsZero() && !item.localExpiry.Before(item.certificate.NotAfter) {
			s.updateStatus(site.ID, "synced", "")
			result.Skipped++
			s.publish(config, fmt.Sprintf("同步中：%s 本地证书仍有效，跳过部署", siteLabel))
			continue
		}
		if err := deployer.DeployCertificate(item.providerSite, item.certificate); err != nil {
			s.updateStatus(site.ID, "failed", err.Error())
			s.fail(result, fmt.Sprintf("%s：部署失败：%v", siteLabel, err))
			s.publish(config, fmt.Sprintf("同步中：%s 部署失败：%v", siteLabel, err))
			continue
		}
		deployed = append(deployed, deployedSite{record: site, site: item.providerSite, remote: item.certificate})
	}
	return s.completeDeployments(ctx, app, provider, deployed, config, result)
}

type pendingCertificateSite struct {
	record       storeRepo.AppSite
	providerSite app_provider.Site
	localExpiry  time.Time
	label        string
}

type certificateFetchResult struct {
	pendingCertificateSite
	certificate certd.Certificate
	err         error
}

func (s *Service) fetchCertificates(ctx context.Context, client CertificateClient, config Config, pending []pendingCertificateSite) ([]certificateFetchResult, bool) {
	results := make([]certificateFetchResult, len(pending))
	if len(pending) == 0 {
		return results, false
	}
	workerCount := certificateFetchConcurrency
	if len(pending) < workerCount {
		workerCount = len(pending)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case index, open := <-jobs:
					if !open {
						return
					}
					item := pending[index]
					s.publish(config, fmt.Sprintf("同步中：正在请求 %s 证书", item.label))
					certificate, err := s.fetchCertificate(ctx, client, item.record.Domains, item.record.PrimaryDomain, config, item.label)
					results[index] = certificateFetchResult{pendingCertificateSite: item, certificate: certificate, err: err}
				}
			}
		}()
	}
	for index := range pending {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return results, true
		case jobs <- index:
		}
	}
	close(jobs)
	workers.Wait()
	return results, ctx.Err() != nil
}

type deployedSite struct {
	record storeRepo.AppSite
	site   app_provider.Site
	remote certd.Certificate
}

func (s *Service) completeDeployments(ctx context.Context, app storeRepo.TargetApp, provider app_provider.Provider, deployed []deployedSite, config Config, result *Result) bool {
	if len(deployed) == 0 {
		return false
	}
	if restarter, supported := provider.(app_provider.CertificateRestarter); supported {
		if ctx.Err() != nil {
			return true
		}
		appLabel := AppLabel(app)
		s.publish(config, fmt.Sprintf("同步中：%s 正在重启应用以使新证书生效", appLabel))
		if err := restarter.Restart(app_provider.App{RootDir: app.RootDir, AppType: app.AppType}); err != nil {
			for _, item := range deployed {
				siteLabel := SiteLabel(app, item.record)
				s.updateStatus(item.record.ID, "failed", err.Error())
				s.fail(result, fmt.Sprintf("%s：重启服务失败：%v", siteLabel, err))
			}
			s.publish(config, fmt.Sprintf("同步中：%s 重启应用失败：%v", appLabel, err))
			return false
		}
		s.publish(config, fmt.Sprintf("同步中：%s 应用重启完成", appLabel))
	}
	for _, item := range deployed {
		if ctx.Err() != nil {
			return true
		}
		siteLabel := SiteLabel(app, item.record)
		if inspector, supported := provider.(app_provider.CertificateInspector); supported {
			activeExpiry, err := inspector.LocalCertificateExpiry(item.site)
			if err != nil || activeExpiry.Before(item.remote.NotAfter) {
				if err == nil {
					err = fmt.Errorf("部署后检测到的证书有效期未更新")
				}
				s.updateStatus(item.record.ID, "failed", err.Error())
				s.fail(result, fmt.Sprintf("%s：部署后验证失败：%v", siteLabel, err))
				continue
			}
		}
		s.updateStatus(item.record.ID, "synced", "")
		result.Succeeded++
		s.publish(config, fmt.Sprintf("同步中：%s 部署完成", siteLabel))
	}
	return false
}

func (s *Service) fetchCertificate(ctx context.Context, client CertificateClient, domains, primaryDomain string, config Config, siteLabel string) (certd.Certificate, error) {
	if domains == "" {
		domains = primaryDomain
	}
	attempts := pollingAttempts(config.MaxWaitMinutes)
	maxAttempts := attempts
	pendingObserved := false
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return certd.Certificate{}, err
		}
		certificate, err := client.GetCertificate(splitDomains(domains))
		if err == nil && (certificate.CertificatePEM != "" || certificate.PfxBase64 != "") {
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
			pending := errors.As(err, &apiErr) && apiErr.Code == certd.ErrCodeOpenCertApplying
			if pending {
				pendingObserved = true
				if attempt+1 < attempts {
					s.publish(config, fmt.Sprintf("同步中：%s：%v，等待 %d 秒后重新检查（%d/%d）", siteLabel, lastErr, int(pollingInterval/time.Second), attempt+1, attempts-1))
				}
			} else if pendingObserved {
				return certd.Certificate{}, lastErr
			}
		}
		if !pendingObserved && maxAttempts > otherRetryAttempts {
			maxAttempts = otherRetryAttempts
		}
		if attempt+1 < maxAttempts {
			if !pendingObserved {
				s.publish(config, fmt.Sprintf("同步中：%s 请求失败：%v，等待 %d 秒后重试（%d/%d）", siteLabel, lastErr, int(otherRetryInterval/time.Second), attempt+1, maxAttempts-1))
				if !s.wait(ctx, otherRetryInterval) {
					return certd.Certificate{}, ctx.Err()
				}
			} else {
				if !s.wait(ctx, pollingInterval) {
					return certd.Certificate{}, ctx.Err()
				}
			}
		}
	}
	return certd.Certificate{}, lastErr
}

func (s *Service) wait(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return ctx.Err() == nil
	}
	if s.sleep != nil {
		done := make(chan struct{})
		go func() {
			s.sleep(duration)
			close(done)
		}()
		select {
		case <-ctx.Done():
			return false
		case <-done:
			return true
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Service) updateStatus(siteId uint, status, detail string) {
	_ = s.sites.UpdateSyncStatus(siteId, status, detail)
}

func (s *Service) fail(result *Result, message string) {
	result.Errors = append(result.Errors, message)
}

type siteScanSummary struct {
	siteCount     int
	disabledCount int
	httpsCount    int
	newCount      int
}

func (s *Service) scanRegisteredSites(ctx context.Context, apps []storeRepo.TargetApp, progress func(string)) ([]string, siteScanSummary, bool) {
	store, ok := s.sites.(SiteScanStore)
	if !ok {
		return []string{"站点仓库不支持站点扫描"}, siteScanSummary{}, false
	}
	var failures []string
	var summary siteScanSummary
	for _, app := range apps {
		if ctx.Err() != nil {
			return failures, summary, true
		}
		appLabel := AppLabel(app)
		publishProgress(progress, fmt.Sprintf("站点扫描中：开始处理 %s，目录 %s", appLabel, app.RootDir))
		provider, found := s.providers.Find(app.AppType)
		if !found {
			failures = append(failures, fmt.Sprintf("%s：未找到 Provider", appLabel))
			continue
		}
		sites, err := provider.ScanSites(app_provider.App{RootDir: app.RootDir, AppType: app.AppType})
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s：站点扫描失败：%v", appLabel, err))
			continue
		}
		records := make([]storeRepo.AppSite, 0, len(sites))
		for _, site := range sites {
			records = append(records, storeRepo.AppSite{
				PrimaryDomain:   site.PrimaryDomain,
				Domains:         strings.Join(site.Domains, ","),
				SubdomainCount:  site.SubdomainCount,
				ConfigPath:      site.ConfigPath,
				CertificatePath: site.CertificatePath,
				PrivateKeyPath:  site.PrivateKeyPath,
				DeploymentName:  site.DeploymentName,
				Https:           site.Https,
			})
		}
		existing, err := store.ListSites(app.ID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s：读取旧站点失败：%v", appLabel, err))
			continue
		}
		known := make(map[string]bool, len(existing))
		for _, site := range existing {
			known[site.PrimaryDomain+"\x00"+site.ConfigPath] = site.Enabled
		}
		summary.siteCount += len(records)
		for _, site := range records {
			enabled, exists := known[site.PrimaryDomain+"\x00"+site.ConfigPath]
			if !exists {
				summary.newCount++
			} else if !enabled {
				summary.disabledCount++
			}
			if site.Https && (!exists || enabled) {
				summary.httpsCount++
			}
		}
		if err := store.SyncSites(app.ID, records); err != nil {
			failures = append(failures, fmt.Sprintf("%s：站点写入失败：%v", appLabel, err))
		}
	}
	return failures, summary, false
}

func (s *Service) publish(config Config, message string) {
	if config.Progress != nil {
		config.Progress(message)
	}
}

func publishProgress(progress func(string), message string) {
	if progress != nil {
		progress(message)
	}
}

func AppLabel(app storeRepo.TargetApp) string {
	return fmt.Sprintf("应用[%s]", app.AppType)
}

func SiteLabel(app storeRepo.TargetApp, site storeRepo.AppSite) string {
	return fmt.Sprintf("%s 站点[Id=%d, %s]", AppLabel(app), site.ID, site.PrimaryDomain)
}

func NotificationTitle(machine string, failureCount int) string {
	return fmt.Sprintf("【Certd Client】 证书同步失败【数量：%d】（%s）", failureCount, machineName(machine))
}

func toProviderSite(site storeRepo.AppSite) app_provider.Site {
	return app_provider.Site{
		PrimaryDomain:   site.PrimaryDomain,
		Domains:         splitDomains(site.Domains),
		ConfigPath:      site.ConfigPath,
		CertificatePath: site.CertificatePath,
		PrivateKeyPath:  site.PrivateKeyPath,
		DeploymentName:  site.DeploymentName,
		Https:           site.Https,
	}
}

func localCertificateExpiry(provider app_provider.Provider, site app_provider.Site) time.Time {
	if inspector, supported := provider.(app_provider.CertificateInspector); supported {
		expiry, _ := inspector.LocalCertificateExpiry(site)
		return expiry
	}
	expiry, _ := certificateExpiry(site.CertificatePath)
	return expiry
}

func pollingAttempts(maxWaitMinutes int) int {
	if maxWaitMinutes < 1 {
		maxWaitMinutes = defaultWaitMinutes
	}
	attempts := maxWaitMinutes * int(time.Minute/pollingInterval)
	if attempts < 1 {
		return 1
	}
	return attempts
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
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' })
}

func machineName(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
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
