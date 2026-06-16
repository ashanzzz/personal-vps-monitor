# 更新日志

本项目遵循语义化版本。日期为发布日。

## v0.9.0

每日签到 + AK 币展示。

### 新增
- **后端每日自动签到**：`main.py` 新增 scheduler job `daily_checkin`，每天 08:01（Asia/Shanghai，跟随 `timezone` 设置）调 `akile_client.checkin(token)`；签到成功 / 今日已签均 info 日志，异常 warning。先 `ensure_fresh()` 保活 token 再签。
- **手动签到接口**：新增 `POST /api/checkin`（需鉴权），手动触发签到并原样返回 akile 响应（成功 `status_code=0`，已签 `status_code=1`）。
- **AK 币余额展示**：`poller.poll_account` 在抓 `GET /v1/user/info` 后额外调 `get_ak_coin_info(token)`，把 `ak_coin` 写入账户缓存 payload 的 profile 字段（独立降级，失败不影响其余账户数据）。`GET /api/account` 响应的 `profile.ak_coin` 随之返回。
- **签到相关 API 客户端**：`akile_client.py` 新增 `BASE_AI = "https://api.akile.ai/api"` 及 `_ai_headers`，新增 `checkin()`（`GET /v1/user/Checkin`）与 `get_ak_coin_info()`（`GET /v1/user/index`）两方法。签到与 AK 币两端点强制走 akile.ai 主机（akilecloud.com 对此两端点高概率 401）。
- **账户页 AK 币卡片 + 签到按钮**：账户页余额卡旁新增「AK 币」统计卡片（金币图标 + 数值 + 单位）；右上新增「签到」按钮，点击调 `api.checkin()` 并 toast 返回消息，成功后刷新账户数据；依据 `last_checkin_time` 判断今日是否已签，已签则按钮置灰显示「今日已签到」。

## v0.8.0

数据库并发加固 + 二手全量分页 + 我的账户缓存 + 三页排序/列宽 + 评分系统 + 抢购规则化 + 高级筛选。

### 新增
- **抢购规则化（每页独立、每条规则独立次数闸）+ 接入评分排序**：抢购设置改为「一条一条规则」，存于 settings `<page>_buy.rules` JSON 数组。每条规则字段：`tags_include`（标签包含，匹配名称/节点，任一命中即可，如 jppro/jpiij）、`spec`（cpu/内存/硬盘/带宽/流量 的 min/max 区间）、`price_type`（monthly|yearly 二选一互斥）、`price_max`（该周期续费价上限，分）、`sale_price_max`（售价上限，分）、`max_buy`（本规则最多抢几次，命中即计数，达上限停）。
  - 抢购引擎（`buy_engine.py`）：逐规则匹配候选机器 → 命中集合按**评分降序**（复用 `scoring.compute_score`）→ 抢分最高的。**评分与抢购解耦**：评分=排序依据，抢购规则=筛选 + 次数闸。
  - 保留三道配额闸门：单规则 `max_buy` / 单轮合计 `max_total_per_run` / 每日合计 `max_total_daily`；`on_limit_reached` 默认 `keep_watching`（持续补抢）。
  - 前端抢购设置 UI（`BuyRulesEditor`）改为规则列表，可增删每条规则，字段如上；每页独立。
- **三页高级筛选**：云 / 轻量 / 二手三页列表工具栏新增「高级筛选」面板，支持按标签（名称/节点）、配置区间（CPU/内存/硬盘）、价格区间、售价上限、IP 状态、区域筛选。纯前端对已加载快照实时过滤，不重新请求。
- **评分系统（算分 + 显示 + 配置）**：云 / 轻量 / 二手三页各新增「评分」列，分数由后端按各页独立的评分规则加权算好后随快照查询返回。
  - `config.py` `DEFAULTS` 新增 `score_rules_cloud` / `score_rules_ls` / `score_rules_secondhand`（`{维度: 权重}` 映射，默认 cpu 每核 50、内存每 GB 20、硬盘每 GB 1、带宽/流量 0、月付价/售价/延迟为负权重）。`get_all_settings` 合并保存值到默认，缺失维度自动补齐。
  - 新增 `backend/app/scoring.py`，`compute_score(machine, rules)` 计算 `score = Σ(维度数值 × 权重)`；内存按 GB、价格按元换算；`latency`、`sale_price` 无数据时跳过该维度且不报错。
  - `GET /api/machines` 每条结果按其所属页规则附带 `score` 字段（`vps→cloud`、`ls→ls`、`secondhand→secondhand`）。
  - 三页列表新增「评分」列，默认按评分降序、支持点击表头排序；每页顶部新增「评分规则」编辑器（`ScoreRulesEditor`），可独立编辑各维度权重并「恢复默认」。
- **三页表头排序**：名称、地区、CPU、内存、硬盘、带宽、流量、价格、库存列均加 `sorter`。原「配置」合并列拆为 CPU / 内存 / 硬盘 / 带宽 / 流量 五个独立可排序列。
- **列宽可拖拽 + 持久化 + 「恢复默认宽度」**：表头用 `react-resizable` 包裹，宽度变化经 500ms 防抖写入 `POST /api/settings`，按页存入 `col_widths_cloud` / `col_widths_ls` / `col_widths_secondhand`（`{列key: 宽度px}`）；「恢复默认宽度」清空该页覆盖（置 `{}`）回到代码默认。复用现有 settings 读写，不新建表。
- **我的账户页缓存 + 可配置刷新间隔（默认 300s）**：新增 `account_cache` 表（单行 id=1，存账户聚合快照 JSON + `updated_at`），由独立账户轮询任务按间隔刷新，进页面秒开不实时打 API。`poller.poll_account` 抓 `GET /v1/user/info`（余额）与 `POST /v1/server/GetServerV2List`（服务器列表），各自独立降级；`main.py` 新增 `_reschedule_account_job`。配置键 `account_refresh_interval`（默认 300s，设置页加滑块）。新增 `POST /api/account/refresh` 强制刷新。
- **二手列表全量分页抓取**：`poller.poll_secondhand` 翻页（PAGE_SIZE=100，MAX_PAGES 上限，空页/凑满 total 即停）抓全二手在售，`upsert` 单条 try/except 容错，单条异常不影响整批落库。

### 变更
- `GET /api/account` 改为读 `account_cache`（带 `updated_at`），不再每次实时打 akile；首访冷缓存时按需抓取一次填充。

### 修复
- **修复线上 `database is locked`**：SQLite 启用 WAL（读写并发）+ `busy_timeout=30s`（写锁等待而非立即报错）+ 连接 `connect_args timeout` + `_upsert` 用 `no_autoflush`，二手数据可正常并发落库。

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
