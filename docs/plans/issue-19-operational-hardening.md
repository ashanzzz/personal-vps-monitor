# Issue #19 运维可观测性与 Swagger 管理面设计稿

> **目标**：让部署后的 Akile Monitor 具备稳定的 API 管理入口、稳定的日志获取入口，并消除调度器对管理 API 的明显拖慢。
>
> **边界**：本设计只处理“运维/调试/管理面”问题；**不新增独立控制页面**，也**不保留旧的 docs 路由兼容层**。

## 1. 设计背景

当前部署是单镜像形态：`nginx + uvicorn + supervisord`。前端 SPA 负责页面展示，后端 FastAPI 负责 API、调度器、日志和数据持久化。

用户希望：

- 能通过 **Swagger / OpenAPI** 管理 API，而不是另做控制页面。
- 能通过 **API 获取日志**，方便快速判断当前现场。
- 后端在轮询、签到、刷新等后台任务运行时，管理 API 仍然可用。

---

## 2. 当前状态（基线）

### 2.1 路由与部署现状

| 位置 | 当前行为 | 问题 |
|---|---|---|
| `backend/app/main.py` | FastAPI docs 配置为 `/docs`、`/redoc`、`/openapi.json` | 这些路径在真实部署入口上会被 SPA 路由/Fallback 竞争，导致看不到 Swagger 页面 |
| `deploy/nginx.conf` | 仅把 `/api/` 和 `/ws` 转发到后端 | 非 `/api/` 路径会落到前端静态站点 |
| `docs/swagger-api-management.md` | 现有文档仍写 `/docs`、`/redoc`、`/openapi.json` | 与真实部署入口不一致 |
| `README.md` | 也引用 `/docs`、`/redoc` | 同样不适配部署入口 |

### 2.2 日志能力

后端已经有现成的日志尾部 API：

- `GET /api/logs/app?lines=500`
- 实现位置：`backend/app/main.py`
- 读取实现：`backend/app/logging_setup.py::read_log_tail()`

这说明“通过 API 获取日志”**不需要从 0 设计**，缺的是：

- 这个能力要被明确纳入 Swagger 管理面
- 文档要跟部署后的真实路径一致

### 2.3 调度器压力

当前 `lifespan()` 在启动后会立刻安排多个任务：

- 分页轮询：`cloud / ls / secondhand`
- 账户缓存轮询
- token 保活/刷新
- 事件清理
- 每日签到

而且多个 job 采用了“启动即跑”的策略（`next_run_time=datetime.now()`）。

另外，`run_page_poll()` 里还会顺手调用 `poll_price_history()`，导致**每个页面轮询都重复同步价格历史**，把独立的数据同步工作绑在了高频轮询里。

从日志看，已经出现：

- `maximum number of running instances reached (1)`
- `Run time of job ... was missed`

这说明调度器已经开始积压，且需要更明确的并发/节流策略。

---

## 3. 目标设计

### 3.1 Swagger / OpenAPI 作为唯一管理入口

管理面统一放到 API 命名空间中：

- `GET /api/docs`
- `GET /api/redoc`
- `GET /api/openapi.json`

设计原则：

1. **不新增独立控制页面**。
2. **不保留旧的 `/docs` 路径兼容层**。
3. 管理操作继续由现有 API 承载，但人类操作优先通过 Swagger 进行。
4. 现有的 `read-api` / `control-api` / `admin` 分层继续保留，各自职责不变。

> 这样做的原因很简单：`/api/` 在 nginx 已经直接转发给后端，Swagger 放到 `/api/` 下不会再被前端 SPA 的 fallback 吃掉。

### 3.2 日志 API 作为正式运维接口

保留并正式文档化：

- `GET /api/logs/app?lines=500`

约束：

- 默认返回最近 500 行
- `lines` 上限保守限制为 5000
- 该接口必须在 Swagger 里可见、可调用
- 这是“看当前现场”的主入口之一，后续调试不再依赖 shell 进入容器

### 3.3 调度器降压与防重入

把“高频轮询”和“低频同步”拆开，避免一个 job 内做太多事：

- 页面轮询只负责该页面数据刷新与自动购买判断
- 价格历史同步改成独立低频 job
- 启动时不要所有 job 同时立刻跑
- 对同类 job 增加明确的运行中保护，避免重复并发进入

### 3.4 可观测性补强

最少要有三层可观测性：

1. **日志尾部**：`/api/logs/app`
2. **Swagger 管理面**：可查看、可调用 API
3. **调度状态日志**：每个 job 记录开始/结束/跳过原因，方便从日志判断当前是否卡在轮询或刷新上

---

## 4. 架构图

架构图单独保存为：`docs/architecture/issue-19-operational-surface.mmd`

这里先说明数据流：

- 浏览器访问 `/api/docs`、`/api/openapi.json`、`/api/logs/app`
- nginx 只负责把 `/api/*` 和 `/ws` 转给后端
- FastAPI 返回 Swagger、日志尾部和业务 API
- 调度器在同一进程内执行后台轮询任务，但需要降压与防重入

---

## 5. API 合同设计

### 5.1 Swagger / OpenAPI

| 路径 | 语义 | 认证 |
|---|---|---|
| `/api/docs` | Swagger UI | 页面本身可公开，接口调用按各自鉴权 |
| `/api/redoc` | ReDoc | 同上 |
| `/api/openapi.json` | OpenAPI 定义 | 同上 |

### 5.2 日志尾部

| 路径 | 方法 | 参数 | 返回 |
|---|---|---|---|
| `/api/logs/app` | `GET` | `lines`（默认 500，最大 5000） | `{ lines, content }` |

约束：

- 继续使用现有管理员鉴权
- 返回内容保持纯文本尾部，方便复制到聊天/issue 里
- 不要求分页游标；尾部查看即可满足当前调试需求

### 5.3 现有管理 API

现有这些接口的职责不改，只是更容易通过 Swagger 使用：

- `/api/auth/status`
- `/api/auth/login`
- `/api/auth/setup`
- `/api/settings`
- `/api/poll/trigger`
- `/api/store/refresh`
- `/api/token/status`
- `/api/token/refresh`
- `/api/akile-login`
- `/api/read/v1/*`
- `/api/control/v1/*`

---

## 6. 调度器设计

### 6.1 现有问题

当前问题不是“单个 API 接口慢”这么简单，而是：

- job 启动太集中
- 页面轮询之间可能重叠
- 价格历史同步被嵌入高频轮询
- 任务失败和跳过主要靠 APScheduler 的默认警告，缺少业务层面的明确状态

### 6.2 目标策略

#### A. 启动错峰

启动后不再让所有 job 同一时刻立即执行。

建议：

- page poll 之间错开初次运行时间
- account poll、token refresh、prune、daily checkin 也使用错峰首跑
- 初次执行可采用“按页序号偏移 + 随机扰动”的方式

#### B. 运行中保护

每类关键 job 进入前先判断自己是否已在运行。

如果已在运行：

- 不排队
- 不等待
- 直接记一条结构化日志：`poll.skip reason=busy`

这样比“让 APScheduler 无限顶着再报 max instances”更容易分析。

#### C. 价格历史独立同步

把 `GetAllRecyclePackage` 的同步从 `run_page_poll()` 中拆出去，改成低频 job。

建议同步节奏：

- 启动后先跑一次
- 然后按较低频率运行（例如 30~60 分钟）

这样页面轮询不会因为价格历史同步而抖动。

#### D. 明确 job 默认值

建议统一设置：

- `coalesce=True`
- `misfire_grace_time` 保守设置
- `max_instances=1` 只作为最后一道防线，而不是唯一策略

### 6.3 需要保留的行为

以下行为要保留：

- 轮询失败不应把整个进程打死
- 账户轮询和页面轮询都要保留独立退化能力
- 现有 UI 仍然可以从缓存读数据
- 手动触发接口仍然可用

---

## 7. 设计分解（实施顺序）

### Task 1：把 Swagger 移到 `/api/` 命名空间

**目标**：让真实部署入口下可直接打开 Swagger。

**影响文件**：

- `backend/app/main.py`
- `docs/swagger-api-management.md`
- `README.md`

**设计点**：

- `docs_url="/api/docs"`
- `redoc_url="/api/redoc"`
- `openapi_url="/api/openapi.json"`
- 删除/不保留旧根路径文档入口

### Task 2：把日志尾部明确纳入 Swagger 管理面

**目标**：让日志成为标准调试入口。

**影响文件**：

- `backend/app/main.py`
- `docs/swagger-api-management.md`

**设计点**：

- 保留 `GET /api/logs/app`
- 在 Swagger 文档里加说明：`lines` 上限、返回格式、用途

### Task 3：调度器错峰与防重入

**目标**：减少 `max instances reached` 和 missed run。

**影响文件**：

- `backend/app/main.py`
- `backend/app/poller.py`

**设计点**：

- 去掉“所有 job 启动即跑”的集中模式
- 给关键 job 加业务级 busy guard
- 让 page poll / account poll / 维护任务更清晰地分离

### Task 4：把价格历史同步拆成独立 job

**目标**：降低页面轮询的负担。

**影响文件**：

- `backend/app/poller.py`
- `backend/app/main.py`

**设计点**：

- `poll_price_history()` 不再挂在每次 page poll 末尾
- 改成独立低频同步

### Task 5：补齐文档与验收说明

**目标**：让后续实现和运维操作都能从文档直接落地。

**影响文件**：

- `docs/swagger-api-management.md`
- `README.md`
- `docs/architecture/issue-19-operational-surface.mmd`

---

## 8. 验收标准

### 管理面

- `GET /api/docs` 可在真实部署入口直接打开
- `GET /api/openapi.json` 返回 OpenAPI JSON
- Swagger 中可以看到 `admin`、`read-api`、`control-api` 分组

### 日志面

- `GET /api/logs/app?lines=200` 能返回最近日志
- 日志 API 能在 Swagger 中直接调用

### 调度器

- 正常运行时不再持续刷 `maximum number of running instances reached (1)`
- 管理 API 在后台轮询期间仍能稳定响应
- 价格历史同步不再和每次页面轮询绑定在一起

### 文档面

- README 和 Swagger 使用文档都指向 `/api/docs`
- 不再以根路径 `/docs` 作为部署入口说明

---

## 9. 非目标

- 不新增独立“控制页面”
- 不保留旧根路径 docs 兼容层
- 不改现有业务 API 的认证模型
- 不做大规模评分算法重写
- 不引入额外数据库或外部队列

---

## 10. 结论

这个问题的本质不是“页面少了一个入口”，而是：

1. **管理入口没有落到真实部署路径上**
2. **日志虽然存在，但没有被当成正式运维接口**
3. **调度器把后台工作和管理可用性绑得太紧**

所以正确的切法是：

- 先把 Swagger / OpenAPI 移到 `/api/`
- 把日志尾部固定成正式 API
- 再把调度器降压、错峰和拆分

这样后续调试、排障和维护才会顺手。
