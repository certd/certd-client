package app_provider

import (
	"bytes"
	"runtime"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// DecodeCommandOutput 解码 Windows 命令输出，避免中文提示写入日志和错误信息时出现乱码。
// Windows 命令的输出编码取决于进程控制台代码页：中文控制台（代码页 936）输出 GBK，
// 终端设置 UTF-8 代码页时输出 UTF-8。因此先按 UTF-8 校验，无法作为 UTF-8 解码时再按 GBK 解码。
// 非 Windows 平台按原样返回。
func DecodeCommandOutput(output []byte) string {
	trimmed := bytes.TrimSpace(output)
	if runtime.GOOS != "windows" || utf8.Valid(trimmed) {
		return string(trimmed)
	}
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(trimmed)
	if err != nil {
		return string(trimmed)
	}
	return string(decoded)
}
