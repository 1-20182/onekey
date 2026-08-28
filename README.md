# Onekey

Onekey Steam Depot Manifest Downloader —— 一键解锁 Steam 游戏库。

> 本项目基于上游 [ikunshare/Onekey](https://github.com/ikunshare/Onekey) 二次开发，按 GPLv2 协议分发。
>
> - 原上游版权：`Copyright (C) ikunshare / ikun0014`
> - 本次改动版权：`Copyright (C) 2026 飞翔的死猪`
>
> 详细授权条款见 [LICENSE](LICENSE)（GNU GPL v2）。

## 这是什么

针对国内网络环境优化的 Steam 本地解锁工具。在纯单机 / 无 VAC / 离线场景下，通过**纯本地模拟**
（写入 `appinfo.vdf`、`depotcache` 与 Lua 注入）解锁游戏库，全程不上传任何账号服务端数据，
**全程无需 API Key**。带绿色玻璃拟态的轻量 UI，自动适配深浅色主题。

## 主要功能

- **免 Key 游戏搜索**：官方商店 API 优先，失败自动回退 GitHub 公开数据源，输入游戏名即可定位 AppID。
- **AppID 直解锁**：不经商店搜索，走国内 CDN 拉取解锁数据，商店域名断连时仍可用。
- **游戏库管理**：新增 / 移除、查看仓库与 DLC 详情。
- **内置内核一键安装**：随附 OpenSteamTools 内核 DLL，自动拷贝到 Steam 根目录。
- **杀软一键放行**：PowerShell 提权自动添加 Windows Defender 排除，降低误报摩擦。
- **国内网络优化**：多 CDN 镜像并发抢答、GitHub 域名 DoH 纯净解析、3 秒镜像超时，下载稳而快。
- **代理配置与连通性测试**，任务状态实时展示，一键重启 Steam（Lua 热重载）。

## 使用方法

1. 打开 Onekey，输入游戏名称或 App ID，点「解锁」。
2. 首次使用点「内核工具 → 安装内核」，把随附的 OpenSteamTools 内核放入 Steam 根目录。
3. 重启 Steam 一次即生效；Lua 配置热重载，之后无需重启。

> 注意：若同时运行其它解锁工具（如 Steam++ / 青霜），会互相覆盖 `appinfo.vdf` 导致解锁失效，
> 请只保留一款解锁工具。

## 内置内核（第三方，GPL-3.0）

本程序将 **OpenSteamTool**（https://github.com/OpenSteam001/OpenSteamTool ，v1.4.8，GPL-3.0）
的 `dwmapi.dll`、`xinput1_4.dll`、`OpenSteamTool.dll` 内置随附，运行时拷贝到 Steam 根目录。
这些 DLL 是**独立的第三方作品**，以 OpenSteamTool 自身的 GPL-3.0 条款分发，与本程序（GPL-2.0）
互不影响，属各自独立授权。

## 封号风险说明（请务必先读）

> 诚实声明：**本项目不提供“绝对不会封号”的保证**，任何声称“100% 不封”的说法都是不可信的。
> 本段只给出真实的技术依据，帮你判断风险高低并自行决定。

### 为什么多数情况下风险低（技术依据）

Onekey 的解锁是**纯本地模拟**：只在你自己的电脑上改 Steam（写入 `appinfo.vdf`、`depotcache`、
SteamTools/OpenSteamTool 的 Lua 注入），**从不修改、也从不向上传任何 Steam 账号的服务端数据**。
Steam 服务端并没有“你已购买该游戏”的授权记录，因此服务端无从比对“你没买却玩过”——社区对
这个机制的主流共识是：Steam **不会因为本地解锁而主动判定越权并封号**。只玩单机、离线、无 DLC
等内容都在本地的游戏时，风险极低。

### 什么情况下会真被封（务必避开）

1. **带 VAC 反作弊的联机游戏**：这类游戏会校验本地游戏文件是否被改动/注入。你带着被注入的解锁
   状态进 VAC 服务器，会被反作弊抓到——这是**真实封号**，且 VAC 封禁是整个账号全局的。
   **绝对不要在带 VAC 的联机游戏上使用。**
2. **联机对战 / 排行榜 / 服务端校验的游戏**：即使非 VAC，联机服务端通常也会校验授权，可能触发
   账号级处罚或数据异常。
3. **内容全部在服务端的 DLC/皮肤/内购**：解锁的只是外壳，服务端不认，既无效果又平白增加风险。

### 安全使用建议

只用 **纯单机 / 无 VAC / 离线** 场景，风险极低；带联机反作弊的游戏坚决不用。

## 构建

- 前端：`frontend/`（Vite），构建产物约 76 KB，无外部依赖。
- 后端：Go + [Wails](https://wails.io) v2。
- 打包：`wails build`。

## 法律责任与无保证声明

本程序按 GNU GPLv2 分发，且如 [LICENSE](LICENSE) 所述提供“不附带任何保证”（WITHOUT ANY
WARRANTY）。一切风险与后果由使用者自行承担。

## 许可证

[GNU General Public License v2.0](LICENSE)