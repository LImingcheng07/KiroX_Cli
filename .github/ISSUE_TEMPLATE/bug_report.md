---
name: Bug 反馈
about: 报告一个 Bug 或异常行为
title: "[Bug] "
labels: bug
assignees: ''
---

## Bug 描述

<!-- 简洁清晰地描述遇到的问题 -->

## 复现步骤

1. 执行命令 `./kirox-cli ...`
2. 观察到 ...
3. 出现错误 ...

## 期望行为

<!-- 你期望发生什么 -->

## 实际行为

<!-- 实际发生了什么，请贴出完整的日志 / 报错信息 -->

```
<在此粘贴日志>
```

## 运行环境

- KiroX CLI 版本：<!-- 例如 v1.0.0，或 git commit hash -->
- 操作系统：<!-- Windows 11 / Ubuntu 22.04 / macOS 14 -->
- Go 版本：<!-- go version 输出 -->
- 邮箱模式：<!-- Outlook 池 / MoeMail -->
- 是否使用代理：<!-- 是/否，类型：HTTP / SOCKS5 / 住宅 / 机房 -->

## IP 风控自查

在提交前，请先确认是否为 IP / 邮箱域名风控（这不是 Bug）：

- [ ] 已使用代理在浏览器中打开 https://outlook.live.com 验证 IP 可用
- [ ] 已通过 `-imap` 模式验证 Outlook 账号 RefreshToken 有效
- [ ] 同样的代理 + 账号在多个 IP / 邮箱上重现该问题

## 补充信息

<!-- 截图、配置片段（请抹去敏感字段如 token、密码等）等 -->
