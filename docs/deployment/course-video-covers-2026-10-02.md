# 家庭关系与领导力默认封面 — 2026-10-02

线上两门课程的 `cover` 为空，旧客户端回退到 `course-family.webp`、`course-team.webp`。两张 WebP 文件仍在包内，未被此前包体优化删除；此次采用兼容性更好的真实视频 JPG 封面，并将它们持久化到课程配置，使当前体验版也能拉取。

## 素材来源

- **领导力 · 团队协作**：现有视频 `website-react/public/assets/videos/laohan-14.mp4`（“怕出错，何以创造？”）第 2 秒。原帧 1080×1920，裁取 `(0,635,1080,1243)` 的老师授课画面，缩放为 960×540 JPG。
- **家庭关系 · 系统排列**：现有视频 `website-react/public/assets/videos/laohan-10.mp4`（“停止内耗：亲密关系的困惑”）第 2 秒。裁取 `(0,657,1080,1265)`，缩放为 960×540 JPG。

去除竖版视频上下标题装饰，保留老师头部和上半身；未生成人物图。源文件保存为 `miniapp/src/static/editorial/course-team.jpg` 和 `course-family.jpg`。通过既有发布脚本产生带内容哈希的 HTTPS 地址：

- `/assets/miniapp-share/course-team-06f9d00bb373.jpg`（50,838 B）
- `/assets/miniapp-share/course-family-1c06ed06518a.jpg`（69,238 B）

## 交付与验证

- 已将两门课程封面保存到线上数据库 `site_configs/default` 的 `home.miniappCourses.items`，同时更新配置文件的同名空封面字段。按课程 ID、标题及空封面条件匹配，在事务锁与配置摘要校验后更新；前后 JSON 对比确认只有这两个 cover 字段改变。
- 当前体验版重新进入报名页后即可拉取真实 HTTPS 配置，无需为这两张已配置封面重新发布客户端。
- 新客户端将默认封面与两个历史 WebP 路径统一解析为新的 HTTPS JPG，保留后台自定义图。新增 JPG 排除出微信主包，完整配置测试和生产构建通过，最终包体 2,005,867 B（小于 2 MiB）。
- 两个线上图片地址经匿名 HTTPS 下载，与本地 JPG 逐字一致；公开配置返回正确 URL。
- 微信开发者工具重新打开生产构建，实际报名列表中的领导力、家庭关系卡片均显示老师视频实景，人物头部和上半身可见。
- 静态站点仅增加两张图片，之前所有文件校验保持相同；未重启服务。持久化网站镜像为 `heart-company-website:course-covers-20261002`（`sha256:1d10408b141843610eb8889dbd0dca52c865f91e33717f088786b3d9b5f7e433`），两个 Compose 覆盖文件只改变 website.image，既有服务镜像和设置保留。

服务器备份和证据：`/opt/heart-company/.deploy/course-covers-20261002/`。包含数据库/文件配置修改前后快照、网站文件校验清单、覆盖文件和有效配置快照。回滚配置时只恢复这两门课的原 cover 值，保留之后的业务修改；已公开的哈希图片应继续保留，避免旧分享卡片失效。
