# 官网 App 下载与版本管理设计

## 目标

为九型芯之力官网增加可信、可维护的 Android App 下载入口，并在后台管理系统提供 APK 版本发布能力。用户无需寻找客服或外部网盘即可在官网获取当前正式版本；管理员无需把大型 APK 提交到 Git 或重新构建官网即可更新安装包。

## 用户体验

### 官网入口

- 顶部导航增加“下载 App”，跳转到首页 `/#download-app`。
- 首页 Hero 增加“下载 App”按钮，平滑滚动到下载区。
- Hero 后紧接完整 App 下载区，避免入口被课程、视频等长页面内容淹没。
- 下载区展示：
  - App 名称与一句话介绍；
  - Android 最新版本号；
  - 发布时间；
  - APK 文件大小；
  - 更新说明；
  - SHA-256 摘要；
  - 安卓安装说明；
  - Android 下载按钮；
  - 指向同一下载页或下载地址的二维码；
  - iOS“敬请期待”状态。

### 设备行为

- 桌面浏览器：显示 Android 下载按钮、二维码和完整版本信息。
- Android 浏览器：主按钮直接请求当前最新版 APK。
- iPhone/iPad：Android 主按钮替换为不可用的“iOS 敬请期待”，仍可查看产品介绍；设备判断同时覆盖传统 iOS UA 与 iPadOS 桌面模式的触摸特征。
- 未知设备：按桌面模式展示 Android 按钮与二维码，不自动触发下载。
- 无已发布版本：显示“Android 版本暂未开放下载”，不提供空链接。
- 版本接口或二维码生成失败：保留下载区布局，给出可恢复提示，不影响官网其他内容。

## 发布与管理

后台新增“App 版本管理”页面，管理员可以：

- 查看 Android 历史版本；
- 上传新的正式 APK；
- 填写更新说明；版本名称和版本号从 APK Manifest 自动提取并只读展示；
- 查看自动计算的文件大小、SHA-256 与上传时间；
- 将某一版本发布为官网最新版；
- 下架当前版本，但不删除历史文件。

首期不实现强制更新、灰度发布、iOS 包上传和 App 内更新检查。这些能力独立于官网下载安装目标，后续可在同一版本模型上扩展。

## 后端架构

### 数据模型

新增 `app_releases` 表：

- `id BIGSERIAL PRIMARY KEY`
- `platform TEXT NOT NULL`，首期仅允许 `android`
- `version_name TEXT NOT NULL`
- `version_code BIGINT NOT NULL`
- `release_notes TEXT NOT NULL DEFAULT ''`
- `file_name TEXT NOT NULL`
- `file_path TEXT NOT NULL`
- `file_size BIGINT NOT NULL`
- `sha256 TEXT NOT NULL`
- `status TEXT NOT NULL`，允许 `draft`、`published`、`archived`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `published_at TIMESTAMPTZ`

数据库增加以下约束：

- `CHECK (platform IN ('android'))`
- `CHECK (status IN ('draft', 'published', 'archived'))`
- `CHECK (version_code > 0)`
- `UNIQUE(platform, version_code)`
- 部分唯一索引 `UNIQUE(platform) WHERE status='published'`

同一平台同一时刻只允许一个 `published` 版本。发布事务锁定该平台的发布记录，先归档旧版本，再发布目标版本；部分唯一索引作为并发发布的最终保护，冲突时整个事务回滚，旧版本继续可用。

### APK 文件存储

- 文件保存在已有持久化上传卷 `/data/uploads/app-releases/`。
- 本地开发默认保存到 `UPLOAD_DIR/app-releases/`。
- APK 不进入 Git、官网 `public/`、数据库字节列或官网构建产物。
- 上传过程直接从 multipart 流写入临时文件，同时计算 SHA-256；完成验证后原子重命名到正式路径。
- 最大 APK 文件限制为 300 MiB，multipart 请求上限额外预留 1 MiB 边界开销，不复用图片上传的 20 MiB 限制。
- 若未来配置 OSS，可在保持公开接口不变的前提下把文件存储实现替换为对象存储。

### 验证与安全

- 只接受 `.apk` 扩展名。
- 通过纯 Go APK 检查器验证 ZIP 结构、`AndroidManifest.xml`、APK 签名、包名、版本名称、版本号与签名证书摘要。
- 包名必须等于 `APP_RELEASE_PACKAGE_NAME`，默认 `com.xinzhili.nine_xing_app`。
- 上传接口以 APK Manifest 提取出的版本名称和版本号为准，管理端不能用表单覆盖包内版本。
- 上传允许在正式证书指纹尚未配置时保存为 `draft`，但 APK 本身仍必须具有有效签名。
- 发布要求 `APP_RELEASE_CERT_SHA256` 已配置且与 APK 签名证书 SHA-256 一致；未配置或不匹配时拒绝发布。
- `file_path` 只保存服务端生成的相对存储键，不直接使用用户输入作为磁盘路径；读取前必须验证解析后的路径仍位于 `UPLOAD_DIR/app-releases/` 内。
- 上传失败时删除临时文件。
- 下载响应设置：
  - `Content-Type: application/vnd.android.package-archive`
  - 安全的 `Content-Disposition`
  - `X-Content-Type-Options: nosniff`
  - `ETag` 使用 SHA-256
  - `Cache-Control` 只允许短时私有缓存并要求重新验证，以便下架生效
- 公开下载使用 `http.ServeContent` 或等价流式方式，支持 `GET`、`HEAD`、Range、`If-None-Match` 与断点续传。
- 上传、发布和下架要求后台写权限；公开元数据与下载无需登录。

### API 与权限

管理端权限与菜单：

- 菜单放在“官网管理”下，标题为“App 版本”。
- 列表与版本详情权限码：`Website:AppReleases:List`。
- 上传、发布和下架权限码：`Website:AppReleases:Write`。
- 页面按钮和 API 分别按对应权限校验。

管理端 API：

- `GET /api/app-releases/list`
- `POST /api/app-releases/upload`
- `POST /api/app-releases/{id}/publish`
- `POST /api/app-releases/{id}/archive`

公开端：

- `GET /api/public/app-release/latest?platform=android`
- `GET|HEAD /api/public/app-release/download?platform=android`
- `GET|HEAD /api/public/app-releases/{id}/download`

公开元数据响应包含 `available`、版本名称、版本号、发布时间、文件大小、SHA-256、更新说明和不可变版本下载 URL，不暴露服务器磁盘路径。

- 最新元数据使用 `Cache-Control: no-cache`，允许浏览器条件请求但每次确认最新版。
- `/api/public/app-release/download` 不直接长期缓存，而是 `302` 跳转到带版本 ID 的不可变下载地址。
- `/api/public/app-releases/{id}/download` 仅允许下载当前 `published` 记录，使用 SHA-256 ETag 与 `Cache-Control: private, max-age=300, must-revalidate`。
- `draft` 版本无论 ID 是否可猜测都返回 404；`archived` 版本返回 410，表示该版本已下架。
- “下架”停止最新版发现并阻止服务端继续提供该文件。已经由用户下载到本地的 APK 无法远程撤回，官网会明确以当前发布状态为准。
- 没有发布版本时，元数据返回 HTTP 200 与 `available:false`；最新版下载返回 404。
- 已发布记录对应文件丢失时，元数据与下载返回 503，官网显示“安装包暂时不可用”，后台显示文件异常。

## 管理端页面

新增“官网管理 / App 版本”菜单，页面包含：

- 当前已发布版本摘要；
- 上传版本按钮与弹窗；
- 版本历史表格；
- 发布、下架操作；
- 上传进度；
- 300 MiB 限制、仅正式签名 APK 的提示。

管理端 nginx 只对 APK 上传路径设置 `client_max_body_size 301m`，并使用 `proxy_request_buffering off`，避免 nginx 先把整个 APK 缓冲到临时盘。官网与管理端的 APK 下载代理路径使用 `proxy_buffering off`，避免大文件响应被代理层完整缓冲。其他 API 继续使用原有限制。

## 官网实现

### 组件边界

- 新建 `AppDownloadSection`：负责下载区渲染、设备判断、版本状态和二维码。
- 新建公开 API 模块：只负责获取最新版本元数据。
- Home 只负责把下载区放在 Hero 后。
- 导航配置继续使用站点配置；新增 `下载 App` hash 链接。

### 二维码

复用官网已有 `qrcode` 依赖，在浏览器生成二维码，不依赖第三方二维码服务。二维码优先编码官网 `/#download-app`，用户扫码后由手机页面再触发下载，便于展示版本与安全信息。

### 配置与文案

下载区的标题、介绍、功能点、安装说明和 iOS 文案写入 `shared/site-config.json`，继续由现有后台站点配置体系运行时加载。版本号、发布时间、大小、摘要和更新说明由版本接口提供，避免重复配置。

## 正式签名约束

当前 Flutter 项目存在 Release 签名接入代码，但工作区中没有 `android/key.properties` 或 keystore。Debug APK 不作为公开下载版本。

本功能完成后，官网在没有已发布正式 APK 时保持“暂未开放”状态。正式上线前需单独创建或提供 Android Release keystore，并安全保存在代码仓库之外。密钥创建和保管属于发布凭证操作，不在未获明确授权时自动执行。

## 错误处理

- 文件过大：返回 413，并显示最大 300 MiB。
- 文件不是有效 APK：返回 400，并保留上传表单输入。
- 包名、包内版本或 APK 签名不符合要求：返回 400，并说明具体检查项。
- 正式证书指纹未配置或不匹配：上传草稿可保留，但发布返回 503 或 409，并提示管理员配置正确证书。
- 磁盘写入失败：返回 500，不创建数据库记录。
- 数据库记录失败：删除已完成但未登记的文件。
- 发布冲突：事务回滚，旧版本继续保持发布状态。
- 最新版本不存在：公开接口返回明确的未发布状态，官网禁用下载。
- 文件丢失：元数据接口不宣告该版本可下载，后台显示文件异常。

## 临时文件与历史文件

- 服务启动时清理 `app-releases/.tmp-*` 中超过 24 小时的上传临时文件。
- 上传在数据库写入前失败时立即删除临时文件；数据库写入失败时删除已落盘但未登记的正式文件。
- 启动检查发现未被数据库引用的孤儿正式文件时记录告警，不自动删除，避免误删人工恢复文件。
- 历史 APK 默认保留以支持回滚；后台显示历史文件总占用空间。首期不自动清理已归档版本，磁盘清理由运维明确执行。

## 测试与验证

- Go 单元测试覆盖 APK Manifest 与签名验证、包名与证书指纹、流式保存、路径边界、SHA-256、并发发布约束、公开元数据、草稿 ID 枚举、下架版本 410、HEAD、条件请求与 Range 下载。
- 管理端测试覆盖版本状态标签、上传表单校验和发布操作。
- 官网测试覆盖：
  - 下载区紧接 Hero；
  - 顶部导航与 Hero 入口；
  - Android、iOS、桌面设备分支；
  - 已发布、未发布、接口失败状态；
  - QR 内容指向官网下载区；
  - 移动端无横向滚动、触摸区域不小于 44px。
- 完成后运行 Go 全量测试、官网测试与构建、管理端类型检查与构建。
