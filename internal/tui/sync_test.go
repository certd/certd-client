package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/certd/certd-client/internal/certd"
	storeRepo "github.com/certd/certd-client/internal/store/repo"
)

func TestSyncLabelsIncludeApplicationTypeAndID(t *testing.T) {
	app := storeRepo.TargetApp{ID: 7, AppType: "iis", RootDir: `C:\\Windows\\System32\\inetsrv`}
	label := syncAppLabel(app)
	if !strings.Contains(label, "iis") || strings.Contains(label, "7") {
		t.Fatalf("expected sync app label to include type without application ID, got %q", label)
	}

	siteLabel := syncSiteLabel(app, storeRepo.AppSite{ID: 42, PrimaryDomain: "example.com"})
	if !strings.Contains(siteLabel, "iis") || !strings.Contains(siteLabel, "42") || !strings.Contains(siteLabel, "example.com") || strings.Contains(siteLabel, "7") {
		t.Fatalf("expected sync site label to include app type, site ID and domain, got %q", siteLabel)
	}
}

func TestFetchCertificateRetriesPendingApplication(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		writer.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			_, _ = writer.Write([]byte(`{"code":20013,"message":"证书正在申请中，请稍后重新获取"}`))
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"crt":    "certificate",
				"detail": map[string]any{"notAfter": 1800000000000},
			},
		})
	}))
	defer server.Close()

	model := Model{syncAttempts: 2}
	client := certd.NewClient(certd.Config{BaseURL: server.URL, KeyId: "id", KeySecret: "secret"})
	certificate, err := model.fetchCertificate(client, "example.com", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || certificate.CertificatePEM != "certificate" {
		t.Fatalf("expected pending request retry, attempts=%d certificate=%#v", attempts, certificate)
	}
}

func TestFetchCertificateStopsWhenPendingApplicationChangesToOtherError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		writer.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			_, _ = writer.Write([]byte(`{"code":20013,"message":"证书正在申请中，请稍后重新获取"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"code":20015,"message":"流水线执行异常，请稍后重试"}`))
	}))
	defer server.Close()

	model := Model{syncAttempts: 60, syncInterval: 0, otherRetryInterval: 0, otherRetryAttempts: 3}
	client := certd.NewClient(certd.Config{BaseURL: server.URL, KeyId: "id", KeySecret: "secret"})
	_, err := model.fetchCertificate(client, "example.com", "example.com")
	if attempts != 2 || err == nil || !strings.Contains(err.Error(), "流水线执行异常") {
		t.Fatalf("expected immediate failure after pending application changed, attempts=%d err=%v", attempts, err)
	}
}

func TestFetchCertificateRetriesOtherAPIErrorWithInjectedNoWait(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":20001,"message":"ApiToken签名错误"}`))
	}))
	defer server.Close()

	progress := make(chan string, 4)
	model := Model{syncAttempts: 60, otherRetryAttempts: 3, syncInterval: 0, syncProgressCh: progress}
	client := certd.NewClient(certd.Config{BaseURL: server.URL, KeyId: "id", KeySecret: "secret"})
	_, err := model.fetchCertificate(client, "example.com", "example.com")
	if attempts != 3 || err == nil || !strings.Contains(err.Error(), "ApiToken签名错误") {
		t.Fatalf("expected retry for non-pending failure, attempts=%d err=%v", attempts, err)
	}
	var messages []string
	for len(progress) > 0 {
		messages = append(messages, <-progress)
	}
	if len(messages) == 0 || !strings.Contains(strings.Join(messages, "\n"), "ApiToken签名错误") {
		t.Fatalf("expected concrete Certd error in progress logs, got %#v", messages)
	}
}
