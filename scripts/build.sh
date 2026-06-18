#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BINARY_NAME="agent-insight"
OUTPUT_DIR="${PROJECT_DIR}/dist"

VERSION="${1:-dev}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

LDFLAGS="-s -w -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT} -X main.buildTime=${BUILD_TIME}"

echo "Building ${BINARY_NAME} ${VERSION}..."

mkdir -p "${OUTPUT_DIR}"

# 默认构建当前平台
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

echo "  Platform: ${GOOS}/${GOARCH}"
CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
  -ldflags "${LDFLAGS}" \
  -o "${OUTPUT_DIR}/${BINARY_NAME}-${GOOS}-${GOARCH}" \
  ./cmd/agent-insight/

echo "Built: ${OUTPUT_DIR}/${BINARY_NAME}-${GOOS}-${GOARCH}"
