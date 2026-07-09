# 九型芯之力小程序 QA 冒烟清单

发布前除了自动化脚本，还需要在微信开发者工具和至少一台真机上做以下检查；同时确认小程序内没有 `pages/chat`、`问 AI` 或 `AI 对话` 入口残留。

## 自动化基线

```bash
npm run test:config
VITE_API_BASE=https://xn--9iq9az5uo8fz16d.com/api npm run build:h5
VITE_API_BASE=https://xn--9iq9az5uo8fz16d.com/api npm run build:mp-weixin
```

## mp-weixin 真机检查

1. 登录链路
   - 首次进入“我的”，点击微信一键登录，确认 `wx.login` 能换取 token。
   - token 过期后进入“我的”，应提示重新登录且页面不闪乱。
2. 微信资料能力
   - 点击头像按钮，确认 `chooseAvatar` 能调起微信头像选择。
   - 昵称输入框 `type="nickname"` 能触发微信昵称建议。
   - 当前不展示手机号授权预留入口；确认页面没有 `getPhoneNumber` 授权按钮，资料保存不依赖手机号。
3. 测试与结果
   - 快速连续点击答题选项，不应跳题或错乱。
   - 完成测试后进入结果页；重启小程序后结果页仍能读取最近一次测试结果。
4. 支付与报告
   - 点击解锁报告，确认 `requestPayment` 能调起微信支付或在测试环境给出可理解提示。
   - 支付取消应提示取消，不应误标为成功。
5. 分享与海报
   - 点击分享好友，确认 `share` 卡片标题、路径、图片正常。
   - 生成海报时确认 `canvas` 能绘制头像、文字并保存到相册。
6. 学习与预约
   - 学习页冷启动能加载内容；断网时显示缓存或失败重试。
   - 预约表单填写后切页返回，草稿不应丢失；提交成功后草稿清空。

## 发布注意

- 生产构建必须配置真实 HTTPS `VITE_API_BASE`。
- 微信后台需要配置 request 合法域名，并保持 `urlCheck` 开启。
- 上传前确认 `dist/build/mp-weixin` 是最近一次构建产物。
