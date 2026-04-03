#!/bin/bash
set -euo pipefail

NM_HOST_NAME="com.cookie.bridge"

if [ -d /mnt/c/Windows ]; then
    WIN_HOME=$(/mnt/c/Windows/System32/cmd.exe /c "echo %USERPROFILE%" 2>/dev/null | tr -d '\r\n' | sed 's|\\|/|g; s|^\([A-Za-z]\):|/mnt/\L\1|')
    NM_DIR="$WIN_HOME/.cookie/native-messaging"

    /mnt/c/Windows/System32/reg.exe DELETE \
        "HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\$NM_HOST_NAME" \
        /f 2>/dev/null || true
    rm -rf "$NM_DIR"
    echo "Windows Native Messaging Host 已移除"
else
    NM_DIR="$HOME/.config/google-chrome/NativeMessagingHosts"
    rm -f "$NM_DIR/$NM_HOST_NAME.json"
    echo "Linux Native Messaging Host 已移除"
fi
