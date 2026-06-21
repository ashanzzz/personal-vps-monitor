# Swagger / OpenAPI API 管理说明

本项目已经开启 FastAPI 自带的 Swagger UI，可直接用浏览器管理接口。

## 入口

- Swagger UI：`http://<服务器IP>:8080/docs`
- ReDoc：`http://<服务器IP>:8080/redoc`
- OpenAPI JSON：`http://<服务器IP>:8080/openapi.json`

> 本项目已开启 `persistAuthorization` 和 `displayRequestDuration`，
> 也就是：授权会尽量保留，接口耗时会直接显示在 Swagger 页面里。

## 先看懂三种鉴权

Swagger 里一共会看到 3 个可授权项：

| 名称 | 用途 | 需要填什么 | 对应环境变量 |
|---|---|---|---|
| `AdminBearer` | 管理员接口 | **只填 JWT 本体**，不要带 `Bearer ` 前缀 | 通过 `/api/auth/login` 或 `/api/auth/setup` 获取 |
| `PublicReadToken` | 第三方只读 API | 只读密钥本体 | `PUBLIC_READ_TOKEN` |
| `PublicControlToken` | 第三方控制 API | 控制密钥本体 | `PUBLIC_CONTROL_TOKEN` |

## 建议的使用顺序

### 1. 先登录拿管理员 JWT

先调用：

- `POST /api/auth/login`
- 或首次初始化时 `POST /api/auth/setup`

成功后，响应里会返回 `token`。

然后在 Swagger 顶部点 **Authorize**：

- 找到 `AdminBearer`
- 粘贴 JWT 本体
- 不要手动加 `Bearer ` 前缀

### 2. 如需给第三方读数据

使用 `PublicReadToken`：

- `GET /api/read/v1/meta`
- `GET /api/read/v1/health`
- `GET /api/read/v1/machines`
- `GET /api/read/v1/machines/{category}`

### 3. 如需第三方自动化控制

使用 `PublicControlToken`：

- `GET /api/control/v1/meta`
- `GET /api/control/v1/health`
- `GET /api/control/v1/settings`
- `POST /api/control/v1/settings`
- `POST /api/control/v1/poll/trigger`
- `POST /api/control/v1/store/refresh`
- `POST /api/control/v1/token/refresh`
- `GET /api/control/v1/token/status`
- `GET /api/control/v1/account`
- `POST /api/control/v1/account/refresh`

## 按 API 类型管理

### A. 管理员接口

这组是内部管理接口，常见功能包括：

- 设置读写：`/api/settings`
- 机器列表：`/api/machines`
- 事件日志：`/api/events`
- 价格历史：`/api/price-history`
- Token 状态与刷新：`/api/token/status`、`/api/token/refresh`
- 账户缓存：`/api/account`、`/api/account/refresh`
- 强制轮询：`/api/poll/trigger`
- 手动签到：`/api/checkin`
- 手动购买：`/api/buy/{plan_id}`

### B. 只读 API

适合给第三方系统、脚本、外部面板直接读取：

- `GET /api/read/v1/meta`
- `GET /api/read/v1/health`
- `GET /api/read/v1/machines`
- `GET /api/read/v1/machines/{category}`

常用查询参数：

| 参数 | 作用 |
|---|---|
| `category` | 分类：`vps` / `ls` / `secondhand` / `cloud` |
| `available_only` | 只看可用机器 |
| `min_score` | 最低评分 |
| `min_star` | 最低星级 |
| `ip_status` | 过滤 IP 状态 |
| `sort` | 排序：`score_desc` / `price_asc` / `star_desc` |

示例：

```text
GET /api/read/v1/machines?available_only=true&min_score=70&sort=score_desc
```

### C. 控制 API

这组是给自动化程序用的“写接口”，可以直接在 Swagger 里做管理动作，不需要单独做控制页面。

- 改设置
- 触发轮询
- 触发单页刷新
- 触发 Token 刷新
- 刷新账户缓存

## 一个最常见的操作流程

### 改最低评分阈值

1. 打开 `/docs`
2. `Authorize` → 填 `PublicControlToken`
3. 找到 `POST /api/control/v1/settings`
4. 发送类似下面的 JSON：

```json
{
  "filter_min_score": 80,
  "filter_min_star": 4
}
```

### 手动刷新一次全量数据

1. `Authorize` → 填 `PublicControlToken`
2. 调用 `POST /api/control/v1/poll/trigger`

### 查看当前 Token 情况

1. `Authorize` → 填 `PublicControlToken`
2. 调用 `GET /api/control/v1/token/status`

## 注意事项

- 只读密钥和控制密钥不要混用。
- `AdminBearer` 只能管理内部接口，不建议拿去给第三方。
- Swagger 只是 API 管理入口，不替代前端页面；但**控制类操作不需要专门的控制页面**，Swagger 已经够用。
- 如果某个接口返回 `401`，通常是授权没带对；如果返回 `503`，通常是对应环境变量没配置。

## 相关文件

- `backend/app/main.py`
- `backend/app/auth.py`
- `backend/app/public_api.py`
- `backend/app/control_api.py`
- `README.md`
