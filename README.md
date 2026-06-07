<h1 align="center">KiroX CLI</h1>

<p align="center">
  AWS Builder ID (Kiro) 批量自动注册工具 · 命令行版本
</p>

<p align="center">
  <img src="https://img.shields.io/badge/version-v1.1.1-6366f1?style=flat-square" alt="version">
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-0078d4?style=flat-square" alt="platform">
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go" alt="go">
  <a href="https://linux.do"><img src="https://img.shields.io/badge/LINUX%20DO-社区-f0b752?style=flat-square" alt="LINUX DO"></a>
  <img src="https://img.shields.io/badge/license-Apache%202.0-green?style=flat-square" alt="license">
</p>

---

> 如果这个项目对你有帮助，欢迎在 GitHub 右上角点一个 **Star** 支持一下。
> 使用或二次开发过程中有问题，也可以加 QQ：**2779249042** 咨询交流。

## 简介

KiroX CLI 是基于 [huey1in/KiroX_Cli](https://github.com/huey1in/KiroX_Cli) 二次开发的命令行工具。原项目来自 [KiroX](https://github.com/huey1in/kirox) 的命令行版本，去除了图形界面依赖，仅保留核心注册逻辑。当前版本保留已验证的 Outlook 邮箱池、MoeMail 临时邮箱和自建 Cloudflare Temp Mail 能力，并提供 `email.TempEmailService` 接口，方便用户按自己的邮箱服务继续扩展。

## 二次开发说明

- 上游项目：[huey1in/KiroX_Cli](https://github.com/huey1in/KiroX_Cli)
- 原作者：[@huey1in](https://github.com/huey1in)
- 本仓库为个人二次开发版本，保留原项目 Apache License 2.0 协议和作者署名。
- 主要改动：保留核心注册流程，支持 Outlook / MoeMail / 自建 Cloudflare Temp Mail，增加代理池轮换、结果文件安全写入、依赖整理和少量基础测试。
- 邮箱扩展建议通过 `internal/email/interface.go` 中的 `TempEmailService` 接口自行实现；除自建 Cloudflare Temp Mail 外，本仓库不内置未验证的第三方邮箱 Provider。

---

## 功能特性

**注册流程**
- 完整的 AWS Builder ID 注册自动化（OIDC 注册 → 设备授权 → 邮箱验证 → 密码设置 → SSO → Kiro Token 交换）
- 注册完成后自动验证账号存活状态、额度信息
- 支持串行 / 并发批量注册
- 已注册的账号自动从 CSV 中剔除，支持断点续跑

**邮箱支持**
- **Outlook 邮箱池**：从 CSV 导入 `邮箱----密码----客户端ID----RefreshToken` 格式账号，自动通过 IMAP 获取验证码
- **MoeMail 临时邮箱**：通过 API 获取临时邮箱并接收验证码
- **Cloudflare Temp Mail**：支持用户自建实例，可通过 `-p` 代理访问
- **自定义邮箱扩展**：实现 `internal/email/interface.go` 中的 `TempEmailService` 接口后，可按需接入自己的邮箱服务

**反检测**
- 随机化 Chrome 版本（120–144）
- 随机化设备指纹（GPU、内存、CPU 核数、屏幕分辨率）
- WebGL 扩展伪造、Canvas 指纹生成
- 基于 `tls-client` 的 TLS 指纹模拟

**代理**
- 支持 HTTP / HTTPS / SOCKS5
- 启动时自动检测出口 IP 归属地

**结果输出**
- 注册成功的账号实时增量写入 JSON
- 包含 refreshToken / clientId / clientSecret / 订阅状态等字段

---

## 快速开始

### 环境要求

- Go 1.24+

### 从源码构建

```bash
# 克隆仓库
# 原项目：git clone https://github.com/huey1in/KiroX_Cli.git
git clone https://github.com/LImingcheng07/KiroX_Cli.git
cd KiroX_Cli

# 拉取依赖
go mod tidy

# 编译
go build -o kirox-cli

# 或直接运行
go run main.go
```

---

## 使用说明

### 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-n` | int | `1` | 注册数量 |
| `-o` | string | `output/results.json` | 结果输出 JSON 路径 |
| `-p` | string | 无 | 代理地址，多个地址用逗号分隔，每任务轮换使用 (e.g. `socks5://ip1:port,socks5://ip2:port`) |
| `-d` | int | `3` | 串行模式下任务间隔秒数 |
| `-j` | int | `1` | 并发数（建议 1–5） |
| `-debug` | bool | `false` | 调试模式（输出详细日志） |
| `-outlook` | bool | `false` | 使用 Outlook 邮箱池模式 |
| `-outlook-csv` | string | `outlook.csv` | Outlook CSV 文件路径 |
| `-moemail-url` | string | `https://api.moemail.app` | MoeMail API 地址 |
| `-moemail-key` | string | 无 | MoeMail API Key |
| `-cftemp` | bool | `false` | 使用自建 Cloudflare Temp Mail |
| `-cftemp-url` | string | 环境变量 `CFTEMP_BASE_URL` | Cloudflare Temp Mail 地址 |
| `-cftemp-admin-key` | string | 环境变量 `CFTEMP_ADMIN_KEY` | Cloudflare Temp Mail 管理密码 |
| `-cftemp-custom-auth` | string | 环境变量 `CFTEMP_CUSTOM_AUTH` | Cloudflare Temp Mail 站点密码，可留空 |
| `-cftemp-domain` | string | 环境变量 `CFTEMP_DOMAIN` | 自定义域名，可留空使用服务默认域名 |
| `-imap` | bool | `false` | IMAP 邮件测试模式 |
| `-imap-csv` | string | `outlook.csv` | IMAP 测试用 CSV |
| `-imap-i` | int | `0` | 测试 CSV 中第几个账号（从 0 开始） |

### 1. 配置环境变量（可选）

在项目根目录创建 `.env` 文件：

```env
MOEMAIL_BASE_URL=https://api.moemail.app
MOEMAIL_API_KEY=your_api_key_here

# Cloudflare Temp Mail（使用 -cftemp 时需要）
CFTEMP_BASE_URL=https://mail.example.com
CFTEMP_ADMIN_KEY=your_admin_key_here
CFTEMP_CUSTOM_AUTH=optional_site_password
CFTEMP_DOMAIN=example.com
```

### 2. Outlook 邮箱池模式

准备 `outlook.csv`，每行一条账号，格式：

```
邮箱----密码----客户端ID----RefreshToken
xxx@outlook.com----password----xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx----M.C5XX...
```

运行：

```bash
# 使用 Outlook 池注册 10 个，并发 3，使用代理
./kirox-cli -outlook -outlook-csv outlook.csv -n 10 -j 3 -p http://user:pass@127.0.0.1:7890
```

### 3. MoeMail 临时邮箱模式

```bash
# 使用 MoeMail 注册 5 个，串行，间隔 5 秒
./kirox-cli -n 5 -d 5 \
  -moemail-url https://api.moemail.app \
  -moemail-key YOUR_API_KEY
```

### 4. Cloudflare Temp Mail 模式

适用于用户自建的 Cloudflare Temp Mail 服务。该模式会复用 `-p` 代理配置，因此自建服务或网络环境需要代理时可以直接传入代理参数。

```bash
./kirox-cli -cftemp \
  -cftemp-url https://mail.example.com \
  -cftemp-admin-key YOUR_ADMIN_KEY \
  -cftemp-domain example.com \
  -n 5 -d 5 \
  -p socks5://127.0.0.1:10808
```

如果服务端有自定义站点密码，再加：

```bash
-cftemp-custom-auth YOUR_SITE_PASSWORD
```

`-cftemp-domain` 可留空，留空时使用自建服务的默认域名策略。

### 5. 自定义邮箱服务扩展

本仓库内置 Outlook、MoeMail 和自建 Cloudflare Temp Mail。其他邮箱服务请自行实现接口：

```go
type TempEmailService interface {
    Create() string
    WaitForCode(timeoutSec, intervalSec int) (string, error)
    GetAddress() string
}
```

接口定义位置：`internal/email/interface.go`。实现后在 `internal/core/registrar.go` 的 `Step3Email` 中按需接入即可。

### 6. IMAP 测试模式

用于验证 Outlook 账号的 RefreshToken 是否有效、能否拉取邮件：

```bash
# 测试 outlook.csv 中第一个账号（索引 0）
./kirox-cli -imap -imap-csv outlook.csv -imap-i 0
```

### 7. 代理配置

支持以下格式（通过 `-p` 参数传入）：

```
http://user:pass@host:port
http://host:port
socks5://host:port
socks5://user:pass@host:port
```

**代理池轮换**：`-p` 支持逗号分隔多个代理地址，每个注册任务自动按顺序轮换使用。例如：

```bash
# 3 个代理轮换，并发 3，每个任务分配不同出口 IP
./kirox-cli -n 9 -j 3 \
  -p "socks5://ip1:10808,socks5://ip2:10808,socks5://ip3:10808"
```

启动时会逐个检测所有代理的出口 IP、地区、ISP 并打印。留空 `-p ""` 则直连。

### 8. 查看结果

注册成功的账号默认写入 `output/results.json`，格式：

```json
[
  {
    "email": "xxx@outlook.com",
    "refreshToken": "...",
    "provider": "BuilderId",
    "clientId": "...",
    "clientSecret": "...",
    "region": "us-east-1",
    "creditUsed": 0,
    "creditLimit": 100,
    "subscription": "FREE"
  }
]
```

结果文件采用 **增量追加** 模式，重复运行不会覆盖之前的成功账号。

---

## 项目结构

```
KiroX_cli/
├── main.go                    # 入口，命令行参数解析、批量调度
├── go.mod / go.sum            # Go 依赖
├── .env                       # 环境变量（MoeMail 等）
├── outlook.csv                # Outlook 账号池（用户提供）
├── output/
│   └── results.json           # 注册结果输出
└── internal/
    ├── core/                  # 注册核心逻辑
    │   ├── registrar.go       # Registrar 结构体，HTTP 客户端
    │   ├── run.go             # 步骤编排
    │   ├── auth.go            # OIDC 注册 / 设备授权
    │   ├── signup_flow.go     # 注册流程
    │   ├── signup_password.go # 密码设置
    │   ├── kiro_auth.go       # SSO 授权
    │   ├── kiro_exchange.go   # Token 交换
    │   └── verify.go          # 账号验证（额度 / 订阅）
    ├── browser/               # 浏览器指纹模拟
    ├── email/                 # 邮箱服务（Outlook IMAP / MoeMail）
    ├── crypto/                # JWE 加密、XXTEA
    └── http/                  # TLS 客户端工具
```

---

## 技术栈

| 层 | 技术 |
|----|------|
| 语言 | Go 1.24 |
| HTTP 客户端 | [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) |
| 加密 | RSA-OAEP-256 + AES-256-GCM (JWE) |
| 配置 | [joho/godotenv](https://github.com/joho/godotenv) |

---

## 关于 Pro / 绑卡脚本

本仓库不把银行卡开通 Pro 的参考脚本作为正式功能发布。相关实现如果用户有需要，请自行基于自己的环境完善和验证；发布版本仅保证核心注册流程、Outlook / MoeMail / 自建 Cloudflare Temp Mail、邮箱接口和结果输出路径。

## 注意事项

- 本工具仅供学习和研究使用，请遵守 AWS 服务条款
- 强烈建议配合**住宅代理**使用，机房 IP 极易触发 AWS / Microsoft 风控
- Outlook 账号需提前准备好有效的 RefreshToken
- 并发数过高可能触发 AWS 风控，建议从 `-j 1` 开始测试
- 已成功注册的账号会自动从 `outlook.csv` 中移除，请提前备份

---

## 常见问题

### IP 纯净度相关

如果运行中频繁出现以下报错，多半是当前出口 IP 不够纯净（代理 IP 已被 AWS / Microsoft 风控）。

**情况一：发送邮箱验证码响应 OTP 400**

建议更换更干净的住宅代理。

> 如果使用的是一次性邮箱（MoeMail 等），OTP 400 也可能是邮箱域名已被 Microsoft / AWS 拉黑导致；可换一个域名再试。

**情况二：注册流程卡住或邮箱无法访问**

- 用本机浏览器（带相同代理）尝试打开 [outlook.live.com](https://outlook.live.com)
- 浏览器都打不开 / 跳验证码 → 当前 IP 已被 Microsoft 风控，需要换代理
- 浏览器能正常访问 → 检查 Outlook 账号的 RefreshToken 是否仍然有效（用 `-imap` 测试模式验证）

### 「邮箱已注册过」

程序检测到该邮箱已经注册过 AWS Builder ID，会自动跳过并从 CSV 中移除该账号，继续尝试下一个。


---

## 作者

**1in** · [@huey1in](https://github.com/huey1in)（原作者）

**LImingcheng07** · 二次开发维护

Copyright © 2026

---

## 开源协议

本项目基于 [Apache License 2.0](LICENSE) 开源。

```
Copyright 2026 1in

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
