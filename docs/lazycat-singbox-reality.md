# 懒猫云实例 sing-box 部署记录

实例：11433 `global-2-NJR3An`（0.1折 全球家宽 大杯02）
日期：2026-11-16

## 硬约束：这台机器 NAT 只放行 TCP

请求 UDP 映射被平台直接拒绝：

```json
{"error":"Host NAT protocol is TCP, requested UDP is not allowed","code":"HOST_NAT_PROTOCOL_MISMATCH","hostNatProtocol":"tcp"}
```

结论：**Hysteria2 / TUIC 这类 QUIC(UDP) 协议在这台机器上装不了**，不是配置问题，
是平台网络层不给你 UDP。只能选 TCP 系协议（VLESS Reality / Trojan / SS / AnyTLS）。

## 网络拓扑（实测，别被「香港」二字误导）

| 位置 | 归属 |
|------|------|
| 入口 167.104.96.167:45433 | 香港 HostHatch，AS43969 Prinode AB，hosting=true |
| 实际出口 | 美国 Illinois Champaign，Pavlov Media 住宅宽带，hosting=false proxy=false |

即「香港中转 + 美国家宽出口」。出口是真实住宅 IP（非机房、非已知代理段），
这是它对 ChatGPT / Netflix 友好的根本原因，也是这台机器的核心价值。
RTT 122ms（天津 → 香港入口）。

## 已装配置

- 包：`apk add sing-box` 主仓库自带 1.12.12，**不需要加 edge 源**
- 服务：OpenRC，`rc-update add sing-box default` 已开机自启
- 入站：VLESS + Reality，:`:443`，flow=xtls-rprx-vision，SNI www.apple.com
- 端口映射：TCP 45433 → 内网 443（mapping id 47523）

## BBR

**本来就已开启，无需操作**：`net.ipv4.tcp_congestion_control = bbr`，
`tcp_available_congestion_control = reno cubic bbr`。LXC 共享宿主机内核，
宿主机已加载 tcp_bbr。另外 HY2 走 QUIC 自带 BBR 系拥塞控制，本来也不依赖内核参数。

## 分享链接

```
vless://bbe1e489-a372-47f1-8f65-41aa93118bd5@167.104.96.167:45433?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.apple.com&fp=chrome&pbk=fSA2tLImc06SvOdAubf2tI2q_w9J9F9cuatYzkMXhUg&sid=c653e997&type=tcp#懒猫云-HK-Reality
```

## mihomo / Clash Meta 片段

```yaml
proxies:
  - name: "懒猫云-HK-Reality"
    type: vless
    server: 167.104.96.167
    port: 45433
    uuid: bbe1e489-a372-47f1-8f65-41aa93118bd5
    network: tcp
    udp: true
    tls: true
    flow: xtls-rprx-vision
    servername: www.apple.com
    client-fingerprint: chrome
    reality-opts:
      public-key: fSA2tLImc06SvOdAubf2tI2q_w9J9F9cuatYzkMXhUg
      short-id: c653e997
  - name: "懒猫云-HK-SS2022"
    type: ss
    server: 167.104.96.167
    port: 55966
    cipher: 2022-blake3-aes-128-gcm
    password: "4iHVwPPj/ENy6UvVulkOHA=="
    udp: true
```

proxy-groups 里把两个节点都放进去，用 url-test 自动选快的：

```yaml
proxy-groups:
  - name: "PROXY"
    type: url-test
    url: "http://www.gstatic.com/generate_204"
    interval: 300
    tolerance: 50
    proxies: ["懒猫云-HK-Reality", "懒猫云-HK-SS2022"]
```

注意：原版 Clash Premium 内核不认 Reality，必须 mihomo 系（Clash Verge Rev /
Mihomo Party / FlClash / OpenClash）。

## 运维命令（经 WebSocket 终端执行）

```
ws_url = wss://dash.lazycat.wiki/api/ws/instances/{id}/terminal?token=<jwt>
```

- `rc-service sing-box status|restart`
- `sing-box check -c /etc/sing-box/config.json`
- 日志：`tail -f /var/log/sing-box.log`

前端的 `GET /api/lazycat/instances/{id}/terminal` 会返回该 ws 地址，
详情抽屉里的「打开终端」直接跳官方 xterm.js 页面。

## 第二个协议：SS2022 兜底（2026-11-16 追加）

Reality 在个别网络会被 QoE 限速或识别，加一条 Shadowsocks 2022 走另一个端口兜底。

- 入站：`:8443`，`2022-blake3-aes-128-gcm`（该算法自带防重放，不要用老 SS 的 aes-256-gcm）
- 端口映射：TCP **55966 → 8443**（mapping id 47529）
- 实测出口同样是美国 Pavlov Media 住宅 IP，`chatgpt.com` 200 / `google.com` 200 / `youtube.com` 200
- **速度比 Reality 低一个数量级**（单线程约 0.6 MB/s vs 3.2 MB/s）：SS 没有多路复用、
  也没有 BBR 类拥塞控制，跨境高延迟链路上很吃亏。所以它是兜底，不是主力。

分享链接：

```
ss://2022-blake3-aes-128-gcm:4iHVwPPj/ENy6UvVulkOHA==@167.104.96.167:55966#懒猫云-HK-SS2022
```

配置变更前已备份：`/etc/sing-box/config.json.bak.<timestamp>`。

## 两个节点怎么选

| 场景 | 用哪个 |
|------|--------|
| 日常看视频、下大文件 | Reality（3.2 MB/s） |
| Reality 被限速/握手失败 | SS2022 兜底 |
| 客户端不支持 Reality | SS2022 |
