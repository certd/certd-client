# certd-client

自动化证书管理系统[Certd](https://github.com/certd/certd) 的官方客户端       
本客户端运行于要部署证书的目标服务器上，承担本地服务器上应用证书的更新检查、拉取和部署工作。
解决某些对于服务器安全性要求较高的场景，不希望暴露ssh端口和密钥的情况。



## 主要功能

1、自动扫描本机上的Nginx、Apache、IIS站点信息（兼容宝塔、1Panel等面板），列出所有的需要部署证书的站点
2、解析站点域名，然后向certd获取证书，如果本地证书有效期低于certd的证书有效期，则更新并部署
3、每天自动运行
4、如果有部署失败的，调用certd接口发送异常通知


## 提供终端UI操作菜单

1、管理Certd授权
2、扫描本机站点 -> 分类表格展示站点列表，类型，配置路径，证书状态【证书已同步，证书申请失败，证书申请中，同步中】，失败原因
3、手动执行部署

> 同时提供cli命令行

CLI 示例：

```bash
# 扫描已登记应用的站点并同步 HTTPS 证书
certd-client sync

# 每天凌晨 02:30 执行一次“站点扫描 + 证书同步”
certd-client start --cron "30 2 * * *"

# 未传 --cron 时，立即执行一次，再按本次启动的小时和分钟每天执行一次
certd-client start
```

```bash
# 查看客户端版本
certd-client version
```

`--cron` 使用五段 Cron 表达式：`分 时 日 月 周`。`start` 启动后会打印启动成功、立即执行一轮任务，并在每轮结束后输出下次执行时间。`sync` 和 `start` 使用与 TUI 相同的 `internal/syncservice` 编排。Linux 上 Nginx 重载会沿用运行进程的 `-p` prefix（无法读取时回退到登记目录），Apache 重载使用应用根目录作为工作目录；目录扫描遇到权限不足会跳过并记录日志。定时任务可通过 `Ctrl+C` 或系统 `SIGTERM` 停止，IIS 仅在 Windows 注册。

## 安装与更新

复制对应系统的命令执行即可：

Linux 和 macOS：

```bash
curl -fsSL https://raw.githubusercontent.com/certd/certd-client/main/scripts/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/certd/certd-client/main/scripts/install.ps1 | iex
```

两个脚本都会提示安装目录，直接回车时安装到当前目录的 `certd-client` 子目录。它们会比较 GitHub 与 AtomGit 最新 Release 下载地址的响应时间，选择更快的源下载当前系统和 CPU 架构对应的包；已安装时直接覆盖二进制完成更新，再启动客户端。

## 版本与发布

版本配置位于 `internal/version/version.go` 的 `Version`，必须符合 Node.js 使用的 [SemVer](https://semver.org/lang/zh-CN/) 格式，例如 `0.1.0`、`1.2.3-rc.1`。终端 UI 标题和 `certd-client version` 都会显示该版本。

本地发布使用 PowerShell：

```powershell
./scripts/release.ps1
```

脚本要求工作区干净，并在修改版本前执行 `go test ./...` 和 `go vet ./...`；任一失败会取消发布。通过检查后，再根据上一个 `v*` 标签后的 Conventional Commits 自动确定版本段：破坏性变更为 major，`feat` 为 minor，`fix` 和 `perf` 为 patch。它会生成或更新 `CHANGELOG.md`，提交版本更新、创建 `vX.Y.Z` 标签并推送到 GitHub。可用 `./scripts/release.ps1 -DryRun` 预览结果，或传 `-Bump major|minor|patch` 覆盖自动判断。

推送版本标签后 GitHub Actions 会运行测试，构建 Windows、Linux 和 macOS 的 amd64/arm64 安装包，并创建 GitHub Release。Release 发布成功后会把 Release 与资产同步到 AtomGit；普通 GitHub push 会同步全部分支和标签到 AtomGit 同名仓库。

GitHub 仓库需要设置 `ATOMGIT_TOKEN` Secret（AtomGit 具备仓库写入和 API 权限的个人令牌）。可选 Variables：`ATOMGIT_REPOSITORY`（默认 `certd/certd-client`）和 `ATOMGIT_API_BASE`（默认 `https://api.atomgit.com/api/v5`）。


## 技术栈

* golang
* sqlite3

## 当前客户端原型

启动 TUI：

```bash
go run ./cmd/certd-client
```

Windows 启动时会请求管理员权限，以读取 IIS 配置并支持后续证书部署；非 Windows 平台不会请求提权。

界面包含“应用扫描”、“扫描站点”、“应用管理”、“Certd接口设置”和“同步证书”五个菜单。选择“应用扫描”后输入根目录，程序会递归识别 Nginx 的 `*/conf/nginx.conf`、`*/sbin/nginx`、`*/bin/nginx` 以及 Apache 的 `*/conf/httpd.conf`、`*/bin/httpd` 等特征，列出安装根目录；IIS 不递归扫描目录，而是执行 `appcmd list site` 检查安装状态并登记 `inetsrv` 目录。使用空格勾选、回车保存到 SQLite 的 `target_app` 表。相同安装路径会原地更新并保留应用 ID。

每次应用扫描开始前，客户端会校验已登记应用的根目录；不存在的目录会标记为禁用，不参与站点扫描。再次扫描到同一路径时会自动恢复启用。

“扫描站点”会扫描所有已登记应用的配置树：Nginx 解析 `server_name`、HTTPS 监听和证书配置；Apache 解析 `VirtualHost`、`ServerName`、`ServerAlias` 与 SSL 指令；IIS 解析 `config/applicationHost.config` 中的站点绑定。结果同步到 SQLite 的 `app_site` 表。站点记录包含主域名、子域名数量、配置文件路径、应用 ID 和 HTTPS 状态。已登记应用会显示在中部区域，操作日志同时写入 `./logs/client.log`，数据库文件默认位于 `./data/certd-client.db`。

应用扫描与站点扫描通过已注册的 `app_provider` 执行。新增应用类型时实现并注册对应 Provider，即可接入同一主流程。

“Certd接口设置”使用 `settings` 表的 `certd` 键保存 baseUrl、keyId、keySecret 和可选的本机名称。选择“同步证书”后，客户端遍历已登记应用的 HTTPS 站点，向 Certd 请求证书时启用自动申请并使用默认模板（`autoApplyTemplateId: 0`），申请中自动重试；当 Certd 证书有效期晚于本地证书时，通过对应 Provider 写入证书和私钥文件，并执行 Nginx 配置重载、Apache graceful 重载或 IIS 绑定刷新使新证书生效。IIS 导入证书时会使用主域名和到期时间设置 FriendlyName，更新全部 HTTPS 绑定，重启 IIS 后检查所有绑定证书是否已生效。同步结束后汇总结果，失败时调用 Certd 默认通知渠道，通知标题附带本机名称；本机名称为空时使用主机名和 IP。

已登记应用表格显示站点总数、HTTPS 站点数、已同步数量和异常数量；非 HTTPS 站点不会参与证书同步。

“应用管理”可查看已登记应用的站点列表，也支持删除应用；在应用管理中按回车查看站点，按 `d` 删除当前应用并按 `y` 确认。站点列表可通过上下键选择，按空格启用或禁用站点；禁用站点不参与证书同步，重新扫描时会保留其禁用状态。删除确认后会同时删除该应用关联的全部站点记录。

执行日志支持 `PageUp` / `PageDown` 分页浏览，新日志产生时自动回到最新页。

扫描在后台执行。扫描时间超过 10 秒时，执行日志每 10 秒记录一次已扫描目录数和当前待扫描目录数，界面保持可响应。
