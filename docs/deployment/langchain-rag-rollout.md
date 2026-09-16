# LangChain RAG 灰度与回滚手册

本文用于部署环境中的人工发布。所有命令都在仓库根目录执行；`TOKEN`、`HOST`、数据库地址和镜像版本由部署环境或秘密管理器注入，不写入 Git。

## 发布前门槛

1. 记录当前 server/knowledge-service 镜像 digest、数据库备份点和激活的知识 `release_id`。
2. 运行完整 Go、Python、Flutter 测试及 `docker compose config`。
3. 生成并执行 500 条评测集，确认：Recall@5 ≥ 85%、引用准确率 ≥ 95%、污染率 = 0、普通检索 P95 ≤ 1200ms、芯之力 P95 ≤ 800ms。
4. `/health/ready`、`/api/app/health` 均健康；确认知识服务只在内部网络监听。

## 基线与 Shadow

```bash
# 旧链路基线
export KNOWLEDGE_BACKEND=local
export LANGCHAIN_ROLLOUT_PERCENT=0
export LANGCHAIN_SKILL_ROLLOUT_PERCENT=0
export LANGCHAIN_XINZHILI_ROLLOUT_PERCENT=0
export LANGCHAIN_XINZHILI_REALTIME_ROLLOUT_PERCENT=0
docker compose up -d --build knowledge-service server

# Shadow：响应仍来自旧 Go RAG，远端结果只进入脱敏对比指标
export KNOWLEDGE_BACKEND=shadow
docker compose up -d --no-deps server
```

Shadow 至少覆盖一个完整业务周期。检查 `/api/app/health` 的 `metrics`：

- `knowledgeShadowErrors / knowledgeShadowTotal` 不高于旧链路错误率；
- `knowledgeShadowOverlapRate` 达到评测预期；
- 日志仅包含 request id、数量、耗时与错误类别，不包含用户问题和文档正文。

## 分阶段发布

远端流量使用稳定用户桶，同一用户不会在阶段内反复切换。每阶段至少观察一个完整业务周期，满足发布前门槛后再升档。

```bash
# 主会话：依次设置 5、20、50、100
export KNOWLEDGE_BACKEND=fallback
export LANGCHAIN_ROLLOUT_PERCENT=5
docker compose up -d --no-deps server

# 技能会话：主会话稳定后，依次设置 5、20、50、100
export LANGCHAIN_SKILL_ROLLOUT_PERCENT=5
docker compose up -d --no-deps server

# 芯之力上传语音 SSE：技能会话稳定后，依次设置 5、20、50、100
export LANGCHAIN_XINZHILI_ROLLOUT_PERCENT=5
docker compose up -d --no-deps server

# 芯之力实时 WebSocket：SSE 全量稳定后，依次设置 5、20、50、100
export LANGCHAIN_XINZHILI_REALTIME_ROLLOUT_PERCENT=5
docker compose up -d --no-deps server
```

每次将示例中的 `5` 依次替换为 `20`、`50`、`100`。升档后验证主会话 SSE、技能文字/语音、芯之力上传语音和实时 WebSocket，并记录：时间、镜像 digest、配置值、请求量、远端错误率、fallback 增量、P50/P95、污染率、引用准确率。

## 回滚演练

每一阶段都执行一次即时回滚，不迁移或删除数据库数据：

```bash
export KNOWLEDGE_BACKEND=local
export LANGCHAIN_ROLLOUT_PERCENT=0
export LANGCHAIN_SKILL_ROLLOUT_PERCENT=0
export LANGCHAIN_XINZHILI_ROLLOUT_PERCENT=0
export LANGCHAIN_XINZHILI_REALTIME_ROLLOUT_PERCENT=0
docker compose up -d --no-deps server
curl --fail --silent --show-error http://127.0.0.1:8080/api/app/health
```

确认新请求全部走旧 Go RAG、进行中的 SSE/WebSocket 正常结束或重连、会话消息与引用记录仍可读取。然后恢复刚才的灰度值并再次冒烟验证。

远端超时和 5xx 会在 `fallback` 模式回退；权限或契约类 4xx 必须 fail-close，禁止用旧链路掩盖范围错误。

## 旧 Go RAG 下线门槛

只有四类入口均 100% 且连续稳定、500 条评测全部达标、污染率为零、回滚演练有记录后，才另开变更删除旧实现。删除前保留一个可部署的旧镜像和数据库兼容窗口；本次迁移不提前移除 fallback。
