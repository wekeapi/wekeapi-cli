# wekeapi-msg-sync 构建脚本：交叉编译 6 个平台二进制到 dist/。
# 编译完成后同步到同级技能仓库 ../wekeapi-skills/wekeapi-core/assets/bin/，供主 CLI 自动发现并启动。
# 用法：.\build.ps1
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$outDir = Join-Path $root 'dist'
# 技能仓库为独立 git 仓库，与源码仓库同级；未克隆时跳过同步
$skillRepo = Join-Path (Split-Path (Split-Path $root -Parent) -Parent) 'wekeapi-skills'
$skillBin = Join-Path $skillRepo 'wekeapi-core\assets\bin'
$syncSkill = Test-Path $skillRepo
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
if ($syncSkill) {
    New-Item -ItemType Directory -Force -Path $skillBin | Out-Null
}
else {
    Write-Warning "未找到技能仓库 $skillRepo，跳过二进制同步（只生成 dist/）"
}

# 同步到 skill assets：刚构建的二进制可能被防病毒/索引短暂占用（“用户映射区域”错误），失败时重试。
function Copy-ToSkills([string]$from, [string]$to) {
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

$targets = @(
    @{ GOOS = 'windows'; GOARCH = 'amd64'; Out = 'wekeapi-msg-sync-windows-amd64.exe' },
    @{ GOOS = 'windows'; GOARCH = 'arm64'; Out = 'wekeapi-msg-sync-windows-arm64.exe' },
    @{ GOOS = 'linux';   GOARCH = 'amd64'; Out = 'wekeapi-msg-sync-linux-amd64' },
    @{ GOOS = 'linux';   GOARCH = 'arm64'; Out = 'wekeapi-msg-sync-linux-arm64' },
    @{ GOOS = 'darwin';  GOARCH = 'amd64'; Out = 'wekeapi-msg-sync-darwin-amd64' },
    @{ GOOS = 'darwin';  GOARCH = 'arm64'; Out = 'wekeapi-msg-sync-darwin-arm64' }
)

Push-Location $root
$oldGOOS = $env:GOOS
$oldGOARCH = $env:GOARCH
$oldCGO = $env:CGO_ENABLED
try {
    # 纯 Go SQLite 驱动，静态二进制，无需本机 cgo 工具链
    $env:CGO_ENABLED = '0'
    foreach ($t in $targets) {
        $env:GOOS = $t.GOOS
        $env:GOARCH = $t.GOARCH
        Write-Host "building $($t.Out)..."
        go build -trimpath -ldflags '-s -w' -o (Join-Path $outDir $t.Out) .
        if ($LASTEXITCODE -ne 0) { throw "build failed: $($t.Out)" }
        # 同步到 skill assets，供主 CLI 自动发现并启动
        if ($syncSkill) {
            Copy-ToSkills (Join-Path $outDir $t.Out) (Join-Path $skillBin $t.Out)
        }
    }
}
finally {
    $env:GOOS = $oldGOOS
    $env:GOARCH = $oldGOARCH
    $env:CGO_ENABLED = $oldCGO
    Pop-Location
}

Write-Host "done: $outDir"
if ($syncSkill) { Write-Host "synced to $skillBin" }
