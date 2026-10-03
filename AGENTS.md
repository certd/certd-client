# 项目协作规范

## 语言

- 与用户沟通、代码注释、文档、日志和面向用户的界面文案，尽量使用中文。
- 技术专有名词、代码标识符、第三方库名称和命令可保留英文，以保证准确性与可检索性。
- 供 `irm ... | iex` 直接执行的 PowerShell 安装脚本必须保存为 UTF-8 无 BOM；BOM 会被管道执行当作脚本首字符，导致 `CmdletBinding` 解析失败。Windows 安装脚本的 CPU 架构检测应优先使用 `PROCESSOR_ARCHITEW6432`、`PROCESSOR_ARCHITECTURE` 等环境变量，不能依赖旧版 .NET 中可能为空的 `RuntimeInformation.OSArchitecture`。当前目录名已是 `certd-client` 时默认安装目录必须使用当前目录，不能重复追加。AtomGit 附件地址应从最新 Release API 的 `browser_download_url` 获取，不能套用 GitLab 风格的 permalink；候选下载源即使返回 HTTP 200 也必须先校验 ZIP 或 `tar.gz` 内容，校验失败应记录原因并自动尝试下一源。对此保留自动化检查。

## TDD 开发

- 新功能、缺陷修复和重构遵循测试驱动开发：先编写或更新测试，再运行测试确认 RED，最后实现最小改动并确认 GREEN。
- 测试应覆盖正常路径、边界条件和明确的错误处理；优先为扫描、数据库和交互状态等核心逻辑添加自动化测试。
- 交付前至少执行相关测试；对 Go 代码默认执行 `go test ./...` 和 `go vet ./...`，无法执行时应说明原因。

## 产品目标

- 开始工作前阅读 `readme.md`，将其描述的 Certd 客户端目标作为设计和实现的边界。
- 当前重点是本机应用与站点扫描、证书部署支持、终端 UI 和 CLI；实现应兼容本地服务器及常见面板环境，避免偏离这些目标的无关改造。
- 应用类型的发现和站点解析必须通过已注册的 `app_provider` 实现；新增应用类型时提供对应 Provider，而不是在 TUI 主流程中增加类型分支。
- Provider 自行维护其应用特征识别和配置解析；可复用的目录遍历、进度统计放在 `internal/app_provider`，不创建独立的 `internal/scanner` 层。
- Provider 的类型专属行为应定义为具体 Provider 结构体的方法；包级仅保留构造函数等不依赖实例的入口。
- Linux/Windows 目录扫描遇到权限不足或枚举后已消失的瞬时目录（例如 `/proc/<pid>`）时应跳过该目录并记录可读的提示，不应因单个受限目录中断整个扫描；根目录本身不可读时仍应返回明确错误。
- 应用发现遍历必须跳过 `docker/overlay2` 及其子目录，避免将容器文件系统层误识别为宿主机应用安装目录。
- IIS Provider 不递归扫描用户输入目录；通过执行 `appcmd list site` 确认 IIS 安装，并以 `inetsrv` 为应用根目录，站点配置从 `config/applicationHost.config` 解析。
- IIS 证书部署不写入 PEM 文件路径；应将 Certd 返回的 PFX 通过 PowerShell 导入 `LocalMachine\My` 证书存储，FriendlyName 使用主域名和证书到期时间，再用 `WebAdministration` 按 IIS 站点名称更新全部 HTTPS 绑定，并验证全部绑定使用的新证书有效期。
- 更新 IIS HTTPS 绑定时必须给 `AddSslCertificate` 传证书指纹的十六进制字符串（`(Get-Item Cert:\LocalMachine\My\<指纹>).Thumbprint`），不能传 `GetCertHash()` 得到的 `byte[]`：`WebAdministration` 绑定对象暴露的是 IIS 原生配置方法，其 `certificateHash` 参数在 `inetsrv\config\schema\rscaext.xml` 中声明为 `string`，传 `byte[]` 会让原生方法统一报“值不在预期的范围内”；.NET 的 `Binding.AddSslCertificate(Byte[], String)` 是 internal 方法，PowerShell 无法调用，其实现同样是先转成十六进制字符串再调用同一个原生方法。绑定脚本必须逐个绑定 `try/catch` 汇总失败原因（含 `bindingInformation`）并在任一失败时 `throw`，不能使用裸管道，否则 `AddSslCertificate` 的非终止错误会让脚本以 0 退出，出现绑定未更新却报告部署成功的情况；更新后还必须重新 `Get-WebBinding` 回读绑定的 `CertificateHash` 并与目标指纹比较，不一致时计入失败，保证“报告成功即绑定已生效”。失败记录还必须带上绑定的 `sslFlags`、旧证书状态（`CertificateHash`/`CertificateStoreName` 能否在证书存储中解析）和新证书 `HasPrivateKey`，便于按绑定定位原因。旧证书已丢失或读取失败时必须先 `RemoveSslCertificate()` 清除残留 SSL 绑定再重新 `AddSslCertificate`，这类绑定本身已无法提供 HTTPS，残留的失效哈希会让重新绑定持续失败。绑定脚本这类多段拼接的 PowerShell 必须用 PowerShell 解析器做语法校验（平台测试 `TestDeployCertificateBindingScriptIsValidPowerShell`），避免语法错误只在用户机器上暴露。
- 执行外部命令失败时，返回的错误与日志必须带上完整标准输出和标准错误（含退出码），不能只返回 `exit status 1` 之类的泛化文本；Windows 上无法本地复现的部署问题尤其依赖该输出定位。
- Windows 上所有 Provider 执行外部命令后，必须用 `internal/app_provider` 的 `DecodeCommandOutput` 解码输出再写入日志和错误信息；Windows 命令输出的编码跟随控制台代码页（中文控制台为 GBK，UTF-8 代码页为 UTF-8），该函数先按 UTF-8 校验、校验失败再按 GBK 解码，既不能无条件按 GBK 解码（UTF-8 输出会变成乱码），也不能直接 `string(output)`（中文控制台会写出乱码）；非 Windows 平台保持原样。
- Windows 上的 Apache Provider 应先按可执行文件路径定位对应服务；找到服务时使用 `net stop <服务名>`、`net start <服务名>` 重启，兼容宝塔注册的 `apache` 服务；未找到服务时才使用带 `-f` 配置文件路径的 `httpd -k graceful` 重载。
- Nginx Provider 解析配置中相对证书路径和执行重载时必须使用同一有效 prefix：优先读取运行中同一 `nginx.exe` 命令行的绝对 `-p`，无法读取或未设置时使用登记的应用根目录；重载须将进程工作目录设为该 prefix，并传入 `-p <prefix>` 与相对 prefix 的 `-c <nginx.conf>`，不能依赖客户端当前工作目录。
- Nginx 站点扫描必须从主 `nginx.conf` 递归解析 `include` 指令，支持通配符和绝对路径，以覆盖面板位于应用根目录之外的虚拟主机目录；已包含的配置文件按规范路径去重后再解析。
- 证书同步由已注册 Provider 的部署能力执行，TUI 不按应用类型分支写入证书文件；非 HTTPS 站点不参与同步。Certd 返回申请中或暂未返回证书时应重试，只有 Certd 证书有效期晚于本地证书才部署。证书部署成功后，Nginx、Apache 和 IIS Provider 必须分别执行配置重载、服务重启或绑定刷新使证书生效；需要重启才能生效的 Provider 应在部署后重启并验证证书，验证失败计入同步异常。同步失败需汇总原因并调用 Certd 默认通知渠道，标题使用 `证书同步失败【数量：N】（本机名称） 【来自CertdClient】` 格式；未填写本机名称时使用主机名和 IPv4。
- 证书请求阶段最多同时处理 3 个站点；并发仅限 Certd 证书获取和申请轮询，证书写入、状态更新、应用重启及部署后验证必须保持串行，避免 Provider 和数据库状态竞争。
- 同步中的请求、等待、重试、失败和完成日志必须包含应用类型、站点 ID 与域名，确保并行处理时能定位当前站点。
- 证书同步编排统一放在 `internal/syncservice`，供 TUI、CLI 和定时任务调用；TUI 仅负责读取界面设置、转发进度和展示结果。一个应用的多个站点完成证书文件写入后最多重启一次；重启失败须将本批已写入站点全部标记失败，单个站点写入失败不得阻断同一应用其余站点的部署和重启。
- CLI 的 `sync` 与 `start` 每一轮必须先执行已登记应用的站点扫描，再执行证书同步；定时任务使用 `start --cron` 的五段 Cron 表达式，启动成功后立即执行一轮，并在每轮结束后打印下次执行时间；未指定时后续按进程启动的小时和分钟每天运行一次。定时任务与 TUI/CLI 共用同一编排服务，不在入口重复实现业务流程。同步任务必须接受 `context.Context`，TUI 按 `Esc` 取消时停止后续请求、部署、重启和失败通知；定时任务响应 `Ctrl+C` 与 `SIGTERM`。
- 定时同步计划统一保存到 `settings` 表（`key = schedule`，含 `cron` 与 `enabled`）；解析、校验、下次执行推导与循环执行统一由 `internal/schedule` 提供，供 TUI、CLI `start` 与系统服务共用，不在各入口重复实现定时逻辑。
- 客户端通过 `internal/service` 封装为系统服务：`certd-client service <install|uninstall|start|stop|ensure|run>` 由 `service.Classify` 分派，Windows 使用 `kardianos/service` 注册为服务、Linux 生成 systemd 单元；`ensure` 复用 `EnsureRunning` 的状态判断，一键确保服务已安装并运行。服务进程工作目录为可执行文件所在目录，读写该目录下的 `./data` 与 `./logs`，并读取 `schedule` 设置按计划执行站点扫描与证书同步；`enabled` 为否时仅心跳上报，不执行同步。无参数直接运行可执行文件仍进入交互式 TUI。
- `service` 子命令中除 `run` 外，Windows 必须先请求 UAC 提权再执行安装、启停等操作；`service run` 由系统服务控制器以 SYSTEM 启动、天然已提权，必须跳过提权流程。Linux 安装 systemd 服务写入 `/etc/systemd/system` 需要 root：非 root 时不得直接调用 `kardianos.Install`（会 `permission denied`），应仅将需要提权的服务安装/启动以 `sudo <可执行文件> service ensure` 作为子进程执行，启用定时计划等只写用户自身数据库的动作仍由父进程以当前用户完成，保持数据库归属一致。
- 检测到新版本时只提示（顶部菜单“更新版本”显示新版本号并闪烁），不得自动下载、替换可执行文件或重启，避免影响正在运行的同步任务；更新由用户通过安装脚本或发布页手动完成。
- CLI 批处理任务的阶段切换必须输出清晰的分隔标题（例如站点扫描、证书同步），每轮结束必须输出成功、跳过、失败数量的执行总结；失败明细继续逐行保留，便于脚本日志排查。
- CLI 批处理模式的启动、进度、总结、错误和下次执行时间必须同时写入标准输出与 `./logs/` 日志文件，不能只写文件导致交互终端无反馈。
- Certd 请求失败的执行日志必须包含接口返回的具体 `message`，不能只显示“请求失败”等泛化文本；申请中等待日志同样应保留具体原因。
- 请求 Certd 证书接口时必须发送 `autoApply: true` 和 `autoApplyTemplateId: 0`，使用 Certd 默认申请模板。
- Certd 开放接口错误码：`20000` ApiToken 错误、`20001` ApiToken 签名错误、`20002` ApiToken 时间戳错误、`20003` 不支持的签名类型、`20010` 请求参数错误、`20011` 证书不存在、`20012` 证书还未生成、`20013` 证书正在申请中、`20014` 域名校验方式未配置、`20015` 流水线执行异常、`20021` 用户邮箱未配置。只有连续返回 `20013` 时使用后台长轮询，不得立即报同步失败，轮询间隔为 10 秒；轮询过程中一旦返回其他错误，必须立即失败，不得继续长轮询。最长等待时长由 Certd 接口设置中的分钟数决定，默认 10 分钟。首次或后续发生的其他接口错误最多重试 3 次，重试间隔不得少于 6 秒。
- 客户端版本统一由 `internal/version.Version` 管理，必须符合 Node.js SemVer；发布标签使用 `vX.Y.Z`，版本号按 Conventional Commits 自动递增，破坏性变更为 major、`feat` 为 minor、`fix` 与 `perf` 为 patch；未出现上述类型时默认升 patch。CHANGELOG 仅记录 `feat`、`fix`、`perf` 类型的提交（包含 scope 与 `!` 标记）。`scripts/release.ps1` 必须在修改版本、提交、打标签和推送之前执行本地 `go test ./...` 与 `go vet ./...`，任一失败即取消发布；首次发布尚无 `v*` 标签时，CHANGELOG 必须包含完整 Git 历史，后续发布仅记录上一版本标签后的提交。发布脚本生成或改写文本文件时必须使用 UTF-8 无 BOM，保证中文 changelog 在跨平台工具中可读；读取 Git 中文历史须显式按 UTF-8 解码，并将标准输出与标准错误分开处理。
- GitHub 发布流程需先完成跨平台构建并创建 GitHub Release；Release 正文必须提取 `CHANGELOG.md` 中当前版本段的提交条目，不能使用 GitHub 自动生成的 Full Changelog。GitHub push 通过 `atomgit.com` 同步代码，GitHub Release 发布成功后通过 `https://api.atomgit.com/api/v5` 创建同版本 Release；创建请求必须显式传入 AtomGit 支持的 `release_status: "latest"`（预发布使用 `pre`），不能使用 `published` 或依赖服务端默认状态。附件先调用 `releases/{tag}/upload_url` 获取签名地址，再按返回的请求头使用 `PUT` 上传。所有 AtomGit 令牌只能通过 GitHub Secret 传入，禁止写入仓库。
- 所有 GitHub Actions 工作流必须提供 `workflow_dispatch`，以支持从 GitHub 页面手动触发；手动发布或同步 Release 时应提供可选或必填的版本标签输入，避免误用当前分支名。
- GitHub 到 AtomGit 的代码同步必须先将 GitHub 分支 fetch 到 `refs/remotes/origin/*`，再显式映射推送至 AtomGit 分支，不能 fetch 到可能已检出的本地分支；AtomGit 作为镜像时可强制更新其分支和标签。
- 平台专用实现的测试必须仅在对应平台执行；发布工作流的测试矩阵至少覆盖 Linux 与 Windows，避免 Linux CI 漏测 Windows 专用行为。

## 数据模型

- 当前应用尚未发布，数据库结构变更不需要维护向下兼容逻辑。
- 所有数据库表和后续字段、索引变更统一通过 GORM `AutoMigrate` 自动同步，不手写迁移脚本。
- 修改 GORM 模型时必须考虑已有数据执行 `AutoMigrate` 的行为；新增字段优先设置安全的 `default` 值，或不要设置 `not null`，避免 SQLite 因历史记录无法填充而启动失败。
- 客户端的可扩展设置统一保存到 `settings` 表：以 `key` 区分设置类型，`setting` 使用 `longtext` 保存 JSON；不为单个设置类型创建专用配置表。
- 每个数据模型使用独立 Repository；跨模型的原子操作由所属业务仓库协调事务，不将站点或设置 CRUD 堆叠到应用仓库中。
- 站点模型使用 `AppId`、`Https` 字段，对应数据库小写列 `app_id`、`https`；不要使用全大写缩写字段名。
- 每次应用扫描前检查已登记根目录；丢失目录标记为禁用，禁用应用不参与站点扫描，重新发现同路径时恢复启用。
- Windows 平台比较应用根目录时必须忽略路径大小写，等价路径应更新已有应用，不能重复插入。
- 删除已登记应用时，必须在同一数据库事务中删除其全部站点记录，避免留下孤立的 `app_site` 数据。
- 站点支持单独启用或禁用；禁用站点不参与证书同步，也不计入应用的站点、HTTPS 站点及同步状态统计；后续扫描发现同一站点时应保留其状态。
- 站点同步写入前必须按 `primary_domain + config_path + app_id` 去重；同一身份的重复结果优先保留 HTTPS 和证书路径信息。Windows 比较 `config_path` 时忽略大小写。

## TUI 交互

- Windows 平台启动客户端时应检测管理员权限；未提升时通过 UAC 重新启动当前可执行文件，非 Windows 平台不进行提权。
- 顶部菜单使用直角边框；左右键和上下键均可选择菜单，当前菜单项下方必须显示中文帮助说明。
- 顶部菜单的“定时同步”按 Enter 后必须先弹出确认提示（不得直接启动），用户确认后才能将客户端注册并启动为系统服务（Windows Service / Linux systemd）；启动成功后退出 TUI，服务在后台按计划执行站点扫描与证书同步，并开机自启。启动前需确保 `schedule` 设置 `enabled` 为 true 且 `cron` 已解析（留空按启动时刻推导每日计划）。注册统一走 `service.EnsureRunning`：先用 `Status` 判断，已运行则不变，已安装未运行则只 `Start`，未安装才 `Install`+`Start`，绝不能对已注册服务重复安装（kardianos `Install` 对已存在服务会报错）。Linux 非 root 下，确认后由 TUI 以 `tea.ExecProcess` 拉起 `sudo certd-client service ensure` 子进程（将终端让给 sudo 以便输入密码）；Windows 启动已 UAC 提权、Linux 已 root 时直接调用 `EnsureRunning`，不需额外提权。提权准备失败（如保存定时设置）或 sudo 被拒时必须将原因展示并留在界面，不得报告虚假成功。
- 耗时的扫描和 I/O 操作必须在后台执行，不能阻塞终端界面的键盘响应和重绘。
- 长时间扫描开始时立即记录日志；执行超过 10 秒后，每 10 秒输出一次进度，至少包含已处理数量和当前待处理数量。
- 站点扫描完成提示必须汇总发现总数、保留的禁用站点数、启用的 HTTPS 站点数和新增站点数；存在错误时同时显示失败数。禁用站点不计入 HTTPS 与同步统计。
- 执行日志需要有独立边框、每条记录的本地时间，并支持 `PageUp` / `PageDown` 分页浏览。
- 界面日志只显示单行简要信息（首行、限长并提示查看日志文件），外部命令的详细错误堆栈和多行输出只写入 `logs/client.log`，避免 PowerShell 异常块撑满日志区域。
- 顶部菜单必须提供“定时设置”：输入五段 Cron 表达式并实时预览下一次执行时间，用 `e` 切换启用/禁用（Cron 含空格，切换键不能占用空格），回车保存前用 `schedule.Validate` 校验，表达式非法不得保存并留在当前屏幕；保存写入 `settings` 表的 `schedule` 键，供 `start`、定时同步与系统服务共用。
- 顶部菜单必须提供“服务管理”：Windows 通过通用链接 `cmd /c start "" services.msc` 直接打开系统服务面板，非 Windows 平台给出 `systemctl` 命令提示；打开动作在后台执行且不等子进程结束，失败原因显示到状态栏并写入日志。
- 顶部菜单必须提供“打开日志”，用系统默认程序打开 `logs/client.log`（Windows 用 `notepad.exe`，macOS 用 `open`，其他用 `xdg-open`），打开失败要把原因显示到状态栏并写入日志；日志路径统一由 `internal/logging` 提供，打开动作在后台执行且不受“已有任务正在执行”限制，因为同步失败时正是最需要查看日志的时刻。
- 日志中的换行与超长文本必须按日志内容区可用宽度折行，折行后的实际高度须参与布局计算，不能越过边框或触发终端滚屏。
- TUI 任一渲染行都应保留终端最后一列，避免 Windows 终端在满宽输出时自动换行，造成增量重绘错位或残留内容。
- 证书同步的进度日志、失败汇总和通知内容必须包含应用类型、站点 ID 与站点域名，方便定位具体部署目标。
- 整个 TUI 输出必须适配当前终端高度；空间不足时优先压缩日志可见行并截断数据列表，不能因终端滚屏让旧内容混入菜单或其他区域。
- 已登记应用以表格展示，包含清晰表头、应用 ID、应用类型、安装目录和站点数量；不展示添加时间作为常规列表列。站点管理列表同样显示站点 ID。
- 表格中的安装目录和配置文件路径列必须随可用终端宽度扩展，并仅在空间不足时截断；省略号的预留宽度必须与实际显示字符一致。
- 应用列表的站点数、HTTPS 站点数、已同步和异常列必须使用固定列宽，表头与数据行使用同一组宽度。
- 应用和站点表格的表头与数据行之间必须显示横向分隔线，且分隔线宽度与表格内容区一致。
- 通过键盘触发的管理操作必须在对应视图中有可见提示，不能依赖用户猜测快捷键。
- 站点管理列表必须显示站点启用状态，并支持用键盘切换状态。
- 客户端崩溃必须留痕：未捕获 panic 与 fatal error 必须写入日志文件，不能只打印到 stdout（bubbletea 默认吞掉 panic 只打印到终端，需用 `WithoutCatchPanics` 让主循环 panic 传播到 recover，并用 `debug.SetCrashOutput` 兜底后台 goroutine panic 与 fatal error）。
- macOS 上 textinput 的 Ctrl+V 剪贴板粘贴会调用 `pbpaste` 子进程，在 TUI raw 模式下可能闪退；macOS 应禁用该绑定，粘贴统一走终端原生 Cmd+V。

## 规范回顾

- 每次任务收尾时，回顾用户提出的要求，识别其中可跨任务复用、稳定且不与既有规范冲突的约束。
- 将这些共性要求精炼后更新本文件；一次性的任务细节、临时偏好和未经确认的推断不写入长期规范。
- 更新后检查规则是否清晰、可执行且没有重复或互相矛盾的表述。
