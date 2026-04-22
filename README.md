# iss-open-cli

面向 AI Agent 和开发者的闪送开放平台命令行工具。

把同城即时配送能力封装成本地命令，让开发者能以最低的心智负担，把「询价、下单、查单、取消」接进自己的 Agent、工作流系统或业务应用。

`中文` | · [参考手册](./skills/references/cli-usage.md) · [接口参考](./skills/references/api-reference.md) · [Agent Skill](./skills/SKILL.md)

> [!IMPORTANT]
> **行业首创**：这是同城即时配送领域首个开源命令行工具，背后是闪送覆盖全国 298 座城市、服务 1 亿+ 注册用户、310 万+ 注册骑手的生产级交付能力（截至 2025 年 12 月）。
>
> **凭证与环境**：`iss-open-cli` 依赖闪送开放平台商户凭证才可调用真实接口。涉及真实下单、取消订单或生产环境调用时，请务必先确认环境、商户身份和业务参数，避免造成实际业务影响。

**目录**

- [为什么做 CLI](#为什么做-CLI)
- [适合谁使用](#适合谁使用)
- [核心能力](#核心能力)
- [安装](#安装)
- [配置](#配置)
- [快速开始](#快速开始)
- [命令行用法](#命令行用法)
- [在 AI Agent 中使用](#在-ai-agent-中使用)
- [典型场景 Playbook](#典型场景-playbook)
- [安全设计](#安全设计)
- [常见问题](#常见问题)
- [故障排查](#故障排查)
- [项目结构](#项目结构)
- [开源协议](#开源协议)

---

## 为什么做 CLI

如果你已经在用大模型、Agent、工作流引擎或者内部自动化系统，你很快会遇到一个现实问题：

> 配送不是「生成一段 HTTP 请求」这么简单。

你需要处理签名、系统参数、表单请求、平台响应解包、错误格式统一、配置加载、日志落盘，还要让 Agent 在本地命令层就能稳定执行，而不是每次都从 SDK 或接口协议重新拼装一遍。

`iss-open-cli` 要解决的，就是这件事：

- 把闪送开放平台核心能力封装成一个对 Agent 友好的本地命令
- 让开发者不需要手写签名逻辑，也不需要重复处理 HTTP 细节
- 让 AI Agent 可以通过稳定、可预测、结构化的 JSON 响应来调用同城配送能力
- 让「接入配送」从一个接口工程问题，变成一个命令调用问题

我们希望同城配送能力，像 `curl` 调一个 API 那样简单地成为 AI 时代的基础设施。

---

## 适合谁使用

- 想为自己的 AI Agent 接入同城配送能力的开发者
- 想在本地脚本、工作流引擎、自动化任务里调用闪送能力的工程团队
- 需要做询价、下单、查单、取消等开放平台集成的业务系统开发者
- 希望把配送能力作为 Agent Tool 暴露给 Claude Code、Codex、Cursor、Openclaw 等智能体使用的团队

---

## 核心能力

当前版本 **[v1.0.0](https://github.com/shansongtech/iss-open-cli/releases/tag/1.0.0)** 内置 **4 个高频动作码**，覆盖同城配送全生命周期的基础闭环：

| 动作码 | 中文名 | 对应平台接口 | 典型用途 |
| --- | --- | --- | --- |
| `orderCalculate` | 订单询价 | `/openapi/merchants/v5/orderCalculate` | 实时估算配送费用、预计接单时长、预计送达时长 |
| `orderPlace` | 提交订单 | `/openapi/merchants/v5/orderPlace` | 基于询价结果正式下单，返回费用明细、补贴、商家权益 |
| `orderInfo` | 查询订单详情 | `/openapi/merchants/v5/orderInfo` | 查询订单状态、骑手信息、实时轨迹、取送密码 |
| `abortOrder` | 取消订单 | `/openapi/merchants/v5/abortOrder` | 取消未完成订单，返回扣款金额、返程费、取消类型 |

运行时查看：

```bash
iss-open-cli --list                              # 列出全部动作码
iss-open-cli --api orderCalculate --example      # 查看参数示例
```

### 订单状态枚举

| `orderStatus` | 描述 |
| --- | --- |
| 20 | 派单中 |
| 30 | 待取货 |
| 40 | 闪送中 |
| 50 | 已完成 |
| 60 | 已取消 |

### 物品类型枚举（节选）

| 值 | 类型 | 值 | 类型 |
| --- | --- | --- | --- |
| 1 | 文件 | 10 | 其他 |
| 3 | 数码 | 12 | 母婴 |
| 5 | 蛋糕 | 13 | 医药健康 |
| 6 | 餐饮 | 15 | 商超 |
| 7 | 鲜花 | 16 | 水果 |
| 9 | 汽配 | | |

完整字段约束与费用类型编码见 [接口参考文档](./skills/references/api-reference.md)。

---


## 安装

当前最新 Release：**[v1.0.0](https://github.com/shansongtech/iss-open-cli/releases/tag/1.0.0)**，已提供 macOS / Linux / Windows 预编译二进制。完整版本列表见 [GitHub Releases](https://github.com/shansongtech/iss-open-cli/releases)。

### 方式一：一键安装脚本（推荐）

仓库提供了 [`install.sh`](./install.sh)，自动完成「平台检测 → 从 GitHub Releases 下载对应二进制 → 解压到 `$HOME/.iss-open-cli` → 配置 PATH」全流程，支持全新安装与原地升级。

```bash
# 自动安装最新版本
curl -fsSL https://raw.githubusercontent.com/shansongtech/iss-open-cli/main/install.sh | bash

# 或指定版本号
curl -fsSL https://raw.githubusercontent.com/shansongtech/iss-open-cli/main/install.sh | bash -s -- 1.0.0
```


### 方式二：手动下载预编译包

如果你不希望执行一键脚本，也可以直接到 [Releases 页面](https://github.com/shansongtech/iss-open-cli/releases/tag/1.0.0) 下载对应平台的压缩包：

| 平台 | 文件名 |
| --- | --- |
| macOS Apple Silicon（M1/M2/M3/M4） | `iss-open-cli-1.0.0-macos-arm64.tar.gz` |
| macOS Intel | `iss-open-cli-1.0.0-macos-amd64.tar.gz` |
| Linux x86_64 | `iss-open-cli-1.0.0-linux-amd64.tar.gz` |
| Linux ARM64 | `iss-open-cli-1.0.0-linux-arm64.tar.gz` |
| Windows x86_64 | `iss-open-cli-1.0.0-windows-amd64.tar.gz` |

以 Apple Silicon Mac 为例：

```bash
VERSION=1.0.0
curl -fsSLo iss-open-cli.tar.gz \
  https://github.com/shansongtech/iss-open-cli/releases/download/${VERSION}/iss-open-cli-${VERSION}-macos-arm64.tar.gz

mkdir -p $HOME/.iss-open-cli
tar -xzf iss-open-cli.tar.gz -C $HOME/.iss-open-cli --strip-components=1
chmod +x $HOME/.iss-open-cli/iss-open-cli

# 可选：清除 macOS Gatekeeper 隔离属性
xattr -d com.apple.quarantine $HOME/.iss-open-cli/iss-open-cli 2>/dev/null || true

# 可选：加入 PATH（zsh 示例）
echo 'export PATH="$PATH:$HOME/.iss-open-cli"' >> ~/.zshrc
source ~/.zshrc
```

### 方式三：从源码构建

适合想阅读源码、做二次开发、或在内网环境中使用的开发者。要求 Go **1.26.0** 或更高版本。

```bash
git clone https://github.com/shansongtech/iss-open-cli.git
cd iss-open-cli
go build -o iss-open-cli .

# 按推荐目录布局安装
mkdir -p $HOME/.iss-open-cli/configs
cp ./iss-open-cli $HOME/.iss-open-cli/iss-open-cli
cp ./configs/config.yaml $HOME/.iss-open-cli/configs/config.yaml
chmod +x $HOME/.iss-open-cli/iss-open-cli
```

### 验证安装

```bash
iss-open-cli --version
iss-open-cli --list
```

如果 `iss-open-cli` 命令未找到，说明 PATH 尚未生效，可执行 `source ~/.zshrc`（或重新开一个终端）后再试，或直接使用全路径 `$HOME/.iss-open-cli/iss-open-cli`。此路径也与 `SKILL.md` 中 Agent 约定保持一致，AI 工具无需额外配置即可发现二进制。

> **macOS 用户注意**：若提示「无法打开，Apple 无法检查是否包含恶意软件」，执行 `xattr -d com.apple.quarantine $HOME/.iss-open-cli/iss-open-cli`。一键安装脚本已默认处理此步骤。

---

## 配置

使用前需要在 [闪送开放平台](https://open.ishansong.com) 获取商户凭证：

- `client_id`
- `app_secret`
- `shop_id`

### 配置模板

```yaml
api:
  base_url: "https://open.ishansong.com"     # 生产环境
  # base_url: "http://open.s.bingex.com"     # 测试环境
  timeout: 3

auth:
  client_id: "your-client-id"
  app_secret: "your-app-secret"
  shop_id: "your-shop-id"

log:
  level: "info"                              # debug / info / warn / errors
```

### 配置项说明

| 配置项 | 说明 |
| --- | --- |
| `api.base_url` | 闪送开放平台接口地址 |
| `api.timeout` | 请求超时时间，单位秒，默认 3 |
| `auth.client_id` | 商户应用标识 |
| `auth.app_secret` | 商户应用密钥，请勿提交到 Git |
| `auth.shop_id` | 商户/门店 ID |
| `log.level` | 日志级别，默认 `info` |

### 环境变量覆盖

适合 CI、Docker、多租户切换场景，优先级高于配置文件：

| 环境变量 | 对应配置项 |
| --- | --- |
| `ISS_OPEN_API_URL` | `api.base_url` |
| `ISS_OPEN_AUTH_CLIENT_ID` | `auth.client_id` |
| `ISS_OPEN_AUTH_SHOP_ID` | `auth.shop_id` |
| `ISS_OPEN_AUTH_APP_SECRET` | `auth.app_secret` |
| `ISS_OPEN_LOG_LEVEL` | `log.level` |

### 开放平台准入

本 CLI 调用的是闪送开放平台 `merchants/v5` 商家接口，使用前请先在闪送开放平台完成：

1. 注册开发者账号并创建应用，获取 `clientId` / `appSecret`
2. 绑定门店，获取 `shopId`
3. 开通「订单计费、提交订单、查询订单详情、订单取消」四项 API 权限

---

## 快速开始

### 1. 查看 CLI 支持什么能力

```bash
iss-open-cli --list
```

### 2. 查看某个动作的参数示例

```bash
iss-open-cli --api orderCalculate --example
```

### 3. 订单询价

```bash
iss-open-cli --api orderCalculate --data '{
  "cityName":"北京市",
  "sender":{
    "fromAddress":"北京市海淀区xx路xx号",
    "fromSenderName":"张三",
    "fromMobile":"13800138000",
    "fromLatitude":"40.049058",
    "fromLongitude":"116.379594"
  },
  "receiverList":[
    {
      "orderNo":"ORDER_202604220001",
      "toAddress":"北京市海淀区yy路yy号",
      "toLatitude":"40.043612",
      "toLongitude":"116.361199",
      "toReceiverName":"李四",
      "toMobile":"13900139000"
    }
  ]
}'
```

成功响应核心字段通常包括：

- `orderNumber`，后续提交订单时使用
- `totalAmount`，原始订单总金额
- `totalFeeAfterSave`，优惠后金额
- `feeInfoList`，费用明细
- `totalDistance`，总距离
- `estimateGrabSecond` / `estimateReceiveSecond`，预计接单和送达时长

### 4. 提交订单

```bash
iss-open-cli --api orderPlace --data '{"issOrderNo":"TDH2026041300954053"}'
```

### 5. 查询订单详情

```bash
iss-open-cli --api orderInfo --data '{
  "issOrderNo":"TDH2026041300954053",
  "thirdOrderNo":"ORDER_202604220001"
}'
```

### 6. 取消订单

```bash
iss-open-cli --api abortOrder --data '{"issOrderNo":"TDH2026041300954053"}'
```

---

## 命令行用法

### 核心 Flag

| Flag | 说明 |
| --- | --- |
| `--api <code>` | 动作码，必填。支持 `orderCalculate` / `orderPlace` / `orderInfo` / `abortOrder` |
| `--data '<json>'` | 业务参数，必填。紧凑 JSON，推荐用单引号包裹避免 Shell 破坏双引号 |
| `--list` | 以表格形式打印全部支持的动作码 |
| `--example` | 配合 `--api` 查看指定动作码的参数示例 |
| `-v, --version` | 输出版本 / commit / 构建时间 |

### 统一输出格式

`iss-open-cli` 的标准输出始终是结构化 JSON。

成功响应：

```json
{
  "status": 200,
  "err": "",
  "success": true,
  "data": {
    "orderNumber": "TDH2026041300954053",
    "totalAmount": 1800
  }
}
```

失败响应：

```json
{
  "status": 400,
  "err": "参数错误:缺少必填参数",
  "success": false,
  "data": null
}
```

Agent 只需要读取 `success` / `status` / `err` / `data` 四个字段即可完成结果判定。

### 错误码语义

| Status | 含义 |
| --- | --- |
| 200 | 成功 |
| 400 | 参数错误（动作码非法、JSON 格式错误、配置缺失） |
| 500 | CLI 内部错误（日志初始化失败、序列化失败） |
| 502 | 远程调用错误（网络异常、平台非 200、响应非合法 JSON） |

---

## 在 AI Agent 中使用

`iss-open-cli` 天然适合作为本地 Tool 提供给 Agent。最典型的调用方式就是直接执行本地命令：

```bash
$HOME/.iss-open-cli/iss-open-cli --api orderInfo --data '{"issOrderNo":"TDH2026041300954053"}'
```

### Agent Skill 目录结构

仓库内置完整的 Agent Skill 体系（`skills/`），Claude Code、Codex、Cursor、Openclaw 等支持 Skill 机制的 Agent 开箱即用：

```text
skills/
├── SKILL.md                      # 意图路由、核心流程、动作码清单、调用约定
├── agents/
│   └── openai.yaml               # Codex / OpenAI 兼容 Agent 的默认提示词
└── references/
    ├── api-reference.md          # 签名协议、必填字段、响应字段、枚举取值
    └── cli-usage.md              # 安装路径、配置、命令格式、故障排查
```

### Agent 调用范式

用户只需要说出自然语言意图，Agent 自动走完整链路：

```
用户：帮我把这份合同从国贸送到望京 SOHO
Agent：
  1. 读取 SKILL.md，识别「同城配送」意图
  2. 读取 api-reference.md，构造 orderCalculate 参数
  3. 执行 iss-open-cli --api orderCalculate --data '{...}'
  4. 解析 JSON，向用户展示预估费用和时效
  5. 用户确认后执行 iss-open-cli --api orderPlace --data '{...}'
  6. 返回订单号与骑手信息
```

### 典型 Agent 集成方式

- 把它挂到 Claude Code、Codex、Cursor 一类 Agent 的 shell / tool 调用链路里
- 把它接进自有 Agent Framework，作为配送相关 Tool
- 把它用于本地脚本、自动化任务、测试联调和业务回归

---

## 典型场景 Playbook

### 场景 1：餐饮商家 SaaS 自动派单

收银台下单后，自动触发同城配送：

```bash
# 步骤 1：询价并向用户展示配送费
ISS_OPEN_AUTH_SHOP_ID="SHOP_BEIJING_001" iss-open-cli --api orderCalculate --data '{...}'

# 步骤 2：用户确认支付后下单
iss-open-cli --api orderPlace --data '{"issOrderNo":"TDH..."}'

# 步骤 3：轮询订单状态推送给门店端和用户端
iss-open-cli --api orderInfo --data '{"issOrderNo":"TDH..."}'
```

### 场景 2：AI 客服自助退换

用户在客服对话中表达「帮我把包裹取消」，Agent 自动识别订单并取消：

```bash
iss-open-cli --api abortOrder --data '{"issOrderNo":"TDH..."}'
```

返回 `deductAmount` 和 `sendBackFee`，Agent 据此向用户解释取消费用。

### 场景 3：多门店 CI / CD 任务

利用环境变量隔离不同门店的凭证，在同一批脚本中并发调用：

```bash
for shop in shop_a shop_b shop_c; do
  ISS_OPEN_AUTH_SHOP_ID="$shop" iss-open-cli --api orderInfo --data "..." &
done
wait
```

---

## 安全设计

同城配送涉及真实资金流、真实骑手、真实用户地址，`iss-open-cli` 在架构层面把安全作为一等公民。

### 签名与传输

| 机制 | 说明 |
| --- | --- |
| **自动签名** | CLI 内置签名算法，客户端无需手动计算 |
| **凭证不出进程** | `appSecret` 仅在内存中参与签名，不写入日志、不随请求 body 传输 |
| **强制 POST form** | 所有调用均为 `POST application/x-www-form-urlencoded`，`data` 为紧凑 JSON 字符串参与签名，避免中间人篡改 |
| **可配置超时** | `api.timeout` 默认 3 秒，防止 Agent 挂起 |

### 审计与可观测

| 机制 | 说明 |
| --- | --- |
| **trace_id 贯穿** | 每次 CLI 启动生成 UUID traceID，写入每条日志，便于跨系统追溯 |
| **双路日志** | `logs/app.log`（info 及以上）与 `logs/errors.log`（error 单独归档），自动按 500MB 轮转、压缩、保留 10 天 |
| **平台侧审计** | 每一次调用都经闪送开放平台，商家后台可查询完整调用记录 |

### 开发者与企业侧

| 机制 | 说明 |
| --- | --- |
| **配置文件外置** | `configs/config.yaml` 不打包进二进制，建议加入 `.gitignore`，避免密钥泄露 |
| **环境变量优先** | 生产环境推荐用 `ISS_OPEN_AUTH_*` 环境变量注入，不落盘 |
| **测试 / 生产隔离** | 通过 `api.base_url` 切换，测试环境 `http://open.s.bingex.com`，生产环境 `https://open.ishansong.com` |
| **最小权限** | 商家后台可按门店、按 API 粒度授权，CLI 不具备越权能力 |

发现安全漏洞？请通过 GitHub Security Advisories 报告，或联系 [open_seller@ishansong.com](mailto:open_seller@ishansong.com)。

---

## 常见问题

### 1. 为什么执行时提示未找到配置文件

请检查以下位置是否存在 `configs/config.yaml`：

- 当前工作目录
- 可执行文件同级目录（即 `$HOME/.iss-open-cli/configs/config.yaml`）

推荐直接按本文档的安装目录结构放置配置文件。

### 2. 为什么明明传了 JSON，还是提示参数格式错误

请优先检查 shell 引号。建议始终使用单引号包住 `--data` 的 JSON 字符串，避免内部双引号被 shell 破坏。必要时可以用 `jq -c` 预先压缩再传入。

### 3. 为什么平台返回成功率不稳定

请先区分：

- 是配置错误
- 是网络问题
- 是开放平台业务返回失败
- 还是测试环境和生产环境混用

建议优先查看 `logs/errors.log` 获取更具体的错误上下文，对照平台返回的 `status` 与 `msg` 排查。

### 4. 这个 CLI 适合生产系统直接调用吗

可以，但建议先明确以下边界：

- 生产凭证与测试凭证必须隔离
- 下单、取消等写操作应放在有业务确认的流程里
- 需要由你的业务系统自己负责幂等、重试和业务补偿策略

`iss-open-cli` 解决的是让 Agent「更容易调用开放平台能力」，不是替代你的业务系统治理能力。

---

## 故障排查

| 现象 | 原因与排查 |
| --- | --- |
| `未找到配置文件` | 检查 `$HOME/.iss-open-cli/configs/config.yaml` 或当前工作目录下的 `configs/config.yaml` 是否存在 |
| `--data 参数不是合法的 JSON 格式` | 优先检查 Shell 引号，推荐单引号包住 JSON；必要时用 `jq -c` 压缩后再传入 |
| `--api 参数非法` | 运行 `iss-open-cli --list` 核对动作码，注意大小写敏感 |
| `平台返回失败：...` | 查看 `logs/app.log`，定位平台返回的 `status` 与 `msg`，对照闪送开放平台错误码文档 |
| `HTTP 状态码异常: xxx` | 网络或代理问题，排查 `api.base_url` 配置与本地网络 |
| 请求超时 | 上调 `api.timeout` 或排查网络抖动 |
| 签名错误 | 通常是 `client_id` / `shop_id` / `app_secret` 不匹配，或本地时钟与服务端偏差过大 |

更多排查要点见 [CLI 使用说明](./skills/references/cli-usage.md#排查要点)。

---

## 项目结构

```text
iss-open-cli/
├── cmd/
│   └── root.go                   # 入口、命令定义、签名、HTTP 客户端、业务 Handler
├── configs/
│   └── config.yaml               # 配置模板
├── skills/
│   ├── SKILL.md                  # Agent 意图路由与调用约定
│   ├── agents/
│   │   └── openai.yaml           # Codex / OpenAI 兼容提示词
│   └── references/
│       ├── api-reference.md      # 接口参考
│       └── cli-usage.md          # CLI 使用说明
├── go.mod
├── LICENSE
├── main.go
└── README.md
```

---


## 开源协议

本项目采用 [Apache License 2.0](./LICENSE)， 欢迎 Issue、PR、Fork，也欢迎 ISV 基于本 CLI 构建上层 Agent Skill 与垂直场景应用。
