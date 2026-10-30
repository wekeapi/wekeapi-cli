# wekeapi-cli

wekeapi-cli 是 wekeapi - 微信协议 API 的**命令行工具**。它基于 wekeapi 提供微信连接能力，包含登录/通讯录/群聊/消息/朋友圈等功能，以及消息同步能力。它可以通过 Agent Skill 提供给 Agent / Harness 工具使用。

## 组成

wekeapi-cli 包含两个可独立构建的 Go module：

| 目录 | Go module | 产物 | 作用 |
| --- | --- | --- | --- |
| `wekeapi-common-cli/` | `wekeapi-cli` | `wekeapi-<os>-<arch>[.exe]` | 命令行客户端，覆盖登录、通讯录、群聊、标签、消息、朋友圈、代理等接口，stdout 恒为 JSON，适合脚本与 Agent 调用 |
| `wekeapi-msg-sync/` | `wekeapi-msg-sync` | `wekeapi-msg-sync-<os>-<arch>[.exe]` | 数据同步服务：接收消息回调、增量同步联系人、消息落库（SQLite / MySQL），并提供 REST 查询 API |

## 与 wekeapi-skills 仓库的关系

本仓库是 wekeapi-cli 工具的源码。另一个仓库 [wekeapi-skills](https://github.com/wekeapi/wekeapi-skills.git) 是 Agent 技能包，其中 `wekeapi-core/assets/bin/` 下的二进制可执行文件由本仓库编译产出。

如果你只是想尝试在 Harness 平台通过 AI 使用微信交互能力，可以直接使用 [wekeapi-skills](https://github.com/wekeapi/wekeapi-skills.git)。

## 目录结构

```
.
├── wekeapi-common-cli/   # 主 CLI（Go module：wekeapi-cli）
│   ├── cmd/              # cobra 子命令（login/contact/chatroom/label/message/sns/proxy/data…）
│   ├── internal/         # client / env / msgservice / runner
│   └── test/             # 端到端测试（编译二进制并以子进程运行）
├── wekeapi-msg-sync/     # 数据同步服务（Go module：wekeapi-msg-sync）
│   ├── internal/         # callback / client / config / server / store / syncer
│   ├── test/             # 黑盒与集成测试
│   └── wekeapi-msg-sync.example.json
└── build.ps1 / build.sh  # 一键编译（Windows / macOS / Linux）
```

## 环境要求

- Go 1.24+
- SQLite（纯 Go 驱动无需安装）；
- 如果使用 MySQL 需安装 5.7+ / 8.0+

## 一键编译

交叉编译两个模块的 6 个平台二进制（windows / linux / darwin × amd64 / arm64），产物收集到 `dist/`：

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

```bash
# macOS / Linux
./build.sh
```

## 单独编译模块

每个模块目录下也有独立的构建脚本，只编译自己：

```bash
# 主 CLI：交叉编译 6 平台到 wekeapi-common-cli/dist/
cd wekeapi-common-cli
go build -o wekeapi .        # 只编当前平台，产物落在当前目录
./build.sh                   # macOS/Linux 交叉编译（Windows 用 .\build.ps1）

# 数据同步服务：交叉编译 6 平台到 wekeapi-msg-sync/dist/
cd ../wekeapi-msg-sync
go build -o wekeapi-msg-sync .
./build.sh
```

## 开始使用

```bash
# 1. 初始化
# 访问凭证 AccessToken 在 wekeapi官网 https://www.wekeapi.com/) 申请，也可以用 wekeapi env 查询 siteURL
cd wekeapi-common-cli
go build -o wekeapi .
./wekeapi env                       # 查看内置环境信息
./wekeapi init --token <AccessToken>

# 2. 扫码登录（首次不传 --app-id，系统自动生成；--out 保存二维码图片）
./wekeapi login qr --device-type Mac --verify-type auto \
  --proxy-type local --proxy-info <代理ID> --out qr.jpg
./wekeapi login check --qr-code-uuid <UUID>      # 每 5 秒轮询一次

# 3. 登录成功后 CLI 会自动写入数据同步服务配置并拉起服务，之后用 data 命令查询
./wekeapi data status
./wekeapi data contacts --keyword 张三
```

手动部署数据同步服务：

```bash
cd wekeapi-msg-sync
cp wekeapi-msg-sync.example.json wekeapi-msg-sync.json   # 填写 token / appId / wxid
./wekeapi-msg-sync run --config ./wekeapi-msg-sync.json --port 3006
```

然后在 wekeapi 平台把回调地址绑定为 `http://<公网可达地址>:3006/callback`，公网地址可以使用 ngrok 等三方工具映射。


## 文档索引

| 文档 | 内容 |
| --- | --- |
| [wekeapi-common-cli/README.md](wekeapi-common-cli/README.md) | 主 CLI 的全部命令、全局参数、退出码与场景示例 |
| [wekeapi-msg-sync/README.md](wekeapi-msg-sync/README.md) | 数据同步服务的配置项、回调接入、REST API 与表结构 |

## Contact

有问题请提 issue 并 @KingpleDev
