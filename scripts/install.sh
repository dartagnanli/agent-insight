#!/usr/bin/env bash
set -euo pipefail

# agent-insight 安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/dartagnanli/agent-insight/main/scripts/install.sh | bash

REPO="dartagnanli/agent-insight"
BINARY_NAME="agent-insight"
INSTALL_DIR="${HOME}/.local/bin"

# 检测平台
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: ${ARCH}" >&2; exit 1 ;;
esac

# 获取最新版本
VERSION="${1:-latest}"
if [ "${VERSION}" = "latest" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/' || echo "0.1.0")"
fi

echo "Installing ${BINARY_NAME} v${VERSION} for ${OS}/${ARCH}..."

# 下载
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${BINARY_NAME}-${VERSION}-${OS}-${ARCH}.tar.gz"
TMPDIR="$(mktemp -d)"
curl -fsSL "${URL}" | tar -xz -C "${TMPDIR}"

# 安装
mkdir -p "${INSTALL_DIR}"
mv "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
rm -rf "${TMPDIR}"

# 检查 PATH
if ! echo "${PATH}" | grep -q "${INSTALL_DIR}"; then
  SHELL_RC=""
  case "$(basename "${SHELL:-bash}")" in
    zsh)  SHELL_RC="${HOME}/.zshrc" ;;
    fish) SHELL_RC="${HOME}/.config/fish/config.fish" ;;
    bash) SHELL_RC="${HOME}/.bashrc" ;;
    *)    SHELL_RC="${HOME}/.profile" ;;
  esac

  FISH_LINE="set -gx PATH ${INSTALL_DIR} \$PATH"
  SH_LINE="export PATH=\"${INSTALL_DIR}:\$PATH\""

  echo ""
  echo "请将 ${INSTALL_DIR} 添加到 PATH："
  if [ "$(basename "${SHELL:-bash}")" = "fish" ]; then
    echo "  echo '${FISH_LINE}' >> ${SHELL_RC}"
  else
    echo "  echo '${SH_LINE}' >> ${SHELL_RC}"
    echo "  source ${SHELL_RC}"
  fi
fi

echo ""
echo "${BINARY_NAME} v${VERSION} 已安装到 ${INSTALL_DIR}/${BINARY_NAME}"
echo "运行 'agent-insight version' 验证安装"
