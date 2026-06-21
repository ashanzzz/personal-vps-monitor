#!/bin/bash
# Unraid 部署脚本 — akile-monitor 单镜像
# 用法：在 Unraid 主机上以 root 权限运行此脚本

set -e

TEMPLATE_URL="https://raw.githubusercontent.com/ashanzzz/akile-monitor/master/my-akile-monitor.xml"
TEMPLATE_DIR="/boot/config/plugins/dockerMan/templates-user"
TEMPLATE_FILE="$TEMPLATE_DIR/my-akile-monitor.xml"

echo "=== Akile Monitor Unraid 部署 ==="
echo ""

# 1. 下载模板
echo "[1/3] 下载 Docker 模板..."
mkdir -p "$TEMPLATE_DIR"
curl -fsSL "$TEMPLATE_URL" -o "$TEMPLATE_FILE"
echo "✓ 模板已保存到: $TEMPLATE_FILE"
echo ""

# 2. 创建数据目录
echo "[2/3] 创建数据目录..."
mkdir -p /mnt/user/appdata/akile-monitor
chmod 755 /mnt/user/appdata/akile-monitor
echo "✓ 数据目录: /mnt/user/appdata/akile-monitor"
echo ""

# 3. 提示用户操作
echo "[3/3] 下一步操作："
echo ""
echo "  1. 打开 Unraid WebGUI → Docker 标签页"
echo "  2. 点击「Add Container」"
echo "  3. 在「Template」下拉框中选择「my-akile-monitor」"
echo "  4. 填入你的 AKILE_TOKEN（必填）"
echo "  5. 如需第三方只读 API，再填 PUBLIC_READ_TOKEN（可选）"
echo "  6. 调整端口（默认 8080）和其他配置（可选）"
echo "  7. 点击「Apply」创建容器"
echo ""
echo "  容器启动后访问: http://你的Unraid-IP:8080"
echo ""
echo "=== 部署脚本执行完毕 ==="
