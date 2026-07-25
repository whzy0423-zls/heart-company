# 微信小程序正式登录切换设计

## 目标

将九型芯之力小程序从缺少微信凭证时的本地模拟登录切换为微信正式登录。小程序通过 `uni.login` 获取临时 code，服务端调用微信 `jscode2session` 换取 openid，并签发项目自己的小程序 JWT。

## 现状

- 小程序已经实现 `uni.login -> POST /api/wx/login` 的调用链。
- Go 服务已经实现 `code2session`、微信用户落库和小程序 JWT 签发。
- 小程序工程仍配置旧 AppID。
- 正式服务器缺少 `WECHAT_APPID`、`WECHAT_SECRET` 和显式的 `WECHAT_LOGIN_DEV=false`，当前会自动进入模拟登录模式。

## 设计

1. 将 `miniapp/src/manifest.json` 与 `miniapp/project.config.json` 中的微信小程序 AppID 更新为正式 AppID。
2. AppSecret 只配置在正式服务器根目录 `.env`，不得写入 Git、日志、构建产物或前端代码。
3. 正式环境设置：
   - `WECHAT_APPID` 为正式小程序 AppID。
   - `WECHAT_SECRET` 为正式小程序 AppSecret。
   - `WECHAT_LOGIN_DEV=false`，禁止凭证缺失时使用模拟 openid。
4. 保留现有接口和数据模型，不新增登录接口，不改变 JWT 或用户表结构。
5. 服务器配置修改前创建带时间戳的 `.env` 备份，只重建 `server` 服务。
6. 完成验证后审查 Git diff，确认 AppSecret 和小程序构建产物均未被跟踪，只提交 AppID 配置、测试、设计和实施文档，并推送到 `main`。

## 数据流

1. 小程序调用 `uni.login({ provider: 'weixin' })` 获取一次性 code。
2. 小程序将 code、渠道和场景发送到 `/api/wx/login`。
3. Go 服务使用正式 AppID 和 AppSecret 请求微信 `sns/jscode2session`。
4. 服务端以 openid/unionid 创建或更新小程序用户。
5. 服务端签发 `TokenKindMiniapp` JWT，小程序保存 token，后续请求通过 Bearer Token 鉴权。

## 错误处理与安全

- 空 code、失效 code、AppID 不匹配和微信接口错误继续返回明确的登录失败响应。
- 不向客户端返回 session_key、AppSecret 或微信接口请求地址中的敏感参数。
- AppSecret 不提交 Git；最终报告不展示 AppSecret、JWT 或其他密钥。
- 如果微信后台尚未配置合法 request 域名，部署后需要在微信公众平台加入正式 HTTPS API 域名。

## 验证

- 新增 AppID 一致性回归断言，确认 `miniapp/src/manifest.json` 与 `miniapp/project.config.json` 均使用同一个正式 AppID。
- 小程序配置测试和构建通过，并确认 `miniapp/dist/build/mp-weixin/project.config.json` 使用相同的正式 AppID 与正式 HTTPS API 地址。
- Go 服务全量测试通过。
- 正式服务器容器内 AppID 匹配、Secret 已设置、模拟登录关闭。
- `/api/status` 返回 200。
- 使用无效 code 请求 `/api/wx/login` 时应返回微信 `code2session` 错误，而不是生成模拟用户，证明正式微信调用链已启用。
- 最终成功登录需要在微信开发者工具或真实小程序中用有效 `wx.login` code 完成一次端到端验证。
