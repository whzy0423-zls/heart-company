# 九型问答 App 总计划流程图

来源计划：
- 后端/后台计划：`/Users/wohenzaiyi/Desktop/nine-xing/docs/app-backend-admin-plan.md`
- Flutter App 计划：`/Users/wohenzaiyi/Desktop/nine-xing-app/docs/flutter-app-plan.md`

执行原则：
- 后端/后台放在当前 `nine-xing` 项目。
- Flutter App 放在独立 `nine-xing-app` 项目。
- 每个阶段必须完成本地验证、联调验证和用户验收后，才进入下一阶段。

```mermaid
flowchart TD
    START([九型问答 App 总计划启动])
    CTRL[固定控制区<br/>Current Status / Phase Progress / Change Log]
    RULE{当前 Phase<br/>是否验收通过?}
    NEXT[进入下一 Phase]
    HOLD[停留当前 Phase<br/>修复问题并重新验证]

    START --> CTRL --> RULE
    RULE -- 是 --> NEXT
    RULE -- 否 --> HOLD --> RULE

    NEXT --> SPLIT{按项目拆分执行}

    subgraph BACKEND["nine-xing：后端 API + 后台管理 + RAG + 计费"]
        direction TB
        B1[Phase 1<br/>App API 基础与健康检查]
        B2[Phase 2<br/>手机号登录与 App 用户体系]
        B3[Phase 3<br/>主卡生成与九型画像]
        B4[Phase 4<br/>会话与消息落库]
        B5[Phase 5<br/>公共知识库导入与基础 RAG]
        B6[Phase 6<br/>私库记忆]
        B7[Phase 7<br/>追问建议、反馈、收藏与搜索]
        B8[Phase 8<br/>成长画像、练习、任务、周报与趋势]
        B9[Phase 9<br/>海报分享与归因]
        B10[Phase 10<br/>计费、权益、支付与模型配置]
        B11[Phase 11<br/>隐私与数据安全]
        B12[Phase 12<br/>推送通知与触达]
        B13[Phase 13<br/>可观测性与运营监控]
        B14[Phase 14<br/>关系合盘]
        BGATE[后端/后台 Release Gate<br/>测试 / 权限 / 数据隔离 / 内容安全 / 后台页面 / App 联调]

        B1 --> B2 --> B3 --> B4 --> B5 --> B6 --> B7 --> B8 --> B9 --> B10 --> B11 --> B12 --> B13 --> B14 --> BGATE
    end

    subgraph APP["nine-xing-app：Flutter Android App"]
        direction TB
        A1[Phase 1<br/>Flutter 独立项目基础]
        A2[Phase 2<br/>手机号登录与账号状态]
        A3[Phase 3<br/>新手引导与主卡生成]
        A4[Phase 4<br/>主副卡管理]
        A5[Phase 5<br/>基础问答窗口]
        A6[Phase 6<br/>公共知识问答体验]
        A7[Phase 7<br/>私库记忆可见化]
        A8[Phase 8<br/>追问建议与回答反馈]
        A9[Phase 9<br/>收藏与历史搜索]
        A10[Phase 10<br/>成长状态画像]
        A11[Phase 11<br/>每日成长练习]
        A12[Phase 12<br/>7 天成长任务]
        A13[Phase 13<br/>海报分享]
        A14[Phase 14<br/>计费与权益]
        A15[Phase 15<br/>模型能力包装]
        A16[Phase 16<br/>成长周报]
        A17[Phase 17<br/>状态趋势图]
        A18[Phase 18<br/>隐私与数据安全]
        A19[Phase 19<br/>关系合盘]
        A20[Phase 20<br/>推送通知]
        A21[Phase 21<br/>崩溃监控与行为埋点]
        A22[Phase 22<br/>上架合规与发布准备]
        AGATE[Flutter Release Gate<br/>flutter analyze / Android 构建 / 页面走查 / 弱网验证 / 技术词包装 / API 联调]

        A1 --> A2 --> A3 --> A4 --> A5 --> A6 --> A7 --> A8 --> A9 --> A10 --> A11 --> A12 --> A13 --> A14 --> A15 --> A16 --> A17 --> A18 --> A19 --> A20 --> A21 --> A22 --> AGATE
    end

    SPLIT --> B1
    SPLIT --> A1

    B1 -. 健康检查接口 .-> A1
    B2 -. 登录接口与 App JWT .-> A2
    B3 -. 主卡画像与主副卡接口 .-> A3
    B3 -. 卡片权益限制 .-> A4
    B4 -. 会话与消息 API .-> A5
    B5 -. 公共知识命中结果 .-> A6
    B6 -. 私库记忆读写与停用 .-> A7
    B7 -. 追问建议、反馈与收藏接口 .-> A8
    B7 -. 历史搜索接口 .-> A9
    B8 -. 状态画像数据 .-> A10
    B8 -. 每日练习数据 .-> A11
    B8 -. 7 天任务数据 .-> A12
    B9 -. 海报分享归因 .-> A13
    B10 -. 权益查询与扣减、支付下单 .-> A14
    B10 -. 芯之力专属模型展示规则 .-> A15
    B8 -. 周报与趋势数据 .-> A16
    B8 -. 趋势图数据 .-> A17
    B11 -. 导出 / 清空 / 删除账号 .-> A18
    B14 -. 合盘解读与权益接口 .-> A19
    B12 -. 设备 token 与推送下发 .-> A20
    B13 -. 埋点上报接口 .-> A21

    BGATE --> ACCEPT{整体验收通过?}
    AGATE --> ACCEPT
    ACCEPT -- 是 --> RELEASE([准备发布或进入下一大版本])
    ACCEPT -- 否 --> FIX[定位失败项<br/>回到对应 Phase 修复并重新验证] --> SPLIT
```
