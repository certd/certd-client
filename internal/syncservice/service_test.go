package syncservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/certd"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
)

func TestRunRestartsApplicationOnceAfterDeployingMultipleSites(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{
		{ID: 1, PrimaryDomain: "one.example.com", Domains: "one.example.com", Https: true},
		{ID: 2, PrimaryDomain: "two.example.com", Domains: "two.example.com", Https: true},
	}}
	provider := &fakeProvider{}
	service := New(store, app_provider.NewRegistry(provider), fakeClientFactory())

	result := service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "fake", RootDir: "/srv/fake"}}, Config{})

	if provider.deployments != 2 {
		t.Fatalf("expected two certificate deployments, got %d", provider.deployments)
	}
	if provider.restarts != 1 {
		t.Fatalf("expected one application restart, got %d", provider.restarts)
	}
	if result.Succeeded != 2 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunMarksAllDeployedSitesFailedWhenRestartFails(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{
		{ID: 1, PrimaryDomain: "one.example.com", Domains: "one.example.com", Https: true},
		{ID: 2, PrimaryDomain: "two.example.com", Domains: "two.example.com", Https: true},
	}}
	provider := &fakeProvider{restartErr: errors.New("重启失败")}
	service := New(store, app_provider.NewRegistry(provider), fakeClientFactory())

	result := service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "fake", RootDir: "/srv/fake"}}, Config{})

	if provider.restarts != 1 {
		t.Fatalf("expected one application restart, got %d", provider.restarts)
	}
	for _, siteID := range []uint{1, 2} {
		status, ok := store.statuses[siteID]
		if !ok || status.status != "failed" {
			t.Fatalf("expected site %d to be marked failed, got %#v", siteID, status)
		}
	}
	if result.Succeeded != 0 || len(result.Errors) != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunContinuesWithOtherSitesWhenOneDeploymentFails(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{
		{ID: 1, PrimaryDomain: "broken.example.com", Domains: "broken.example.com", Https: true},
		{ID: 2, PrimaryDomain: "ok.example.com", Domains: "ok.example.com", Https: true},
	}}
	provider := &fakeProvider{deployErrFor: map[string]error{"broken.example.com": errors.New("写入失败")}}
	service := New(store, app_provider.NewRegistry(provider), fakeClientFactory())

	result := service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "fake", RootDir: "/srv/fake"}}, Config{})

	if provider.restarts != 1 {
		t.Fatalf("expected one application restart for the successful deployment, got %d", provider.restarts)
	}
	if store.statuses[1].status != "failed" || store.statuses[2].status != "synced" {
		t.Fatalf("unexpected site statuses: %#v", store.statuses)
	}
	if result.Succeeded != 1 || len(result.Errors) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunDoesNotRestartWhenLocalCertificateIsStillValid(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{{
		ID: 1, PrimaryDomain: "current.example.com", Domains: "current.example.com", Https: true,
	}}}
	provider := &fakeInspectableProvider{localExpiry: time.Now().Add(48 * time.Hour)}
	service := New(store, app_provider.NewRegistry(provider), fakeClientFactory())

	result := service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "inspectable", RootDir: "/srv/fake"}}, Config{})

	if provider.deployments != 0 || provider.restarts != 0 {
		t.Fatalf("expected no deployment or restart, deployments=%d restarts=%d", provider.deployments, provider.restarts)
	}
	if result.Skipped != 1 || result.Succeeded != 0 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestFetchCertificateRetriesWhileCertificateIsApplying(t *testing.T) {
	client := &sequenceClient{responses: []certificateResponse{
		{err: &certd.APIError{Code: certd.ErrCodeOpenCertApplying, Message: "证书正在申请中，请稍后重新获取", PipelineId: 101, CertId: 202}},
		{certificate: certd.Certificate{CertificatePEM: "certificate", NotAfter: time.Now().Add(time.Hour)}},
	}}
	service := New(&fakeSiteStore{}, app_provider.NewRegistry(), func(certd.Config) CertificateClient { return client })
	service.sleep = func(time.Duration) {}
	var progress []string

	certificate, err := service.fetchCertificate(context.Background(), client, "example.com", "example.com", Config{MaxWaitMinutes: 1, Progress: func(message string) {
		progress = append(progress, message)
	}}, "应用[nginx] 站点[Id=1, example.com]")

	if err != nil || client.requests != 2 || certificate.CertificatePEM != "certificate" || len(client.pipelineIds) != 2 || client.pipelineIds[0] != 0 || client.pipelineIds[1] != 101 {
		t.Fatalf("expected pending certificate to be retried, requests=%d certificate=%#v err=%v", client.requests, certificate, err)
	}
	if !strings.Contains(strings.Join(progress, "\n"), "应用[nginx] 站点[Id=1, example.com]") {
		t.Fatalf("expected site label in pending progress, got %#v", progress)
	}
}

func TestFetchCertificateFailsImmediatelyWhenApplyingChangesToAnotherError(t *testing.T) {
	client := &sequenceClient{responses: []certificateResponse{
		{err: &certd.APIError{Code: certd.ErrCodeOpenCertApplying, Message: "证书正在申请中，请稍后重新获取"}},
		{err: &certd.APIError{Code: certd.ErrCodeOpenPipelineError, Message: "流水线执行异常，请稍后重试"}},
	}}
	service := New(&fakeSiteStore{}, app_provider.NewRegistry(), func(certd.Config) CertificateClient { return client })
	service.sleep = func(time.Duration) {}

	_, err := service.fetchCertificate(context.Background(), client, "example.com", "example.com", Config{MaxWaitMinutes: 10}, "应用[nginx] 站点[Id=1, example.com]")

	if client.requests != 2 || err == nil || !strings.Contains(err.Error(), "流水线执行异常") {
		t.Fatalf("expected immediate failure after applying status changed, requests=%d err=%v", client.requests, err)
	}
}

func TestRunFetchesThreeCertificatesInParallel(t *testing.T) {
	sites := make([]storeRepo.AppSite, 4)
	for index := range sites {
		sites[index] = storeRepo.AppSite{ID: uint(index + 1), PrimaryDomain: fmt.Sprintf("site-%d.example.com", index+1), Https: true}
	}
	client := newConcurrentCertificateClient()
	service := New(&fakeSiteStore{sites: sites}, app_provider.NewRegistry(&fakeProvider{}), func(certd.Config) CertificateClient { return client })
	completed := make(chan Result, 1)
	go func() {
		completed <- service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "fake", RootDir: "/srv/fake"}}, Config{})
	}()
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(client.release) })

	for index := 0; index < 3; index++ {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatalf("expected request %d to start in parallel", index+1)
		}
	}
	if client.maxActiveRequests() != 3 {
		t.Fatalf("expected exactly three concurrent requests, got %d", client.maxActiveRequests())
	}
	releaseOnce.Do(func() { close(client.release) })
	result := <-completed
	if result.Succeeded != 4 || len(result.Errors) != 0 || client.maxActiveRequests() > 3 {
		t.Fatalf("unexpected concurrent sync result=%#v max=%d", result, client.maxActiveRequests())
	}
}

func TestRunReportsMissingDependencies(t *testing.T) {
	if result := New(nil, nil, nil).Run(context.Background(), nil, Config{}); len(result.Errors) != 1 || result.Errors[0] != "站点仓库未初始化" {
		t.Fatalf("expected missing site store error, got %#v", result)
	}
	if result := New(&fakeSiteStore{}, nil, fakeClientFactory()).Run(context.Background(), nil, Config{}); len(result.Errors) != 1 || result.Errors[0] != "应用 Provider 未注册" {
		t.Fatalf("expected missing provider registry error, got %#v", result)
	}
}

func TestRunSkipsNonHTTPSSiteAndPublishesProgress(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{{ID: 1, PrimaryDomain: "plain.example.com", Https: false}}}
	var progress []string
	service := New(store, app_provider.NewRegistry(&fakeProvider{}), fakeClientFactory())

	result := service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "fake"}}, Config{Progress: func(message string) {
		progress = append(progress, message)
	}})

	if len(result.Errors) != 0 || store.statuses[1].status != "skipped" || !strings.Contains(strings.Join(progress, "\n"), "非 HTTPS 站点") {
		t.Fatalf("expected non-HTTPS site to be skipped with progress, result=%#v statuses=%#v progress=%#v", result, store.statuses, progress)
	}
}

func TestRunAppendsNotificationFailure(t *testing.T) {
	client := notificationFailClient{}
	service := New(&fakeSiteStore{}, app_provider.NewRegistry(), func(certd.Config) CertificateClient { return client })

	result := service.Run(context.Background(), []storeRepo.TargetApp{{ID: 10, AppType: "missing"}}, Config{MachineName: "web-01"})

	if len(result.Errors) != 2 || !strings.Contains(result.Errors[1], "发送 Certd 默认通知失败") {
		t.Fatalf("expected notification failure to be included, got %#v", result)
	}
}

func TestRunStopsImmediatelyWhenContextIsCanceled(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{{ID: 1, PrimaryDomain: "one.example.com", Https: true}}}
	provider := &fakeProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := New(store, app_provider.NewRegistry(provider), fakeClientFactory())

	result := service.Run(ctx, []storeRepo.TargetApp{{ID: 10, AppType: "fake", RootDir: "/srv/fake"}}, Config{})

	if provider.deployments != 0 || provider.restarts != 0 || !result.Canceled || len(result.Errors) != 0 {
		t.Fatalf("expected canceled synchronization without side effects, provider=%#v result=%#v", provider, result)
	}
}

func TestRunConfiguredScansSitesBeforeSynchronizing(t *testing.T) {
	store := &fakeSiteStore{sites: []storeRepo.AppSite{{ID: 1, PrimaryDomain: "example.com", Https: true}}}
	provider := &fakeProvider{scannedSites: []app_provider.Site{{PrimaryDomain: "example.com", Domains: []string{"example.com"}, Https: true}}}
	apps := fakeAppStore{apps: []storeRepo.TargetApp{{ID: 10, AppType: "fake", RootDir: "/srv/fake", Enabled: true}}}
	settings := fakeSettingsStore{value: `{"baseUrl":"http://certd","keyId":"id","keySecret":"secret"}`}
	service := New(store, app_provider.NewRegistry(provider), fakeClientFactory())

	var progress []string
	result := service.RunConfigured(context.Background(), apps, settings, func(message string) {
		progress = append(progress, message)
	})

	if provider.siteScans != 1 || store.syncs != 1 {
		t.Fatalf("expected site scan before sync, siteScans=%d syncs=%d", provider.siteScans, store.syncs)
	}
	if result.Errors != nil {
		t.Fatalf("unexpected result errors: %#v", result.Errors)
	}
	joined := strings.Join(progress, "\n")
	if !strings.Contains(joined, "============ 站点扫描 =============") || !strings.Contains(joined, "============ 证书同步 =============") {
		t.Fatalf("expected stage titles in progress, got %q", joined)
	}
	if !strings.Contains(joined, "禁用 1 个，HTTPS 0 个") {
		t.Fatalf("disabled HTTPS sites must not be counted in scan summary, got %q", joined)
	}
}

func TestCertificateExpiryReturnsUsefulErrors(t *testing.T) {
	if expiry, err := certificateExpiry(""); err != nil || !expiry.IsZero() {
		t.Fatalf("expected empty certificate path to be ignored, expiry=%v err=%v", expiry, err)
	}
	if _, err := certificateExpiryPEM([]byte("not a certificate")); err == nil || !strings.Contains(err.Error(), "格式无效") {
		t.Fatalf("expected invalid PEM error, got %v", err)
	}
	if _, err := certificateExpiryPEM([]byte("-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----")); err == nil || !strings.Contains(err.Error(), "解析本地证书失败") {
		t.Fatalf("expected invalid certificate error, got %v", err)
	}
}

type fakeSiteStore struct {
	sites    []storeRepo.AppSite
	statuses map[uint]syncStatus
	syncs    int
}

type syncStatus struct {
	status string
	detail string
}

func (s *fakeSiteStore) ListEnabledSites(uint) ([]storeRepo.AppSite, error) {
	return s.sites, nil
}

func (s *fakeSiteStore) UpdateSyncStatus(id uint, status, detail string) error {
	if s.statuses == nil {
		s.statuses = make(map[uint]syncStatus)
	}
	s.statuses[id] = syncStatus{status: status, detail: detail}
	return nil
}

func (s *fakeSiteStore) ListSites(uint) ([]storeRepo.AppSite, error) { return s.sites, nil }

func (s *fakeSiteStore) SyncSites(uint, []storeRepo.AppSite) error {
	s.syncs++
	return nil
}

type fakeCertificateClient struct{}

func (fakeCertificateClient) GetCertificate([]string, int64) (certd.Certificate, error) {
	return certd.Certificate{CertificatePEM: "certificate", NotAfter: time.Now().Add(24 * time.Hour)}, nil
}

func (fakeCertificateClient) SendDefaultNotification(string, string) error { return nil }

func fakeClientFactory() ClientFactory {
	return func(certd.Config) CertificateClient { return fakeCertificateClient{} }
}

type certificateResponse struct {
	certificate certd.Certificate
	err         error
}

type sequenceClient struct {
	responses   []certificateResponse
	requests    int
	pipelineIds []int64
}

func (c *sequenceClient) GetCertificate(_ []string, pipelineId int64) (certd.Certificate, error) {
	response := c.responses[c.requests]
	c.requests++
	c.pipelineIds = append(c.pipelineIds, pipelineId)
	return response.certificate, response.err
}

func (c *sequenceClient) SendDefaultNotification(string, string) error { return nil }

type notificationFailClient struct{}

func (notificationFailClient) GetCertificate([]string, int64) (certd.Certificate, error) {
	return certd.Certificate{}, errors.New("unexpected certificate request")
}

func (notificationFailClient) SendDefaultNotification(string, string) error {
	return errors.New("通知服务不可用")
}

type concurrentCertificateClient struct {
	mu      sync.Mutex
	active  int
	max     int
	started chan struct{}
	release chan struct{}
}

func newConcurrentCertificateClient() *concurrentCertificateClient {
	return &concurrentCertificateClient{started: make(chan struct{}, 4), release: make(chan struct{})}
}

func (c *concurrentCertificateClient) GetCertificate([]string, int64) (certd.Certificate, error) {
	c.mu.Lock()
	c.active++
	if c.active > c.max {
		c.max = c.active
	}
	c.mu.Unlock()
	c.started <- struct{}{}
	<-c.release
	c.mu.Lock()
	c.active--
	c.mu.Unlock()
	return certd.Certificate{CertificatePEM: "certificate", NotAfter: time.Now().Add(time.Hour)}, nil
}

func (c *concurrentCertificateClient) SendDefaultNotification(string, string) error { return nil }

func (c *concurrentCertificateClient) maxActiveRequests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.max
}

type fakeProvider struct {
	deployments  int
	restarts     int
	restartErr   error
	deployErrFor map[string]error
	scannedSites []app_provider.Site
	siteScans    int
}

func (p *fakeProvider) Type() string { return "fake" }

func (p *fakeProvider) ScanApps(string, func(app_provider.Progress)) ([]app_provider.App, error) {
	return nil, nil
}

func (p *fakeProvider) ScanSites(app_provider.App) ([]app_provider.Site, error) {
	p.siteScans++
	return p.scannedSites, nil
}

func (p *fakeProvider) DeployCertificate(site app_provider.Site, _ certd.Certificate) error {
	p.deployments++
	return p.deployErrFor[site.PrimaryDomain]
}

func (p *fakeProvider) Restart(app_provider.App) error {
	p.restarts++
	return p.restartErr
}

type fakeInspectableProvider struct {
	fakeProvider
	localExpiry time.Time
}

func (p *fakeInspectableProvider) Type() string { return "inspectable" }

func (p *fakeInspectableProvider) LocalCertificateExpiry(app_provider.Site) (time.Time, error) {
	return p.localExpiry, nil
}

type fakeAppStore struct {
	apps []storeRepo.TargetApp
	err  error
}

func (s fakeAppStore) List() ([]storeRepo.TargetApp, error) { return s.apps, s.err }

type fakeSettingsStore struct {
	value string
	err   error
}

func (s fakeSettingsStore) GetSetting(string) (string, error) { return s.value, s.err }
