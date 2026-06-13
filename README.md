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

## Unraid 部署

两种方式任选其一：

### 方式 1：WebGUI 手动部署（推荐）

1. SSH 到 Unraid 或使用 Web Terminal，运行：
   ```bash
   wget https://raw.githubusercontent.com/ashanzzz/akile-monitor/master/my-akile-monitor.xml \
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
curl -fsSL https://raw.githubusercontent.com/ashanzzz/akile-monitor/master/unraid-deploy.sh | bash
```

然后按脚本提示在 WebGUI 中完成容器创建。

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
