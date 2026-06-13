# Akile Monitor

一键部署的 AkileCloud 监控工具，支持新机 VPS、轻量机、二手机上架/下架/价格变化实时告警，内置 Telegram 推送，预留自动抢购接口。

## 快速部署

只需要两步：

**1. 创建 `.env` 文件（填入你的 Token）**

```bash
echo "AKILE_TOKEN=your_token_here" > .env
```

**2. 启动**

```bash
docker compose up -d
```

访问 `http://你的服务器IP:8080` 即可打开监控页面。

> 默认端口 8080，可通过 `PORT=9090 docker compose up -d` 自定义。

---

## GitHub Actions 构建（手动触发）

进入仓库 → **Actions** → **Build and Push Docker Image** → **Run workflow**，填入 tag（默认 `latest`）点击运行。

构建完成后镜像推送到 `ghcr.io/ashanzzz/akile-monitor:latest`。

首次使用需要在 GitHub 仓库 Settings → Packages 中将镜像设为私有（默认继承仓库可见性）。

---

## 配置说明

所有配置均可在前端页面「设置」中修改，无需重启容器：

- **Akile Token** — API 鉴权 Token
- **轮询间隔** — 新机/轻量机/二手机分别可单独设置
- **监控范围** — 选择关注的机型、区域
- **Telegram 通知** — Bot Token + Chat ID + 各类事件开关
- **自动抢购** — 预留接口，配置目标套餐 ID 和余额下限

---

## 数据持久化

SQLite 数据库挂载在 `akile-data` volume，重建容器数据不丢失。

## 架构

单容器：nginx（前端静态 + 反代 `/api` 和 `/ws`）+ uvicorn（FastAPI 后端）+ APScheduler（轮询任务），由 supervisord 管理进程。
