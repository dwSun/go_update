#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
OUT="$ROOT/dist"
NAME=go_update

mkdir -p "$OUT"
rm -f "$OUT"/*

build() {
	local goos=$1 goarch=$2 ext=$3
	local out="$OUT/${NAME}-${goos}-${goarch}${ext}"
	echo ">> ${goos}/${goarch} -> ${out}"
	GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -ldflags="-s -w" -o "$out" "$ROOT"
}

build windows amd64 .exe
build linux   arm64 ""
build linux   amd64 ""

if ! command -v upx >/dev/null; then
	echo "错误: 未找到 upx，请先安装 (pacman -S upx)" >&2
	exit 1
fi

echo
echo ">> UPX 压缩"
upx --best -q "$OUT"/*

echo
echo "完成，输出目录: $OUT"
ls -lh "$OUT"
