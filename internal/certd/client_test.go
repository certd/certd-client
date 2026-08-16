package certd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetCertificateExposesDocumentedAPIErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":20013,"message":"证书正在申请中，请稍后重新获取","data":{"pipelineId":101,"certId":202}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, KeyId: "key-id", KeySecret: "key-secret"})
	_, err := client.GetCertificate([]string{"example.com"}, 0)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 20013 || apiErr.PipelineId != 101 || apiErr.CertId != 202 {
		t.Fatalf("expected API error code 20013, got %#v", err)
	}
}

func TestTokenUsesDocumentedSignature(t *testing.T) {
	client := NewClient(Config{BaseURL: "https://certd.example.com", KeyId: "key-id", KeySecret: "key-secret"})
	client.now = func() time.Time { return time.Unix(1700000000, 0) }

	token, err := client.token()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected token: %q", token)
	}
	content, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["keyId"] != "key-id" || payload["t"] != float64(1700000000) || payload["encrypt"] != false || payload["signType"] != "md5" {
		t.Fatalf("unexpected token payload: %#v", payload)
	}
	signature, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(signature) != client.sign(string(content)) {
		t.Fatalf("unexpected token signature: %q", signature)
	}
}

func TestGetCertificateSendsAutoApplyRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/cert/get" || request.Header.Get("x-certd-token") == "" {
			t.Fatalf("unexpected Certd request: %s %s headers=%v", request.Method, request.URL.Path, request.Header)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload certificateRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Domains != "example.com,www.example.com" || !payload.AutoApply || payload.AutoApplyTemplateId != 0 {
			t.Fatalf("unexpected request body: %#v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"crt":"certificate","key":"private-key","pfx":"cGZ4","detail":{"notAfter":1800000000000}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, KeyId: "key-id", KeySecret: "key-secret"})
	certificate, err := client.GetCertificate([]string{"example.com", "www.example.com"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if certificate.CertificatePEM != "certificate" || certificate.PrivateKeyPEM != "private-key" || certificate.PfxBase64 != "cGZ4" || !certificate.NotAfter.Equal(time.UnixMilli(1800000000000)) {
		t.Fatalf("unexpected certificate: %#v", certificate)
	}
}

func TestGetCertificateSendsPipelineIdWhenPolling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload certificateRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.PipelineId != 101 || !payload.AutoApply || payload.AutoApplyTemplateId != 0 {
			t.Fatalf("unexpected polling request: %#v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"crt":"certificate","detail":{"notAfter":1800000000000}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, KeyId: "key-id", KeySecret: "key-secret"})
	if _, err := client.GetCertificate([]string{"example.com"}, 101); err != nil {
		t.Fatal(err)
	}
}

func TestSendDefaultNotification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/notification/send" {
			t.Fatalf("unexpected notification request: %s %s", request.Method, request.URL.Path)
		}
		var payload notificationRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Title != "证书同步失败" || payload.Content != "example.com：部署失败" || payload.NotificationType != "certDeployError" {
			t.Fatalf("unexpected notification: %#v", payload)
		}
		_, _ = writer.Write([]byte(`{"code":0,"data":{"success":true}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, KeyId: "key-id", KeySecret: "key-secret"})
	if err := client.SendDefaultNotification("证书同步失败", "example.com：部署失败"); err != nil {
		t.Fatal(err)
	}
}
