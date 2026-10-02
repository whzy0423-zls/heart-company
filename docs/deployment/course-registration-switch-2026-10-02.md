# 课程报名总开关 — 2026-10-02

后台入口：**小程序管理 → 课程配置 → 课程报名总开关 → 显示课程报名模块**。修改后点击保存。配置存储在现有数据库站点配置的 `home.miniappCourses.enabled`；旧配置缺少该字段时默认开启，与视频课堂开关相互独立。

关闭后：首页隐藏课程推荐，报名页保留需求类型和真实表单，隐藏课程卡片、金额及营销介绍。旧课程详情链接回到报名表单，未支付订单隐藏继续购买入口。服务端同步拒绝新的课程绑定报名和新建/重试预支付。普通表单继续事务保存客户线索、报名记录及后台通知。

既有课程配置、订单和已购课程访问保留。关闭前发起的付款继续处理回调和对账，不阻断支付确认。重新开启恢复原课程目录。小程序再次进入相关页面会刷新配置。

## 验证

- 管理后台 7 项课程编辑测试通过，覆盖默认值、关闭保存后重载、编辑和重新开启，保留视频课堂配置。
- 管理后台类型检查及生产构建通过。
- Go 35 项相关测试通过，包括隔离 PostgreSQL 中的真实表单、客户、通知保存和后台广播，关闭/重开支付、历史凭据及已购课程读取。相关包 `go vet` 通过。
- 小程序完整 `test:config` 行为测试和生产微信构建通过。
- 独立本地 API 的实际页面验证：关闭只保留表单，旧详情链接跳回表单；开启恢复课程卡片和价格。线上开关没有为验证而切换。
- 独立代码审查未发现阻塞问题。

## 部署

仅重建并替换服务端和管理后台容器；其他项目容器的 ID 和镜像均与部署前一致。两个 Compose 覆盖文件的有效配置对比确认仅 server/admin 镜像字段改变。线上站点配置 JSON 在部署前后完全相同，课程报名继续保持开启。未在生产发起报名、支付或通知测试。

服务端在当前已含付款权益修复的镜像上替换经过测试的 Go 1.22.12 Linux/amd64 程序。程序 SHA-256：`9d065a12528200c29f90cfce8535245ad552d35130eabe4bd94ac45390f104e9`。

- 服务端：`heart-company-server:course-registration-20261002`，镜像 `sha256:fbc6900246dc92384868fb2111375d8b6c13b2cc73e7d4fdbb65c00dced5fa22`。
- 后台：`heart-company-admin:course-registration-20261002`，镜像 `sha256:10a80859f2520757f485e6524e5391756beb38166cbc374fc39870492a0cce02`。
- 线上 HTTPS 健康接口返回成功；后台 HTML 与构建产物相同，新 `courses-D2Gq63gR.js` 返回成功并包含开关。

线上备份和部署证据：`/opt/heart-company/.deploy/course-registration-20261002/`。包含两个覆盖文件原件、有效配置的前后快照、容器前后快照、站点配置前后快照及健康结果。配置快照仅保存在服务器私有目录。

回滚时恢复 `default.override.before.yml` 到 `docker-compose.override.yml`，恢复 `release.override.before.yml` 到 `.deploy/release-20260930-ba3550f/deploy.override.yml`，然后在 `/opt/heart-company` 执行：

```sh
docker compose -f docker-compose.yml -f docker-compose.override.yml \
  -f .deploy/course-registration-20261002/rollback.override.yml \
  up -d --no-deps --no-build server admin
```

回滚镜像为 `heart-company-server:before-registration-20261002` 与 `heart-company-admin:before-registration-20261002`。

微信开发者工具项目为 `miniapp/dist/build/mp-weixin`；本次服务端部署不等于微信平台审核发布。

## 微信包体

交付验证时开发者工具显示原包上传超过 2 MiB（3030 KB）。将 36 张已部署的重复封面改用现有 HTTPS 原图，排除这些本地副本及 12 张无引用的 WebP；教师原图/头像及九型、三中心 canvas PNG 保留。新增构建后验证确认托管文件和原图字节一致、保留图片未改变，并限制主包小于 2 MiB。

最终完整行为测试及生产构建通过。主包实际文件总量为 **2,005,340 B（1.912 MiB）**，减少 1,220,913 B 冗余资源，距 2 MiB 上限余 91,812 B。37 个 HTTPS 托管图片均验证 HTTP 200、JPEG MIME 与 JPEG 文件签名。该包已生成，未执行微信平台上传发布。

微信开发者工具通过「项目 → 重新打开此项目」加载了该生产构建；首页正常显示真实老师资料与当前开启的课程入口，控制台未见运行错误。独立 H5/API 测试进程及临时测试网页已关闭。

收到旧包超限截图后，再次在开发者工具执行「工具 → 预览」。微信端打包上传成功并生成预览二维码，工具显示实际代码包 **1931 KB**，未再出现 80051 超限错误。本次仅生成开发预览，未提交审核或发布正式版本。
