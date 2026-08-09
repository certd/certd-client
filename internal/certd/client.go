// Package certd implements the Certd OpenAPI protocol used by the client.
package certd

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL   string
	KeyId     string
	KeySecret string
}

const (
	ErrCodeOpenKeyError         = 20000
	ErrCodeOpenKeySignError     = 20001
	ErrCodeOpenKeyExpiresError  = 20002
	ErrCodeOpenKeySignTypeError = 20003
	ErrCodeOpenParamError       = 20010
	ErrCodeOpenCertNotFound     = 20011
	ErrCodeOpenCertNotReady     = 20012
	ErrCodeOpenCertApplying     = 20013
	ErrCodeOpenDomainNoVerifier = 20014
	ErrCodeOpenPipelineError    = 20015
	ErrCodeOpenEmailNotFound    = 20021
)

type APIError struct {
	Code    int
	Message string
}

func (err *APIError) Error() string {
	return fmt.Sprintf("Certd 接口错误：%s", err.Message)
}

type Certificate struct {
	CertificatePEM string
	PrivateKeyPEM  string
	PfxBase64      string
	NotAfter       time.Time
}

type Client struct {
	config     Config
	httpClient *http.Client
	now        func() time.Time
}

type certificateRequest struct {
	Domains             string `json:"domains"`
	AutoApply           bool   `json:"autoApply"`
	AutoApplyTemplateId int    `json:"autoApplyTemplateId"`
}

type notificationRequest struct {
	Title            string `json:"title"`
	Content          string `json:"content"`
	NotificationType string `json:"notificationType"`
}

type certificateResponse struct {
	Crt    string `json:"crt"`
	Key    string `json:"key"`
	Pfx    string `json:"pfx"`
	Detail struct {
		NotAfter int64 `json:"notAfter"`
	} `json:"detail"`
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func NewClient(config Config) *Client {
	return &Client{
		config: Config{
			BaseURL:   strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
			KeyId:     strings.TrimSpace(config.KeyId),
			KeySecret: strings.TrimSpace(config.KeySecret),
		},
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

func (client *Client) GetCertificate(domains []string) (Certificate, error) {
	domains = client.normalizedDomains(domains)
	if len(domains) == 0 {
		return Certificate{}, fmt.Errorf("证书域名不能为空")
	}
	var response certificateResponse
	if err := client.post("/api/v1/cert/get", certificateRequest{Domains: strings.Join(domains, ","), AutoApply: true, AutoApplyTemplateId: 0}, &response); err != nil {
		return Certificate{}, err
	}
	certificate := Certificate{
		CertificatePEM: response.Crt,
		PrivateKeyPEM:  response.Key,
		PfxBase64:      response.Pfx,
	}
	if response.Detail.NotAfter > 0 {
		certificate.NotAfter = time.UnixMilli(response.Detail.NotAfter)
		if response.Detail.NotAfter < 100000000000 {
			certificate.NotAfter = time.Unix(response.Detail.NotAfter, 0)
		}
	}
	return certificate, nil
}

func (client *Client) SendDefaultNotification(title, content string) error {
	return client.post("/api/v1/notification/send", notificationRequest{
		Title:            title,
		Content:          content,
		NotificationType: "certDeployError",
	}, nil)
}

func (client *Client) post(path string, payload any, result any) error {
	if client.config.BaseURL == "" || client.config.KeyId == "" || client.config.KeySecret == "" {
		return fmt.Errorf("Certd 接口设置不完整")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码 Certd 请求失败: %w", err)
	}
	token, err := client.token()
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, client.config.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建 Certd 请求失败: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-certd-token", token)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("调用 Certd 接口失败: %w", err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("读取 Certd 响应失败: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Certd 接口返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(content)))
	}
	var envelope apiResponse
	if err := json.Unmarshal(content, &envelope); err != nil {
		return fmt.Errorf("解析 Certd 响应失败: %w", err)
	}
	if envelope.Code != 0 {
		return &APIError{Code: envelope.Code, Message: envelope.Message}
	}
	if result != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("解析 Certd 数据失败: %w", err)
		}
	}
	return nil
}

func (client *Client) token() (string, error) {
	content, err := json.Marshal(struct {
		KeyId    string `json:"keyId"`
		Time     int64  `json:"t"`
		Encrypt  bool   `json:"encrypt"`
		SignType string `json:"signType"`
	}{
		KeyId:    client.config.KeyId,
		Time:     client.now().Unix(),
		Encrypt:  false,
		SignType: "md5",
	})
	if err != nil {
		return "", fmt.Errorf("生成 Certd 授权内容失败: %w", err)
	}
	return base64.StdEncoding.EncodeToString(content) + "." + base64.StdEncoding.EncodeToString([]byte(client.sign(string(content)))), nil
}

func (client *Client) sign(content string) string {
	sum := md5.Sum([]byte(content + client.config.KeySecret))
	return hex.EncodeToString(sum[:])
}

func (client *Client) normalizedDomains(domains []string) []string {
	result := make([]string, 0, len(domains))
	known := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if _, exists := known[domain]; exists {
			continue
		}
		known[domain] = struct{}{}
		result = append(result, domain)
	}
	return result
}
