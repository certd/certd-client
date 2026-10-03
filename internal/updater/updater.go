package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repository = "certd/certd-client"

type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

type Channel struct {
	Name, Version, URL string
	Latency            time.Duration
}
type Result struct {
	Version  string
	Channels []Channel
	Fastest  Channel
}

func AssetName(goos, arch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("certd-client-%s-%s%s", goos, arch, ext)
}

// CompareVersions returns -1, 0, or 1 using SemVer precedence.
func CompareVersions(current, latest string) (int, error) {
	parse := func(v string) ([3]uint64, string, error) {
		var nums [3]uint64
		v = strings.TrimPrefix(v, "v")
		core := strings.SplitN(v, "-", 2)
		parts := strings.Split(core[0], ".")
		if len(parts) != 3 {
			return nums, "", fmt.Errorf("invalid semantic version %q", v)
		}
		for i, part := range parts {
			parsed, err := strconv.ParseUint(part, 10, 64)
			if err != nil || part == "" {
				return nums, "", fmt.Errorf("invalid semantic version %q", v)
			}
			nums[i] = parsed
		}
		pre := ""
		if len(core) == 2 {
			pre = core[1]
		}
		return nums, pre, nil
	}
	a, ap, err := parse(current)
	if err != nil {
		return 0, err
	}
	b, bp, err := parse(latest)
	if err != nil {
		return 0, err
	}
	for i := range a {
		if a[i] < b[i] {
			return -1, nil
		}
		if a[i] > b[i] {
			return 1, nil
		}
	}
	if ap == bp {
		return 0, nil
	}
	if ap == "" {
		return 1, nil
	}
	if bp == "" {
		return -1, nil
	}
	return strings.Compare(ap, bp), nil
}

func Check(ctx context.Context, client *http.Client, goos, arch string) (Result, error) {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	urls := []struct{ name, url string }{{"AtomGit", "https://api.atomgit.com/api/v5/repos/" + repository + "/releases/latest"}, {"GitHub", "https://api.github.com/repos/" + repository + "/releases/latest"}}
	type response struct {
		channel Channel
		release Release
		err     error
	}
	ch := make(chan response, len(urls))
	for _, endpoint := range urls {
		go func(name, url string) {
			started := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				ch <- response{err: err}
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				ch <- response{err: err}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				ch <- response{err: fmt.Errorf("%s 返回 HTTP %d", name, resp.StatusCode)}
				return
			}
			var rel Release
			err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel)
			if err != nil {
				ch <- response{err: err}
				return
			}
			item := Channel{Name: name, Version: strings.TrimPrefix(rel.TagName, "v"), Latency: time.Since(started)}
			for _, asset := range rel.Assets {
				if asset.Name == AssetName(goos, arch) {
					item.URL = asset.URL
					break
				}
			}
			if item.URL == "" && rel.TagName != "" {
				item.URL = fmt.Sprintf("https://%s/%s/releases/download/%s/%s", map[string]string{"AtomGit": "atomgit.com", "GitHub": "github.com"}[name], repository, rel.TagName, AssetName(goos, arch))
			}
			ch <- response{channel: item, release: rel}
		}(endpoint.name, endpoint.url)
	}
	result := Result{}
	for range urls {
		item := <-ch
		if item.err == nil && item.channel.Version != "" {
			result.Channels = append(result.Channels, item.channel)
		}
	}
	if len(result.Channels) == 0 {
		return result, fmt.Errorf("无法从 AtomGit 或 GitHub 获取版本信息")
	}
	result.Version = result.Channels[0].Version
	for _, candidate := range result.Channels[1:] {
		cmp, err := CompareVersions(result.Version, candidate.Version)
		if err == nil && cmp < 0 {
			result.Version = candidate.Version
		}
	}
	for _, candidate := range result.Channels {
		cmp, err := CompareVersions(candidate.Version, result.Version)
		if err == nil && cmp == 0 && (result.Fastest.Name == "" || candidate.Latency < result.Fastest.Latency) {
			result.Fastest = candidate
		}
	}
	return result, nil
}

func Download(ctx context.Context, client *http.Client, channel Channel) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channel.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "certd-client-update-*")
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(f, io.LimitReader(resp.Body, 512<<20)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err = f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	if err = validateArchive(f.Name(), runtime.GOOS); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func validateArchive(path, goos string) error {
	found := false
	if err := walkExecutable(path, goos, func(io.Reader, os.FileMode) error { found = true; return nil }); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("归档中没有 %s", executableName(goos))
	}
	return nil
}

func walkExecutable(path, goos string, visit func(io.Reader, os.FileMode) error) error {
	name := executableName(goos)
	if goos == "windows" {
		z, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			if filepath.Base(f.Name) == name {
				r, err := f.Open()
				if err != nil {
					return err
				}
				err = visit(r, 0755)
				r.Close()
				return err
			}
		}
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) == name {
			return visit(tr, os.FileMode(h.Mode))
		}
	}
}

func executableName(goos string) string {
	if goos == "windows" {
		return "certd-client.exe"
	}
	return "certd-client"
}

// InstallArchive extracts a validated package next to the running executable and replaces it.
func InstallArchive(archive, executable string) error {
	if err := validateArchive(archive, runtime.GOOS); err != nil {
		return err
	}
	dir := filepath.Dir(executable)
	temp, err := os.MkdirTemp(dir, ".certd-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	err = walkExecutable(archive, runtime.GOOS, func(src io.Reader, mode os.FileMode) error {
		out, err := os.OpenFile(filepath.Join(temp, executableName(runtime.GOOS)), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, src)
		return err
	})
	if err != nil {
		return err
	}
	newBinary := filepath.Join(temp, executableName(runtime.GOOS))
	if _, err = os.Stat(newBinary); err != nil {
		return fmt.Errorf("安装包缺少可执行文件：%w", err)
	}
	if runtime.GOOS == "windows" {
		backup := executable + ".old"
		_ = os.Remove(backup)
		var removeErr error
		for attempt := 0; attempt < 20; attempt++ {
			removeErr = os.Rename(executable, backup)
			if removeErr == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if removeErr != nil {
			return fmt.Errorf("移动旧可执行文件 %s 失败：%w", executable, removeErr)
		}
		defer os.Remove(backup)
		var renameErr error
		for attempt := 0; attempt < 20; attempt++ {
			renameErr = os.Rename(newBinary, executable)
			if renameErr == nil {
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		_ = os.Rename(backup, executable)
		return fmt.Errorf("重命名更新文件 %s -> %s 失败：%w", newBinary, executable, renameErr)
	}
	var renameErr error
	for attempt := 0; attempt < 20; attempt++ {
		renameErr = os.Rename(newBinary, executable)
		if renameErr == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if renameErr != nil {
		return fmt.Errorf("重命名更新文件 %s -> %s 失败：%w", newBinary, executable, renameErr)
	}
	return nil
}
