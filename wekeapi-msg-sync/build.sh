#!/usr/bin/env bash
# wekeapi-msg-sync 构建脚本（macOS/Linux）：交叉编译 6 个平台二进制到 dist/。
# 编译完成后同步到同级技能仓库 ../wekeapi-skills/wekeapi-core/assets/bin/，供主 CLI 自动发现并启动。
# 用法：./build.sh
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
out_dir="$root/dist"
# 技能仓库为独立 git 仓库，与源码仓库同级；未克隆时跳过同步
skill_repo="$(cd "$root/../.." && pwd)/wekeapi-skills"
skill_bin="$skill_repo/wekeapi-core/assets/bin"
sync_skill=1
mkdir -p "$out_dir"
if [[ -d $skill_repo ]]; then
  mkdir -p "$skill_bin"
else
  sync_skill=0
  echo "警告：未找到技能仓库 $skill_repo，跳过二进制同步（只生成 dist/）" >&2
fi

command -v go >/dev/null 2>&1 || { echo "错误：未找到 go，请先安装 Go 1.24+" >&2; exit 2; }

# bash 3.2（macOS 自带）兼容：GOOS/GOARCH/输出名 三元组
targets=(
  "windows/amd64/wekeapi-msg-sync-windows-amd64.exe"
  "windows/arm64/wekeapi-msg-sync-windows-arm64.exe"
  "linux/amd64/wekeapi-msg-sync-linux-amd64"
  "linux/arm64/wekeapi-msg-sync-linux-arm64"
  "darwin/amd64/wekeapi-msg-sync-darwin-amd64"
  "darwin/arm64/wekeapi-msg-sync-darwin-arm64"
)

old_goos=${GOOS-}
old_goarch=${GOARCH-}
old_cgo=${CGO_ENABLED-}
restore_env() {
  if [[ -n $old_goos ]]; then export GOOS=$old_goos; else unset GOOS; fi
  if [[ -n $old_goarch ]]; then export GOARCH=$old_goarch; else unset GOARCH; fi
  if [[ -n $old_cgo ]]; then export CGO_ENABLED=$old_cgo; else unset CGO_ENABLED; fi
}
trap restore_env EXIT

# 纯 Go SQLite 驱动，静态二进制，无需本机 cgo 工具链
export CGO_ENABLED=0
cd "$root"
for t in "${targets[@]}"; do
  IFS='/' read -r goos goarch name <<<"$t"
  export GOOS=$goos GOARCH=$goarch
  echo "building $name..."
  go build -trimpath -ldflags '-s -w' -o "$out_dir/$name" .
  [[ $name == *.exe ]] || chmod +x "$out_dir/$name"
  # 同步到 skill assets，供主 CLI 自动发现并启动
  if [[ $sync_skill -eq 1 ]]; then
    cp "$out_dir/$name" "$skill_bin/$name"
  fi
done

echo "done: $out_dir"
if [[ $sync_skill -eq 1 ]]; then echo "synced to $skill_bin"; fi
