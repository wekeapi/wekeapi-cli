#!/usr/bin/env bash
# 一键编译（开源仓库使用）：交叉编译 wekeapi-common-cli 与 wekeapi-msg-sync 共 6 个平台。
# 二进制内置线上环境；产物收集到本仓库 dist/，并同步到同级目录 ../wekeapi-skills/wekeapi-core/assets/bin/。
# 用法：./build.sh
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
dist_dir="$root/dist"
# 技能仓库为独立 git 仓库，约定与本仓库同级克隆；未克隆时只输出 dist/，跳过同步
skill_repo="$(cd "$root/.." && pwd)/wekeapi-skills"
skill_bin="$skill_repo/wekeapi-core/assets/bin"
sync_skill=1

command -v go >/dev/null 2>&1 || { echo "错误：未找到 go，请先安装 Go 1.24+" >&2; exit 2; }

if [[ -d $skill_repo ]]; then
  mkdir -p "$dist_dir" "$skill_bin"
else
  sync_skill=0
  mkdir -p "$dist_dir"
  echo "警告：未找到技能仓库 $skill_repo，跳过二进制同步（只生成 dist/）" >&2
fi

# bash 3.2（macOS 自带）兼容：模块目录/产物前缀 与 GOOS/GOARCH/扩展名 两组三元组
modules=(
  "wekeapi-common-cli/wekeapi"
  "wekeapi-msg-sync/wekeapi-msg-sync"
)
targets=(
  "windows/amd64/.exe"
  "windows/arm64/.exe"
  "linux/amd64/"
  "linux/arm64/"
  "darwin/amd64/"
  "darwin/arm64/"
)

# 纯 Go 依赖，静态二进制；避免交叉编译时链接本机 cgo 工具链
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
for m in "${modules[@]}"; do
  IFS='/' read -r dir prefix <<<"$m"
  cd "$root/$dir"
  for t in "${targets[@]}"; do
    IFS='/' read -r goos goarch ext <<<"$t"
    export GOOS=$goos GOARCH=$goarch
    name="$prefix-$goos-$goarch$ext"
    echo "building $name..."
    go build -trimpath -ldflags '-s -w' -o "$dist_dir/$name" .
    [[ $name == *.exe ]] || chmod +x "$dist_dir/$name"
    # 同步到 skill assets，供 skill 直接调用
    if [[ $sync_skill -eq 1 ]]; then
      cp -f "$dist_dir/$name" "$skill_bin/$name"
      [[ $name == *.exe ]] || chmod +x "$skill_bin/$name"
    fi
  done
done

if [[ $sync_skill -eq 1 ]]; then echo "synced to $skill_bin"; fi
echo "done: $dist_dir"
