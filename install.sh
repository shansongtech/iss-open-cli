#!/bin/bash

# iss-open-cli 一键安装脚本（GitHub 公网版本）
#
# 从 GitHub Releases 下载预编译二进制，安装到 $HOME/.iss-open-cli，
# 并将其加入当前 shell 的 PATH。
#
# 用法：
#   # 自动安装最新版本
#   curl -fsSL https://raw.githubusercontent.com/shansongtech/iss-open-cli/main/install.sh | bash
#
#   # 安装指定版本
#   curl -fsSL https://raw.githubusercontent.com/shansongtech/iss-open-cli/main/install.sh | bash -s -- 1.0.0
#
#   # 克隆仓库后本地执行
#   bash install.sh [版本号]

set -e

VERSION="$1"

if [[ -n "$VERSION" ]] && [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "用法: $0 [版本号]" >&2
    echo "示例: $0 1.0.0" >&2
    exit 1
fi

# 配置项
GITHUB_REPO="shansongtech/iss-open-cli"
RELEASE_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download"
LATEST_API_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
DOWNLOAD_DIR="$HOME/.iss-open-cli/downloads"
INSTALL_DIR="$HOME/.iss-open-cli"
BINARY_NAME="iss-open-cli"

DOWNLOADER=""
if command -v curl >/dev/null 2>&1; then
    DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
    DOWNLOADER="wget"
else
    echo "需要安装 curl 或 wget，但两者都未找到" >&2
    exit 1
fi

download_file() {
    local url="$1"
    local output="$2"

    if [ "$DOWNLOADER" = "curl" ]; then
        if [ -n "$output" ]; then
            curl -fsSL -o "$output" "$url"
        else
            curl -fsSL "$url"
        fi
    elif [ "$DOWNLOADER" = "wget" ]; then
        if [ -n "$output" ]; then
            wget -q -O "$output" "$url"
        else
            wget -q -O - "$url"
        fi
    else
        return 1
    fi
}

# 从 GitHub API 拉取最新 release 的 tag_name，适配带/不带 v 前缀两种风格
get_latest_version() {
    local json
    json=$(download_file "$LATEST_API_URL") || return 1
    echo "$json" \
        | grep -m1 '"tag_name"' \
        | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/' \
        | sed -E 's/^v//'
}

case "$(uname -s)" in
    Darwin) os="macos" ;;
    Linux) os="linux" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *) echo "不支持的操作系统: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) echo "不支持的架构: $(uname -m)" >&2; exit 1 ;;
esac

# Rosetta 检测：运行在 Apple Silicon 上的 amd64 进程，改用 arm64 二进制
if [ "$os" = "macos" ] && [ "$arch" = "amd64" ]; then
    if [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
        arch="arm64"
    fi
fi

platform="${os}-${arch}"

if [ "$os" = "windows" ]; then
    BINARY_FILE="${BINARY_NAME}-windows-${arch}.exe"
else
    BINARY_FILE="${BINARY_NAME}-${os}-${arch}"
fi

echo "检测到平台: $platform"
echo ""

mkdir -p "$DOWNLOAD_DIR"
mkdir -p "$INSTALL_DIR"

# 未指定版本时，从 GitHub API 拉取最新版本号
if [ -z "$VERSION" ]; then
    echo "正在从 GitHub 获取最新版本..."
    VERSION=$(get_latest_version || true)
    if [ -z "$VERSION" ]; then
        echo "获取最新版本失败，请直接指定版本号，例如: $0 1.0.0" >&2
        echo "版本列表见: https://github.com/${GITHUB_REPO}/releases" >&2
        exit 1
    fi
    if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "从 GitHub 获取的版本号格式无效: $VERSION" >&2
        exit 1
    fi
    echo "最新版本: $VERSION"
fi

echo ""
echo "正在安装 iss-open-cli 版本: $VERSION"
echo ""

DOWNLOAD_URL="${RELEASE_BASE_URL}/${VERSION}/${BINARY_NAME}-${VERSION}-${platform}.tar.gz"

echo "下载地址: $DOWNLOAD_URL"

ARCHIVE_PATH="$DOWNLOAD_DIR/${BINARY_NAME}-${VERSION}-${platform}.tar.gz"
if ! download_file "$DOWNLOAD_URL" "$ARCHIVE_PATH"; then
    echo "下载失败，请检查:" >&2
    echo "  - 版本 '$VERSION' 是否在 https://github.com/${GITHUB_REPO}/releases 存在" >&2
    echo "  - 平台 '$platform' 对应的 tar.gz 是否已发布" >&2
    echo "  - 网络是否能正常访问 github.com（必要时配置代理）" >&2
    rm -f "$ARCHIVE_PATH"
    exit 1
fi

echo "[OK] 下载完成"

TEMP_DIR="$DOWNLOAD_DIR/temp-extract-$$"
mkdir -p "$TEMP_DIR"

if ! tar -xzf "$ARCHIVE_PATH" -C "$TEMP_DIR" 2>/dev/null; then
    echo "解压归档文件失败" >&2
    rm -rf "$TEMP_DIR" "$ARCHIVE_PATH"
    exit 1
fi

echo "[OK] 归档文件已解压"

# 查找解压后的目录。兼容「解压后有一层子目录」和「直接解压到当前层」两种打包方式。
EXTRACTED_DIR=$(find "$TEMP_DIR" -maxdepth 1 -type d ! -path "$TEMP_DIR" | head -n 1)
if [ -z "$EXTRACTED_DIR" ]; then
    # 直接解压到 TEMP_DIR 的情况
    EXTRACTED_DIR="$TEMP_DIR"
fi

echo "正在安装文件到: $INSTALL_DIR"

# 检测是否存在旧版本
IS_UPGRADE=false
if [ -f "$INSTALL_DIR/$BINARY_NAME" ] || [ -f "$INSTALL_DIR/${BINARY_NAME}.exe" ]; then
    IS_UPGRADE=true
    echo "[检测] 发现已安装的旧版本，将执行升级安装"

    OLD_EXECUTABLE=""
    if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
        OLD_EXECUTABLE="$INSTALL_DIR/$BINARY_NAME"
    elif [ -f "$INSTALL_DIR/${BINARY_NAME}.exe" ]; then
        OLD_EXECUTABLE="$INSTALL_DIR/${BINARY_NAME}.exe"
    fi

    if [ -n "$OLD_EXECUTABLE" ]; then
        OLD_VERSION=$("$OLD_EXECUTABLE" --version 2>&1 | head -n 1 || echo "未知版本")
        echo "  当前版本: $OLD_VERSION"
        echo "  新版本: $VERSION"
    fi

    echo ""
fi

# 升级安装：只替换二进制文件，保留 configs/ 和 logs/
if [ "$IS_UPGRADE" = true ]; then
    echo "[升级] 正在替换二进制文件..."

    NEW_BINARY=""
    if [ -f "$EXTRACTED_DIR/$BINARY_NAME" ]; then
        NEW_BINARY="$EXTRACTED_DIR/$BINARY_NAME"
    elif [ -f "$EXTRACTED_DIR/${BINARY_NAME}.exe" ]; then
        NEW_BINARY="$EXTRACTED_DIR/${BINARY_NAME}.exe"
    fi

    if [ -z "$NEW_BINARY" ]; then
        echo "错误: 压缩包中未找到二进制文件" >&2
        rm -rf "$TEMP_DIR" "$ARCHIVE_PATH"
        exit 1
    fi

    if cp -f "$NEW_BINARY" "$INSTALL_DIR/"; then
        echo "  [OK] 二进制文件已更新（保留现有 configs/ 与 logs/）"
    else
        echo "  错误: 替换二进制文件失败" >&2
        rm -rf "$TEMP_DIR" "$ARCHIVE_PATH"
        exit 1
    fi
else
    # 全新安装：复制所有文件
    echo "[安装] 正在复制文件..."

    if cp -r "$EXTRACTED_DIR"/* "$INSTALL_DIR/" 2>/dev/null || cp -r "$EXTRACTED_DIR"/. "$INSTALL_DIR/" 2>/dev/null; then
        echo "  [OK] 文件已复制到安装目录"
    else
        echo "  错误: 复制文件失败" >&2
        rm -rf "$TEMP_DIR" "$ARCHIVE_PATH"
        exit 1
    fi
fi

# 确保可执行文件有执行权限
if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
elif [ -f "$INSTALL_DIR/${BINARY_NAME}.exe" ]; then
    chmod +x "$INSTALL_DIR/${BINARY_NAME}.exe"
fi

# macOS 用户从 GitHub 下载的二进制可能带 quarantine 属性，主动清除避免首次运行弹窗
if [ "$os" = "macos" ] && [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    xattr -d com.apple.quarantine "$INSTALL_DIR/$BINARY_NAME" 2>/dev/null || true
fi

echo "[OK] 安装完成到: $INSTALL_DIR"

# 自动配置 PATH 的函数
configure_path() {
    local install_dir="$1"
    local export_line='export PATH="$PATH:'"$install_dir"'"'

    local current_shell
    current_shell=$(basename "$SHELL")
    local config_file=""

    case "$current_shell" in
        bash)
            if [ -f "$HOME/.bashrc" ]; then
                config_file="$HOME/.bashrc"
            elif [ -f "$HOME/.bash_profile" ]; then
                config_file="$HOME/.bash_profile"
            else
                config_file="$HOME/.bashrc"
            fi
            ;;
        zsh)
            config_file="$HOME/.zshrc"
            ;;
        fish)
            export_line='set -gx PATH $PATH '"$install_dir"
            config_file="$HOME/.config/fish/config.fish"
            mkdir -p "$(dirname "$config_file")"
            ;;
        *)
            return 1
            ;;
    esac

    if [ -f "$config_file" ] && grep -q "$install_dir" "$config_file" 2>/dev/null; then
        return 0
    fi

    echo "[配置] 检测到您使用 $current_shell shell"
    echo "正在自动将 $install_dir 添加到 PATH..."

    echo "" >> "$config_file"
    echo "# iss-open-cli installation" >> "$config_file"
    echo "$export_line" >> "$config_file"
    echo "[OK] 已将 PATH 配置添加到 $config_file"
    echo ""
    echo "[提示] 需要重新加载配置才能生效："
    echo "  方式1: 重新打开终端窗口"
    echo "  方式2: 运行命令: source $config_file"
    return 0
}

rm -rf "$TEMP_DIR" "$ARCHIVE_PATH"

echo ""
echo "正在验证安装..."
EXECUTABLE=""
if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    EXECUTABLE="$INSTALL_DIR/$BINARY_NAME"
elif [ -f "$INSTALL_DIR/${BINARY_NAME}.exe" ]; then
    EXECUTABLE="$INSTALL_DIR/${BINARY_NAME}.exe"
fi

if [ -n "$EXECUTABLE" ] && "$EXECUTABLE" --version >/dev/null 2>&1; then
    INSTALLED_VERSION=$("$EXECUTABLE" --version 2>&1 | head -n 1)
    echo "[OK] 安装成功!"
    echo "  版本: $INSTALLED_VERSION"
    echo "  位置: $INSTALL_DIR"
else
    echo "[警告] 文件已安装，但无法验证版本"
    echo "  位置: $INSTALL_DIR"
    echo "  请手动运行: $INSTALL_DIR/$BINARY_NAME --help"
fi

echo ""
echo "[完成] 安装成功!"
echo ""
echo "下一步："
echo "  1. 编辑配置: $INSTALL_DIR/configs/config.yaml（填入 client_id / app_secret / shop_id）"
echo "  2. 查看帮助: $BINARY_NAME --help"
echo "  3. 查看动作码: $BINARY_NAME --list"
echo ""

# 检查并配置 PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    configure_path "$INSTALL_DIR"
else
    echo "[OK] $INSTALL_DIR 已在 PATH 中"
fi
