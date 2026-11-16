# 个人 vps 监控 (Personal VPS Monitor)

统一管理 **懒猫云**、**AkileCloud**、**ACCK** 多平台个人云服务器账户的监控与自动化中心。支持多平台账户资产看板、每日自动签到、新机与二手机监控告警、以及具备安全防误购保护锁的选购与订单试算模块。

## 核心功能

* **多平台每日自动签到**：
  * **懒猫云**：每日自动签到领取积分，支持设置页账号 + 密码 + 2FA 一键登录并自动保存 Token；
  * **AkileCloud**：每日随机时间自动签到获取 AK 币，打卡时自动调用 `/v1/user/refreshToken` 顺带延长 Token 有效期（实现 Token 永续）；
  * **ACCK**：自动打卡获取 AC 积分与权益，同步拉取积分流水与日志。
* **多平台资产看板**：
  * 聚合展示懒猫云、Akile 与 ACCK 的账户余额、积分、实名状态、会员等级与到期服务器实例。
* **市场监控与告警（Akile / 懒猫云）**：
  * 支持新机 VPS、轻量机、二手机上架/下架/价格变化实时告警，支持 Telegram 推送。
* **安全防误购保护机制（Safe Dry-Run）**：
  * 内置**防误购保护锁**（`safe_buy_mode=True`，`real_purchase_enabled=False`），默认全面拦截所有自动扣费与下单动作，购买相关逻辑仅在模拟模式下运行，杜绝资金损失。
  * **懒猫云实例销毁类接口已永久封禁**：逆向产物、客户端封装、UI 与文档中均不存在销毁入口，运行时另有硬拦截。

## 快速部署

只需要两步：

**1. 创建 `.env` 文件（填入你的 Token）**

```bash
echo "AKILE_TOKEN=your_akile_token" > .env
echo "ACCK_TOKEN=your_acck_token" >> .env
echo "PUBLIC_READ_TOKEN=your_read_only_token_here" >> .env
echo "PUBLIC_CONTROL_TOKEN=your_control_token_here" >> .env
```

**2. 启动**

```bash
docker compose up -d --build
```

访问 `http://你的服务器IP:8080` 即可打开监控页面。

> 默认端口 8080，可通过 `PORT=9090 docker compose up -d` 自定义。

---

## Unraid 部署

两种方式任选其一：

### 方式 1：WebGUI 手动部署（推荐）

1. SSH 到 Unraid 或使用 Web Terminal，运行：
   ```bash
   wget https://raw.githubusercontent.com/ashanzzz/personal-vps-monitor/master/my-akile-monitor.xml \
     -O /boot/config/plugins/dockerMan/templates-user/my-akile-monitor.xml
   ```

2. 打开 Unraid WebGUI → **Docker** 标签页

3. 点击 **Add Container**

4. 在 **Template** 下拉框中选择 **my-akile-monitor**

5. 填入你的 **AKILE_TOKEN**（必填）

6. 调整端口（默认 8080）和数据目录（默认 `/mnt/user/appdata/akile-monitor`）

7. 点击 **Apply** 创建容器

8. 容器启动后访问 `http://你的Unraid-IP:8080`

### 方式 2：一键部署脚本

在 Unraid 主机上以 root 运行：

```bash
curl -fsSL https://raw.githubusercontent.com/ashanzzz/personal-vps-monitor/master/unraid-deploy.sh | bash
```

然后按脚本提示在 WebGUI 中完成容器创建。

---

## GitHub Actions 构建（手动触发）

进入仓库 → **Actions** → **Build and Push Docker Image** → **Run workflow**，填入 tag（默认 `latest`）点击运行。

构建完成后镜像推送到 `ghcr.io/ashanzzz/personal-vps-monitor:latest`。

首次使用需要在 GitHub 仓库 Settings → Packages 中将镜像设为私有（默认继承仓库可见性）。

---

## Swagger / OpenAPI 文档

如果你要直接“管理 API”，优先看这里：

- Swagger UI：`/api/docs`
- ReDoc：`/api/redoc`
- 使用说明：`docs/swagger-api-management.md`

Swagger 已开启授权记忆和请求耗时显示，适合直接在页面里完成：

- 只读 API 查询
- 控制 API 写设置/触发刷新
- 管理员登录后的内部调试

---

## 配置说明

所有配置均可在前端页面「设置」中修改，无需重启容器：

- **Akile Token** — API 鉴权 Token
- **Public Read Token** — 第三方只读 API 密钥，开启 `/api/read/v1`
- **Public Control Token** — 第三方可写控制 API 密钥，开启 `/api/control/v1`
- **轮询间隔** — 新机/轻量机/二手机分别可单独设置
- **监控范围** — 选择关注的机型、区域
- **评分阈值** — 可设置最低评分 / 最低星级，控制推送筛选
- **Telegram 通知** — Bot Token + Chat ID + 各类事件开关
- **自动抢购** — 预留接口，配置目标套餐 ID 和余额下限

---

## 数据持久化

数据存储在 Docker volume `akile-data` 或 Unraid 的 `/mnt/user/appdata/akile-monitor`，包含：

- SQLite 数据库（机器列表、历史记录、配置）
- 轮询状态缓存

---

## 技术栈

- **后端**: FastAPI + SQLite + APScheduler
- **前端**: React + Vite + Ant Design (dark theme)
- **部署**: Docker 单镜像（nginx + uvicorn + supervisord）

---

## 目录结构

```
.
├── backend/          # FastAPI 后端
│   └── app/
│       ├── main.py           # 入口 + APScheduler
│       ├── akile_client.py   # API 客户端
│       ├── poller.py         # 轮询逻辑
│       ├── notify.py         # Telegram 推送
│       ├── database.py       # SQLite ORM
│       └── ws.py             # WebSocket 广播
├── frontend/         # React 前端
│   └── src/
│       ├── pages/            # 总览、监控面板、设置
│       └── components/       # MachineCard、AlertPanel
├── deploy/
│   ├── nginx.conf            # 单镜像 nginx 配置
│   └── supervisord.conf      # 进程管理
├── Dockerfile                # 单镜像多阶段构建
├── docker-compose.yml        # 单服务配置
├── my-akile-monitor.xml      # Unraid Docker 模板
└── .github/workflows/build.yml  # GitHub Actions
```

---

## License

MIT
