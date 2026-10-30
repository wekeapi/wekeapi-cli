# 一键编译（开源仓库使用）：交叉编译 wekeapi-common-cli 与 wekeapi-msg-sync 共 6 个平台。
# 二进制内置线上环境；产物收集到本仓库 dist/，并同步到同级目录 ../wekeapi-skills/wekeapi-core/assets/bin/。
# 用法：.\build.ps1
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$distDir = Join-Path $root 'dist'
# 技能仓库为独立 git 仓库，约定与本仓库同级克隆；未克隆时只输出 dist/，跳过同步
$skillRepo = Join-Path (Split-Path $root -Parent) 'wekeapi-skills'
$skillBin = Join-Path $skillRepo 'wekeapi-core\assets\bin'
$syncSkill = Test-Path $skillRepo

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw '未找到 go，请先安装 Go 1.24+'
}
New-Item -ItemType Directory -Force -Path $distDir | Out-Null
if ($syncSkill) {
    New-Item -ItemType Directory -Force -Path $skillBin | Out-Null
}
else {
    Write-Warning "未找到技能仓库 $skillRepo，跳过二进制同步（只生成 dist/）"
}

# 同步到 skill assets：刚构建的二进制可能被防病毒/索引短暂占用（“用户映射区域”错误），失败时重试。
function Copy-WithRetry([string]$from, [string]$to) {
    for ($i = 1; $i -le 8; $i++) {
        try {
            [System.IO.File]::Copy($from, $to, $true)
            return
        }
        catch {
            if ($i -eq 8) { throw }
            Start-Sleep -Milliseconds 500
        }
    }
}

# 模块与产物前缀：产物命名为 <前缀>-<os>-<arch>[.exe]
$modules = @(
    @{ Dir = 'wekeapi-common-cli'; Prefix = 'wekeapi' },
    @{ Dir = 'wekeapi-msg-sync'; Prefix = 'wekeapi-msg-sync' }
)

$targets = @(
    @{ GOOS = 'windows'; GOARCH = 'amd64'; Ext = '.exe' },
    @{ GOOS = 'windows'; GOARCH = 'arm64'; Ext = '.exe' },
    @{ GOOS = 'linux';   GOARCH = 'amd64'; Ext = '' },
    @{ GOOS = 'linux';   GOARCH = 'arm64'; Ext = '' },
    @{ GOOS = 'darwin';  GOARCH = 'amd64'; Ext = '' },
    @{ GOOS = 'darwin';  GOARCH = 'arm64'; Ext = '' }
)

$oldGOOS = $env:GOOS
$oldGOARCH = $env:GOARCH
$oldCGO = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'  # 纯 Go 依赖，静态二进制，交叉编译时无需本机 cgo 工具链
    foreach ($m in $modules) {
        Push-Location (Join-Path $root $m.Dir)
        try {
            foreach ($t in $targets) {
                $env:GOOS = $t.GOOS
                $env:GOARCH = $t.GOARCH
                $out = "$($m.Prefix)-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
                Write-Host "building $out..."
                go build -trimpath -ldflags '-s -w' -o (Join-Path $distDir $out) .
                if ($LASTEXITCODE -ne 0) { throw "build failed: $out" }
                if ($syncSkill) {
                    Copy-WithRetry (Join-Path $distDir $out) (Join-Path $skillBin $out)
                }
            }
        }
        finally {
            Pop-Location
        }
    }
}
finally {
    $env:GOOS = $oldGOOS
    $env:GOARCH = $oldGOARCH
    $env:CGO_ENABLED = $oldCGO
}

if ($syncSkill) { Write-Host "synced to $skillBin" }
Write-Host "done: $distDir"
