# 小程序公共微信支付调用说明

## 适用范围与数据流

当前小程序复用微信支付 v3 JSAPI 链路：业务接口创建订单并返回 `payParams`，公共方法调用 `uni.requestPayment`，随后查询业务状态接口。只有服务端收到并校验微信支付通知、将订单落账并发放权益后，前端才把购买视为成功。`requestPayment` 的 `success` 回调只表示收银台流程返回，不能单独作为已支付凭据。

```text
业务页 -> 鉴权下单接口 -> { outTradeNo, amount, payParams }
       -> 公共支付方法 -> 微信收银台
       -> 业务状态接口轮询 -> 服务端确认 paid/owned -> 刷新业务内容
微信支付 -> POST /api/pay/notify -> 服务端验签、核对订单和金额、幂等落账
```

## 后台上下架控制

管理员在后台「小程序管理 → 支付设置」（页面路径 `/miniapp/payment`，需要 `Website:Write` 权限）维护全局开关。该页面通过现有站点配置接口读取和保存 `home.miniappPayment.enabled`：

```json
{
  "home": {
    "miniappPayment": {
      "enabled": false
    }
  }
}
```

`enabled: true`（缺少该字段时的兼容默认值）表示支付入口正常展示；设为 `false` 后，结果页深度报告解锁、课堂系列/单课购买和「我的」页面支付联调入口均隐藏或置为不可购买状态。服务端也会拒绝新的报告、课堂、支付测试和开发模拟支付下单，返回 HTTP `503`；客户端不能通过直接调用接口绕过该开关。

配置接口：管理端使用 `GET /api/site-config` 读取完整配置，修改 `home.miniappPayment.enabled` 后用 `PUT /api/site-config` 保存；小程序使用公开的 `GET /api/public/site-config` 读取开关。保存接口接收完整站点配置对象，不能只提交 `miniappPayment` 子对象。

下架只影响新支付，不撤销历史订单或权益：已购买课件、已解锁报告和订单状态查询继续可用；微信仍可能发送已完成订单的通知，`POST /api/pay/notify` 不受该开关拦截，仍会验签、幂等落账并发放权益。后台保存后，小程序在下一次刷新公共站点配置时读取新值；服务端每次下单都会读取持久化配置，因此无需重启服务。

小程序请求客户端 `src/api/request.js` 使用 `VITE_API_BASE`（已含 `/api`）、自动添加已登录用户的 Bearer token，并解包后端 `{ code: 0, data: ... }`。下文接口路径均省略基址中的 `/api`。

## 公共前端方法

实现位置：`src/utils/payment.js`。

| 方法 | 入参 | 返回/用途 |
| --- | --- | --- |
| `requestWechatPayment(payParams)` | 服务端返回的 `payParams` | Promise；调用一次微信收银台，取消或失败时 reject。不查询订单状态。 |
| `payWechatOrder(order, { devPay })` | 服务端订单；开发模拟支付函数 `devPay(order)` | Promise；真实订单调用 `requestWechatPayment`；仅当 `order.payParams.devMode === true` 时调用业务提供的鉴权开发模拟接口。 |
| `createWechatPaymentController(options)` | 见下表 | 返回 `{ purchase, retry, stop, reset, snapshot }`；统一下单、支付和有界状态确认。 |
| `payForReport(testRecordId)` | 当前用户的测试记录 ID | Promise；保留结果页原有入口，支付后等待服务端报告解锁状态确认再 resolve。 |

`createWechatPaymentController` 的 `options`：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `create()` | 是 | 调用业务下单接口，返回至少包含 `outTradeNo`、`payParams` 的订单。金额、标题、商品身份由服务端决定。 |
| `status(order)` | 是 | 查询服务端权威状态。返回 `{ status: 'paid' }` 或 `{ owned: true }` 表示已获得权益；`pending` 表示继续等待。 |
| `devPay(order)` | 开发模拟时 | 调用对应业务的鉴权 `dev-pay` 接口；正式支付不调用。 |
| `onChange(snapshot)` | 否 | 状态变化回调，用于同步页面文案和按钮状态。 |
| `onSuccess(status)` | 否 | 服务端确认成功后调用，可刷新报告、课件或用户权益。 |
| `wait(ms)` | 否 | 自定义等待函数，主要用于测试。 |
| `intervalMs` / `maxAttempts` | 否 | 状态查询间隔和最大次数；默认沿用现有课堂链路的 1.2 秒、最多 6 次。 |

`snapshot()` 返回 `{ state, message, order, status }`。`state` 为 `idle`、`creating`、`pending`、`success`、`cancelled` 或 `failure`；查询超时以 `failure` 和明确的超时文案表示。`purchase()` 与 `retry()` 返回本次操作的最终 snapshot，重复点击在同一轮支付中复用同一个 Promise。页面卸载时调用 `stop()`，取消过期异步结果更新；用户主动关闭支付提示时调用 `reset()`。`cancelled` 是用户在收银台的操作结果，不代表服务端订单已经关闭；若随后收到成功通知，仍以服务端状态和权益为准。

`payParams` 由后端 `wxpay.Prepay` 生成，字段为 `timeStamp`、`nonceStr`、`package`、`signType`、`paySign`，开发环境可能另有 `devMode: true`。前端不要生成、修改或持久化签名材料。

### 课堂购买调用示例

```js
import {
  createClassroomOrderApi,
  devPayClassroomOrderApi,
  getClassroomOrderStatusApi,
} from '../../api'
import { createWechatPaymentController } from '../../utils/payment'

const targetType = 'content' // 或 'series'
const refId = '21'
const payment = createWechatPaymentController({
  create: () => createClassroomOrderApi(targetType, refId),
  devPay: (order) => devPayClassroomOrderApi(order.outTradeNo),
  status: () => getClassroomOrderStatusApi(targetType, refId),
  onChange: (snapshot) => {
    paymentState.value = snapshot.state
    paymentMessage.value = snapshot.message
  },
  onSuccess: async () => {
    await loadDetail() // 重新读取服务端课件权限
  },
})

const result = await payment.purchase()
// result.state === 'success' 才是服务端确认后的购买成功。
// 页面卸载时调用 payment.stop()。
```

后续新增付费业务时，页面只接入该控制器并提供 `create/status/onSuccess` 业务适配器；如需开发模拟，再提供该业务自己的 `devPay`。不应复用报告或课堂的 `dev-pay` 接口去支付其他商品。

## 当前后端接口映射

以下业务接口均要求小程序登录态；`request.js` 的 API 基址已经包含 `/api`。

| 业务 | 下单请求 | 状态确认 | 开发模拟 |
| --- | --- | --- | --- |
| 深度报告 | `POST /miniapp/report/order`，`{ "testRecordId": "21" }` | `GET /miniapp/report/status?testRecordId=21`，返回 `{ unlocked, priceCents }`；公共层将 `unlocked: true` 视作已确认成功 | `POST /miniapp/report/dev-pay`，`{ "out_trade_no": "...", "trade_state": "SUCCESS" }` |
| 课堂系列/单课 | `POST /miniapp/classroom/orders`，`{ "targetType": "series"/"content", "refId": "21" }` | `GET /miniapp/classroom/orders/status?targetType=...&refId=21`，返回 `{ outTradeNo, product, refId, title, amount, status, owned }` | `POST /miniapp/classroom/orders/dev-pay`，`{ "outTradeNo": "..." }` |
| 微信支付联调单 | `POST /miniapp/wechat-pay-test/order`，无请求金额；后端固定为 10 分 | 当前无面向小程序的订单状态接口，不发放业务权益 | 不支持开发模拟；只用于生产微信支付拉起联调 |

报告下单返回 `{ outTradeNo, amount, payParams }`；课堂下单返回 `{ outTradeNo, product, refId, title, amount, payParams }`。`amount` 与 `priceCents` 的单位均为人民币**分**，仅作展示或核对，不允许客户端在下单请求中提交或覆盖金额。报告价格由服务端配置 `WXPAY_REPORT_PRICE_CENTS`，课堂价格取自服务端已发布的系列/课件销售快照。未来调整金额也只修改服务端商品/价格配置。

## 状态与异常处理

- `pending`：微信侧或后端通知尚未完成，继续按有界轮询查询；超过次数显示确认超时，不自动授予权益。
- `paid` / `owned: true` / 报告 `unlocked: true`：服务端已确认并发放对应业务权益，才执行 `onSuccess`。
- `closed`、`cancelled`、`canceled`、`failed`、`refunded`：终态或不可继续支付状态，展示失败并允许用户重新发起业务下单。
- 用户取消微信收银台：显示取消，不显示支付成功；已有订单可能仍为 `pending`，再次购买由服务端处理复用或重建。
- 网络错误、参数错误或确认超时：保留明确错误消息和重试操作；不将 `requestPayment` 返回当作落账证明。

## 新业务接入与生产配置

1. 服务端先定义商品/业务目标、可售校验、价格来源及状态查询接口；金额必须由服务端读取，保存订单金额和标题快照。客户端只传业务标识，例如目标 ID。
2. 服务端复用现有 `orders`、`wxpay.Prepay` 和 `/api/pay/notify`；在支付回调中验签、核对 AppID/商户号/订单号/精确金额，幂等更新订单，并在同一事务中发放对应权益。`/api/pay/notify` 仅供微信支付通知，小程序不要调用。
3. 前端通过 `createWechatPaymentController` 接入新业务的下单、查状态、成功刷新；列表和详情中的展示价读取服务端价格字段，不在小程序写死价格或商户参数。
4. 生产服务端配置 `WXPAY_MCH_ID`、`WXPAY_APPID`、`WXPAY_API_V3_KEY`、`WXPAY_SERIAL_NO`、`WXPAY_PRIVATE_KEY_PATH`、`WXPAY_PLATFORM_CERT_PATH` 或公钥配置，以及公网 HTTPS `WXPAY_NOTIFY_URL`；`WXPAY_DEV` 必须为 `false`。证书、私钥、APIv3 Key、openid 和签名逻辑只留在服务端。
5. 在微信开发者工具及真机验证支付成功、取消、重复点击、通知延迟、轮询超时、重复通知和退款状态；生产环境不得调用任何 `dev-pay` 接口。

“我的”页面 0.10 元联调入口是特殊测试单：目前只复用 `requestWechatPayment` 验证真实收银台能否拉起，缺少业务状态查询接口，所以不能作为其他付费功能的完成态模板。
