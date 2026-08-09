package app_provider

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/certd/certd-client/internal/certd"
)

// WriteCertificateFiles writes certificate artifacts to the paths discovered for a site.
func WriteCertificateFiles(site Site, certificate certd.Certificate) error {
	if certificate.CertificatePEM == "" && certificate.PfxBase64 == "" {
		return fmt.Errorf("Certd 未返回证书内容")
	}
	wroteCertificate := false
	if certificate.CertificatePEM != "" {
		if err := writeFile(site.CertificatePath, []byte(certificate.CertificatePEM)); err != nil {
			return fmt.Errorf("写入证书文件失败: %w", err)
		}
		wroteCertificate = true
	}
	if certificate.PrivateKeyPEM != "" {
		if err := writeFile(site.PrivateKeyPath, []byte(certificate.PrivateKeyPEM)); err != nil {
			return fmt.Errorf("写入私钥文件失败: %w", err)
		}
	}
	if certificate.PfxBase64 != "" && !wroteCertificate && site.CertificatePath != "" {
		pfx, err := base64.StdEncoding.DecodeString(certificate.PfxBase64)
		if err != nil {
			return fmt.Errorf("解析 PFX 证书失败: %w", err)
		}
		if err := writeFile(site.CertificatePath, pfx); err != nil {
			return fmt.Errorf("写入 PFX 证书失败: %w", err)
		}
	}
	return nil
}

func writeFile(path string, content []byte) error {
	if path == "" {
		return fmt.Errorf("证书部署路径为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}
