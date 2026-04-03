#!/bin/bash
set -euo pipefail

NM_HOST_NAME="com.cookie.bridge"
BINARY_ABS="$(readlink -f "${1:-cookie-cli}" 2>/dev/null || echo "$(pwd)/${1:-cookie-cli}")"

if [ ! -f "$BINARY_ABS" ]; then
    echo "错误: 找不到 $BINARY_ABS，请先运行 make build"
    exit 1
fi

if [ -d /mnt/c/Windows ]; then
    # WSL2 环境
    WIN_PROFILE=$(/mnt/c/Windows/System32/cmd.exe /c "echo %USERPROFILE%" 2>/dev/null | tr -d '\r\n')
    # C:\Users\WPS -> /mnt/c/Users/WPS
    WIN_HOME=$(echo "$WIN_PROFILE" | sed 's|\\|/|g; s|^\([A-Za-z]\):|/mnt/\L\1|')
    # C:\Users\WPS -> C:\\Users\\WPS (for JSON)
    WIN_PROFILE_ESC=$(echo "$WIN_PROFILE" | sed 's|\\|\\\\|g')

    NM_DIR="$WIN_HOME/.cookie/native-messaging"
    EXT_DIR="$WIN_HOME/cookie-bridge-extension"

    echo "检测到 WSL2 环境"
    echo "  Windows 家目录: $WIN_HOME"
    echo "  Binary: $BINARY_ABS"

    mkdir -p "$NM_DIR"

    # 创建 .bat 启动器
    BAT_FILE="$NM_DIR/$NM_HOST_NAME.bat"
    printf '@echo off\r\nwsl.exe -- "%s" native-messaging-host\r\n' "$BINARY_ABS" > "$BAT_FILE"

    # 转换路径为 Windows 格式
    WIN_BAT=$(wslpath -w "$BAT_FILE" | sed 's|\\|\\\\|g')

    # 尝试从已安装的扩展获取 ID
    EXT_ID="EXTENSION_ID_HERE"
    if [ -f "$EXT_DIR/manifest.json" ]; then
        # Chrome 不会在 manifest.json 里放 id，扩展 ID 需要从 chrome://extensions 获取
        :
    fi

    # 创建 NM manifest
    JSON_FILE="$NM_DIR/$NM_HOST_NAME.json"
    cat > "$JSON_FILE" <<NMEOF
{
  "name": "$NM_HOST_NAME",
  "description": "Cookie Bridge Native Messaging Host",
  "path": "$WIN_BAT",
  "type": "stdio",
  "allowed_origins": ["chrome-extension://$EXT_ID/"]
}
NMEOF

    # 注册到 Windows 注册表
    WIN_JSON=$(wslpath -w "$JSON_FILE")
    /mnt/c/Windows/System32/reg.exe ADD \
        "HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\$NM_HOST_NAME" \
        /ve /t REG_SZ /d "$WIN_JSON" /f

    echo ""
    echo "Native Messaging Host 已注册"
    echo "  Manifest: $JSON_FILE"
    echo "  Launcher: $BAT_FILE"
    echo ""
    echo "重要: 请编辑 $JSON_FILE"
    echo "将 EXTENSION_ID_HERE 替换为你的扩展 ID（在 chrome://extensions 查看）"
else
    # 原生 Linux 环境
    NM_DIR="$HOME/.config/google-chrome/NativeMessagingHosts"
    mkdir -p "$NM_DIR"

    JSON_FILE="$NM_DIR/$NM_HOST_NAME.json"
    cat > "$JSON_FILE" <<NMEOF
{
  "name": "$NM_HOST_NAME",
  "description": "Cookie Bridge Native Messaging Host",
  "path": "$BINARY_ABS",
  "type": "stdio",
  "allowed_origins": ["chrome-extension://EXTENSION_ID_HERE/"]
}
NMEOF

    echo "Native Messaging Host 已注册: $JSON_FILE"
    echo "请编辑该文件，将 EXTENSION_ID_HERE 替换为你的扩展 ID"
fi
