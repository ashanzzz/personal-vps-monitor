# 更新日志

本项目遵循语义化版本。日期为发布日。

## 未发布

三页列表（云服务器 / 轻量 / 二手机）支持表头排序 + 列宽拖拽记忆 + 一键恢复默认宽度。

### 新增
- **表头排序**：三页列表（共用 `CategoryPage`）的名称、地区、CPU、内存、硬盘、带宽、流量、价格、库存列均加 `sorter`，支持点击表头按数值/字符串排序。原「配置」合并列拆为 CPU / 内存 / 硬盘 / 带宽 / 流量 五个独立可排序列。
- **列宽可拖拽 + 持久化**：表头单元格用 `react-resizable` 包裹，可拖拽调整列宽。宽度变化经 500ms 防抖后写入 `POST /api/settings`，按页存入 `col_widths_cloud` / `col_widths_ls` / `col_widths_secondhand`（值为 `{列key: 宽度px}`）；刷新或重新进入页面后读回应用。三页各存各的。
- **「恢复默认宽度」按钮**：每页列表工具栏新增按钮，点击清空该页 `col_widths_*`（置为 `{}`）并恢复代码中定义的默认列宽。
- 后端 `config.py` 的 `DEFAULTS` 新增 `col_widths_cloud` / `col_widths_ls` / `col_widths_secondhand` 三个键（默认 `{}`），复用现有 settings 读写，不新建表。

## v0.7.0

清理旧版全局抢购 + 设置页版本信息展示。

### 新增
- **设置页「版本信息」卡片**：调用已有的 `GET /api/version`，展示版本号、git_sha（取前 7 位，为空显示 `-`）、构建时间（为空显示 `-`）。页面加载时拉取一次。

### 变更
- 修复二手列表 `page_size` 请求参数（驼峰 `pageSize` → 下划线 `page_size`）。此前已合并（PR #1）但尚未随版本发布。

### 移除
- **永久移除旧版全局二手自动抢购**（`auto_buy_*` 全套）：删除 `config.py` 中 6 个 legacy 键、`main.py` 中遗留的全局 poller 注释与重排逻辑、前端 `api.ts` 的 `auto_buy_*` 字段、`SettingsPage.tsx` 的旧版抢购卡片、`MachinePanel.tsx`/`MachineCard.tsx` 的 `autoBuyEnabled` 传参。新抢购统一走每页（云服务器 / 轻量 / 二手机）条件范围规则，不再保留任何兼容层。二手机手动「抢购」按钮保留（走 `POST /api/buy/{plan_id}`，购买周期改取 `secondhand_buy.cycle`）。

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
