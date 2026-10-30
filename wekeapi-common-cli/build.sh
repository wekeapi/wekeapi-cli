#!/usr/bin/env bash
# wekeapi CLI 构建脚本（macOS/Linux）：交叉编译 6 个平台二进制（内置线上环境）。
# 编译完成后同步到同级技能仓库 ../wekeapi-skills/wekeapi-core/assets/bin/。
# 用法：./build.sh
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "$root/.." && pwd)
out_dir="$root/dist"
# 技能仓库为独立 git 仓库，与源码仓库同级；未克隆时跳过同步
skill_repo="$(cd "$repo_root/.." && pwd)/wekeapi-skills"
skill_bin="$skill_repo/wekeapi-core/assets/bin"
sync_skill=1

usage() {
  cat <<'EOF'
用法：./build.sh [-h]
  交叉编译 6 个平台二进制到 dist/，并同步到同级技能仓库 ../wekeapi-skills/wekeapi-core/assets/bin/
  -h, --help             显示本帮助
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    *) echo "错误：未知参数 $1" >&2; usage >&2; exit 2 ;;
  esac
done

command -v go >/dev/null 2>&1 || { echo "错误：未找到 go，请先安装 Go 1.24+" >&2; exit 2; }

# bash 3.2（macOS 自带）兼容：不用关联数组，用 GOOS/GOARCH/输出名 三元组
targets=(
  "windows/amd64/wekeapi-windows-amd64.exe"
  "windows/arm64/wekeapi-windows-arm64.exe"
  "linux/amd64/wekeapi-linux-amd64"
  "linux/arm64/wekeapi-linux-arm64"
  "darwin/amd64/wekeapi-darwin-amd64"
  "darwin/arm64/wekeapi-darwin-arm64"
)

mkdir -p "$out_dir"
if [[ -d $skill_repo ]]; then
  mkdir -p "$skill_bin"
else
  sync_skill=0
  echo "警告：未找到技能仓库 $skill_repo，跳过二进制同步（只生成 dist/）" >&2
fi

# 静态二进制，无依赖；避免交叉编译时链接本机 cgo 工具链
old_goos=${GOOS-}
old_goarch=${GOARCH-}
old_cgo=${CGO_ENABLED-}
restore_env() {
  if [[ -n $old_goos ]]; then export GOOS=$old_goos; else unset GOOS; fi
  if [[ -n $old_goarch ]]; then export GOARCH=$old_goarch; else unset GOARCH; fi
  if [[ -n $old_cgo ]]; then export CGO_ENABLED=$old_cgo; else unset CGO_ENABLED; fi
}
trap restore_env EXIT

export CGO_ENABLED=0
for t in "${targets[@]}"; do
  IFS='/' read -r goos goarch name <<<"$t"
  export GOOS=$goos GOARCH=$goarch
  echo "building $name..."
  go build -trimpath -ldflags '-s -w' -o "$out_dir/$name" .
  [[ $name == *.exe ]] || chmod +x "$out_dir/$name"
  # 同步到 skill assets，供 skill 直接调用
  if [[ $sync_skill -eq 1 ]]; then
    cp -f "$out_dir/$name" "$skill_bin/$name"
    [[ $name == *.exe ]] || chmod +x "$skill_bin/$name"
  fi
done

if [[ $sync_skill -eq 1 ]]; then echo "synced to $skill_bin"; fi
echo "done: $out_dir"
