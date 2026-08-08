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


## 技术栈

* golang
* sqlite3

## 当前客户端原型

启动 TUI：

```bash
go run ./cmd/certd-client
```

Windows 启动时会请求管理员权限，以读取 IIS 配置并支持后续证书部署；非 Windows 平台不会请求提权。

界面包含“应用扫描”、“扫描站点”和“应用管理”三个菜单。选择“应用扫描”后输入根目录，程序会递归识别 Nginx 的 `*/conf/nginx.conf`、`*/sbin/nginx`、`*/bin/nginx` 以及 Apache 的 `*/conf/httpd.conf`、`*/bin/httpd` 等特征，列出安装根目录；IIS 不递归扫描目录，而是执行 `appcmd list site` 检查安装状态并登记 `inetsrv` 目录。使用空格勾选、回车保存到 SQLite 的 `target_app` 表。相同安装路径会原地更新并保留应用 ID。

每次应用扫描开始前，客户端会校验已登记应用的根目录；不存在的目录会标记为禁用，不参与站点扫描。再次扫描到同一路径时会自动恢复启用。

“扫描站点”会扫描所有已登记应用的配置树：Nginx 解析 `server_name`、HTTPS 监听和证书配置；Apache 解析 `VirtualHost`、`ServerName`、`ServerAlias` 与 SSL 指令；IIS 解析 `config/applicationHost.config` 中的站点绑定。结果同步到 SQLite 的 `app_site` 表。站点记录包含主域名、子域名数量、配置文件路径、应用 ID 和 HTTPS 状态。已登记应用会显示在中部区域，操作日志同时写入 `./logs/client.log`，数据库文件默认位于 `./data/certd-client.db`。

应用扫描与站点扫描通过已注册的 `app_provider` 执行。新增应用类型时实现并注册对应 Provider，即可接入同一主流程。

“应用管理”可查看已登记应用的站点列表，也支持删除应用；在应用管理中按回车查看站点，按 `d` 删除当前应用并按 `y` 确认。删除确认后会同时删除该应用关联的全部站点记录。

执行日志支持 `PageUp` / `PageDown` 分页浏览，新日志产生时自动回到最新页。

扫描在后台执行。扫描时间超过 10 秒时，执行日志每 10 秒记录一次已扫描目录数和当前待扫描目录数，界面保持可响应。
