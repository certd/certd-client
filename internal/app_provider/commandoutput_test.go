package app_provider

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestDecodeCommandOutputDecodesWindowsGbkText(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 命令输出使用 GBK 解码")
	}
	gbkOutput, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("值不在预期的范围内。"))
	if err != nil {
		t.Fatal(err)
	}

	if decoded := DecodeCommandOutput(gbkOutput); decoded != "值不在预期的范围内。" {
		t.Fatalf("expected decoded GBK output, got %q", decoded)
	}
}

// 控制台已设置为 UTF-8 代码页时命令输出本身就是 UTF-8，不能再按 GBK 二次解码。
func TestDecodeCommandOutputKeepsUtf8Chinese(t *testing.T) {
	if decoded := DecodeCommandOutput([]byte("HTTPS 绑定证书更新失败：值不在预期的范围内。\r\n")); decoded != "HTTPS 绑定证书更新失败：值不在预期的范围内。" {
		t.Fatalf("expected untouched utf-8 output, got %q", decoded)
	}
}

func TestDecodeCommandOutputTrimsAsciiOutput(t *testing.T) {
	if decoded := DecodeCommandOutput([]byte("  ABC123\r\n")); decoded != "ABC123" {
		t.Fatalf("expected trimmed ascii output, got %q", decoded)
	}
}

// 真实 PowerShell 的中文错误输出无论是 GBK 还是 UTF-8，解码后都必须可读。
func TestDecodeCommandOutputKeepsRealPowerShellChineseReadable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 需要调用 powershell.exe")
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("未找到 powershell.exe")
	}
	message := "HTTPS 绑定证书更新失败：值不在预期的范围内。"
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "throw '"+message+"'").CombinedOutput()
	if err == nil {
		t.Fatal("expected PowerShell failure")
	}

	decoded := DecodeCommandOutput(output)
	if !strings.Contains(decoded, message) {
		t.Fatalf("expected readable Chinese output, got %q", decoded)
	}
}
