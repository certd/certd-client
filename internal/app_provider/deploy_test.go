package app_provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/certd/certd-client/internal/certd"
)

func TestWriteCertificateFilesWritesPEMAndPrivateKey(t *testing.T) {
	root := t.TempDir()
	site := Site{
		CertificatePath: filepath.Join(root, "cert", "fullchain.pem"),
		PrivateKeyPath:  filepath.Join(root, "key", "private.key"),
	}
	certificate := certd.Certificate{CertificatePEM: "CERTIFICATE", PrivateKeyPEM: "PRIVATE KEY"}

	if err := WriteCertificateFiles(site, certificate); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]string{
		site.CertificatePath: certificate.CertificatePEM,
		site.PrivateKeyPath:  certificate.PrivateKeyPEM,
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != expected {
			t.Fatalf("unexpected content for %s: %q", path, content)
		}
	}
}
