# HKG-3（懒猫云 10674）接入交接说明

最后更新：2026-11-28

## 1. 目标

把 `10674`（HKG-3，`216.236.60.76`，Alpine/OpenRC，无 Docker，`ssh_port=0`）变成一条可选线路机，
贯通四层：宿机原生 sing-box → `:3338` 控制面编排 → `vpn-probe` 纳管/自愈 → 五维订阅与拓扑分组。

## 2. 代码状态（均已合入 master）

### personal-vps-monitor

| 提交 | 内容 |
| --- | --- |
| `ee0d3ad` | `pkg/core/sshexec/singbox_native.go`：原生 sing-box 渲染/安装/健康探测/NAT 端口重发布（含回滚） |
| `09d2f85` | `POST /api/lazycat/ports/republish`：按 (私网端口, 旧公网端口) 反查映射，先增后删，失败回滚 |
| `34ea85e` | `pkg/core/sshexec/singbox_deploy.go`：部署编排 + `POST /api/lazycat/singbox/deploy`、`GET /api/lazycat/singbox/health` |

关键设计：
- 宿机不装 Docker，单二进制 `sing-box 1.11.15` + `/etc/init.d/sing-box`（OpenRC）。
- 远程脚本只用 BusyBox sh + `wget`/`tar`/`rc-service`，禁用 `python3`/`docker`/`jq`/`sed -i`。
- 配置由 Go 侧整份渲染后 base64 传输，避免 heredoc 与空格敏感替换。
- NAT 实例换端口策略：**私网端口不动**，只做 `PortMapper.AddPortMapping` 新映射 + 成功后删旧映射。

### vpn-probe

| 提交 | 内容 |
| --- | --- |
| `64ce247` | hub 通道、native 真实存活校验、NAT 自愈、HK 分组/拓扑/Tab 关键字 |

关键设计：
- `internal/vpsmgr/exec_remote.go`：`:3338` HubClient（token 缓存 + 401 重登），命令执行与端口重发布共用。
- `NeedsHub()`：懒猫云且 `SSHPort<=0` 时走 hub，不新开 SSH 端口。
- `detector` native 分支改用 `rc-service`/`systemctl` 真实校验（注意 `inactive` 不能用 Contains 判 `active`）。
- `auto_healer`：固定出口 NAT 走公网映射重发布，宿机进程与配置零改动。
- 纳管口径：`lazycat` 仅跳过 `category != "固定出口"`。

## 3. 已完成实机验证

- `10674` 上 `sing-box 1.11.15` 已安装并运行，Reality VLESS 入站监听 `30021`。
- 服务端配置经交叉验证一致：UUID / public_key（由 private_key 用 X25519 推导比对）/ short_id / SNI 全部匹配。
- 同机回环自测（127.0.0.1:30021 同套密钥）返回 `HTTP=204`，**证明服务端配置与 REALITY 组合本身正确**。
- `216.236.60.76:30021` 从外部 TCP 握手可达（境内与海外探针均能建连）。

## 4. 当前阻塞

从 `vpn-probe` 探测该节点失败：TCP 可达，但 REALITY 协商失败（服务端日志
`REALITY: processed invalid connection`，客户端 `EOF`）。

进一步实测：在 HKG-3 宿机上用同套密钥拨自己的公网地址 `216.236.60.76:30021`，
返回 `dial tcp 216.236.60.76:30021: i/o timeout` —— **该 NAT 映射回环不通**。

结论：服务端无问题，问题在 `216.236.60.76:30021` 这条 NAT 映射的转发质量，
需要商家侧确认放通情况，或换新映射端口重试（可直接调 `POST /api/lazycat/ports/republish`）。

## 5. 另一处结构性缺失

`vpn-probe` 的 `config.yaml` 当前**没有 `vps_hub` 段**，因此
`/api/vps/manage` 长期返回 `count: 0`，hub 纳管从未真正生效。
补配置形如：

```yaml
vps_hub:
  url: http://<personal-vps-monitor 地址>:3338
  username: <admin>
  password: <admin 密码>
```

补完后重启 `vpn-probe` 容器，`/api/vps/manage` 应出现 `10674`（`exec_mode=hub`, `ssh_port=0`, `is_nat=true`）。

## 6. 待办

1. 打通 `30021` 映射（找商家或 `ports/republish` 换端口），再跑一次 `POST /api/probe` 验收。
2. 补 `vps_hub` 配置，确认 `10674` 出现在 `/api/vps/manage`。
3. 节点命名必须为 `HK-HKG3-NAT` 一类，**不得含 `LZ-`**（会被判为家宽）。
4. 验收通过后触发五维同步，确认节点进入 `ashan-transit`（自建-线路机）。

## 7. 操作方式

- 部署：`POST /api/lazycat/singbox/deploy`，body `{instance_id, listen_port, sni, remark}`。
- 健康：`GET /api/lazycat/singbox/health?instance_id=10674`。
- 换公网端口：`POST /api/lazycat/ports/republish`，body `{instance_id, private_port, old_public_port, remark}`。
- 均为 `Authorization: Bearer <token>`，token 由 `POST /api/auth/login` 获取。

注意：`/api/auth/login` 连续失败会按来源 IP 锁定约 13 分钟，调试时不要反复试错。
