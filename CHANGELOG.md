# 更新日志

本项目遵循语义化版本。日期为发布日。

## v0.6.0

我的账户页（余额 + 账户信息 + 服务器列表）。

### 新增
- **「我的账户」页**：左侧菜单新增（钱包图标），展示：
  - **账户余额**：来自 `GET /v1/user/info` 的 `money` 字段（分），前端换算成元显示。
  - **账户信息**：用户名、邮箱、角色、Telegram 绑定、实名状态、二次验证状态。
  - **我的服务器列表**：来自 `POST /v1/server/GetServerV2List`；无实例时显示「当前没有服务器」空状态，不报错。服务器行可展开查看完整 JSON（字段名按 Akile 实际返回自适应）。
- 新增 API：`GET /api/account`（require_auth），后端一次聚合账户资料 + 余额 + 服务器列表；两个上游调用各自独立降级，一个失败不影响另一个。
- client 新增 `get_user_info` 与 `get_my_servers`（`GetServerV2List` 的 `list:null` 归一化为空列表）。

### 说明
- 余额接口经实测确认走 `GET /v1/user/info`（`/balance`、`/wallet`、`/getUserInfo` 均 404）。

## v0.5.0

多页面独立抢购 + 条件范围规则引擎 + 调试日志 + 版本管理。

### 新增
- **三页面独立**：云服务器 / 轻量服务器 / 二手机各自独立列表页、独立刷新间隔、独立抢购配置与调度器（每页一个 `poll_<page>` job）。
- **条件范围抢购规则引擎**（`buy_engine.py`）：每条规则可设标题关键词、CPU/内存/硬盘/带宽/流量/价格区间、地区、单规则限量；所有条件留空=不限，命中全部条件才抢，可应对全新名称/配置的新品。
- **多候选优选**（`rank_candidates`）：同一轮命中多台时，按价格升序、同价按硬件分降序优先下单；硬件分权重（CPU/内存/硬盘/带宽/流量）可在设置页调整。
- **抢购数量全手动**，三道配额闸门：单规则上限 `max_per_rule`、单轮合计 `max_total_per_run`、每日合计 `max_total_daily`；达上限策略 `on_limit_reached`（持续补抢 / 停止，默认持续补抢）。代码不写死任何上限。
- **刷新间隔滑块**：每页 60–6000s 滑块，可切自定义输入低至 5s（带风控提示）。
- **plan 标签映射**：列表按标签名展示（如 jppro/jpiij），plan_id 悬浮可见，无映射时直显 id。
- **调试日志系统**（`logging_setup.py`）：滚动文件 `/app/data/akile-monitor.log`（5MB×5）+ stdout 双写，结构化打点（poll.fetch / rule.match / buy.attempt / buy.pick / buy.result / token.refresh）；新增「调试日志」页可在线查看 + 下载，设置页可切 INFO/DEBUG。
- **版本管理**（`version.py`）：`__version__` 单一来源；`GET /api/version`，前端 footer 显示版本号，启动日志打印 `app.start version=`。
- 新增 API：`GET /api/version`、`GET /api/logs/app`（均 require_auth）。

### 变更
- 旧版全局二手自动抢购（`auto_buy_*`）改为「兼容保留」，新抢购统一走每页条件范围规则；旧键保留向后兼容。
- 二手抓取增加 `poll.fetch` 日志打点，空仓（list:null）明确标注 `reason=empty_market`，不再误判为故障。
- GitHub Actions 构建改为双 tag：推 master 时同时打 `:latest` 与 `:vX.Y.Z`（版本号从 `version.py` 读取）。

### 说明
- 续 token 逻辑（`token_manager.ensure_fresh`）独立运行于后端调度器，到期前 6h 自动续期，不依赖外部进程。
- 部署：`ghcr.io/ashanzzz/akile-monitor`，仅 linux/amd64。

## v0.4.0

基线版本。

### 包含
- Akile token 自动续期落入后端调度器（脱离外部进程独立运行）。
- 完整设置页（API/连接、监控范围、Telegram、Webhook、自动抢购、系统六大卡片）。
- 管理员用户名/密码鉴权（bcrypt + JWT），全部 `/api/*` require_auth，WS `?token=` 鉴权。
- 二手机被墙状态（IP normal/gfw）+ 二手↔官方套餐对比。
- GitHub Actions amd64-only 构建并推送 ghcr。
