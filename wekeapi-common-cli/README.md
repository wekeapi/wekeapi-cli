# wekeapi-cli 使用文档

wekeapi（微信协议 API）的命令行客户端。覆盖官方全部 101 个接口，按模块组织为 8 组子命令（含 `data` 数据查询与服务管理），全部参数通过命令行传入，stdout 输出 JSON，适合脚本与大模型 Skill 调用。

## 目录

- [wekeapi-cli 使用文档](#wekeapi-cli-使用文档)
  - [目录](#目录)
  - [构建与安装](#构建与安装)
  - [全局约定](#全局约定)
    - [全局参数（所有命令可用）](#全局参数所有命令可用)
    - [初始化配置](#初始化配置)
    - [输出与退出码](#输出与退出码)
    - [参数便利规则](#参数便利规则)
  - [快速上手：登录流程](#快速上手登录流程)
  - [命令参考](#命令参考)
    - [login 登录](#login-登录)
    - [contact 通讯录](#contact-通讯录)
    - [chatroom 群聊](#chatroom-群聊)
    - [label 标签](#label-标签)
    - [message 消息](#message-消息)
    - [sns 朋友圈](#sns-朋友圈)
    - [proxy 代理](#proxy-代理)
    - [data 数据查询与服务管理](#data-数据查询与服务管理)
  - [常用场景示例](#常用场景示例)

## 构建与安装

```bash
# 本机构建
go build -o wekeapi .

# 交叉编译（静态二进制，无依赖）
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o wekeapi-linux-amd64 .
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o wekeapi-darwin-arm64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o wekeapi-windows-amd64.exe .
```

Windows 下推荐用构建脚本（交叉编译 6 平台到 `dist\`）：

```powershell
.\build.ps1
```

macOS/Linux 用 `build.sh`，行为一致：

```bash
./build.sh
```

安装：把产物放到 `PATH` 中的任意目录（如 `/usr/local/bin/wekeapi`、`%USERPROFILE%\bin`）。

## 全局约定

### 全局参数（所有命令可用）

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--token` | 访问凭证 AccessToken（**必填**，映射到 `X-Access-Token` 请求头；可 `init` 保存后免传） | — |
| `--app-id` | 设备 appId（除 `login qr` 外的接口均需要，自动写入 body 的 `appId` 字段；可 `init` 保存后免传） | — |
| `--timeout` | HTTP 超时（大文件上传建议调大，如 `--timeout 10m`） | `60s` |
| `--pretty` | 格式化（缩进）输出 JSON | 关闭 |
| `--data` | 直接指定请求 JSON body：传 JSON 字符串或 `@文件路径`（Windows 建议 `@文件`，引号易被 shell 剥离）；与命令自带参数合并，同名字段以命令参数为准；传完整接口响应（含 `code`/`data`）时自动取其中的 `data` 作 body | — |
| `--msg-sync-dir` | `wekeapi-msg-sync` 服务目录（含配置、日志、PID 与 SQLite）；只有 `data`/`login`/`message` 等需要读写服务的命令实际使用 | 环境变量 `WEKEAPI_MSG_SYNC_HOME` > 配置 `msgSyncDir` > `~/.wekeapi/msg-sync` |

### 初始化配置

```bash
# 交互向导（推荐，终端直接运行）：提示输入 AccessToken（回车用默认/保留当前值）
wekeapi init

# 非交互方式（脚本/skill 自动化，stdin 非终端时不提示）：全部用 flag 传入
wekeapi init --token <TOKEN> [--app-id <APP_ID>][--msg-sync-dir <服务目录>]

# 保存后，所有命令自动读取配置，可免传 --token/--app-id/--base-url（显式 flag 优先于配置文件）
wekeapi contact list

# 登录成功获得 appId 后，建议用返回的 appId 更新配置
wekeapi init --token <TOKEN> --app-id <APP_ID>
```

- AccessToken 请到官网 https://www.wekeapi.com/ 申请
- `--app-id` 省略时保留配置中已有值；显式清空用 `--app-id=`（等号空值语法）。**不要写 `--app-id ""`**
- `--msg-sync-dir` 可选：指定 `wekeapi-msg-sync` 服务目录（含 `wekeapi-msg-sync.json` 与可执行文件）；省略时使用默认目录 `~/.wekeapi/msg-sync`。

### 输出与退出码

- **stdout**：仅输出 API 返回的原始 JSON（`{"code":0,"message":"OK","data":{...}}`）。
- **stderr**：错误信息与提示。
- **退出码**：
  - `0` — 成功（API 返回 `code == 0`）
  - `1` — API 业务错误（`code != 0`，stdout 仍输出原始 JSON）
  - `2` — 本地错误（缺参数、网络失败、文件读写失败等）

### 参数便利规则

- **列表参数**：逗号分隔，如 `--wxids wxid_a,wxid_b`。
- **媒体文件**：发送图片/视频/文件/语音时，`--file <本地路径>`（自动 base64）与 `--url <http(s)地址>` 二选一。
- **XML 参数**：转发/下载类接口的 `--xml '<msg>...</msg>'`（自动 base64）与 `--xml-file <路径>` 二选一。
- **文件保存**：下载类命令与二维码类命令支持 `--out <路径>`，自动将结果保存为本地文件（stdout 仍输出原始 JSON）。
- **批量保存**：`sns download-image` 返回多张图片地址，用 `--out-dir <目录>` 按序保存为 `1.jpg`、`2.png` …（目录不存在会自动创建）。
- **复杂字段**：个别数组/对象字段可直接用 `--data '{"key":[...]}'` 透传；Windows PowerShell 下请改用 `--data @文件路径`（PowerShell 会剥离参数中的内嵌双引号，直接写 JSON 会解析失败）。
- **显式空值**：用 `--flag=`（等号后为空），如 `--app-id=`、`--base-url=`；**不要写 `--flag ""`**（Windows PowerShell 5.1 会丢弃空字符串参数，使该 flag 吞掉紧随其后的 flag；CLI 对以 `-` 开头的 appId 值会直接报本地错误并给出修复提示）。

## 快速上手：登录流程

```bash
# 1. 获取登录二维码（首次不传 --app-id，系统自动生成；配置中已有 appId 时会复用它，
#    如需按全新设备登录加 --new-device，等价 --app-id=；--out 保存二维码图片）
wekeapi login qr --token <TOKEN> \
  --device-type Mac --verify-type auto \
  --proxy-type local --proxy-info <代理ID> \
  --out qr.jpg
# => 记录返回的 appId 和 qrCodeUUID；再次获取二维码务必带上同一个 --app-id

# 2. 每 5 秒轮询登录状态（status: 0-未扫码 1-已扫码 2-已登录 3-失败 10-需验证 11-待手机确认）
wekeapi login check --token <TOKEN> --app-id <APP_ID> --qr-code-uuid <UUID>

# 3. 登录成功后：CLI 自动记录 appId/wxid，并把凭证同步到 wekeapi-msg-sync.json、
#    确保数据同步服务运行（已运行且凭证无变化时不重启；重同步用 wekeapi data sync）
wekeapi data status

# 4. 后续接口都带 --app-id；查询数据用 data 命令
wekeapi data contacts --keyword <昵称/备注/微信号>

# 日常巡检在线状态（1-在线 -1-离线 0-未登录；默认不自动重连，确需重连传 --auto-connect）
# 在线且本地还没有服务配置时，会像登录成功后一样创建 wekeapi-msg-sync.json 并启动服务
# （交互终端先询问数据库类型，默认 SQLite）
wekeapi login status --token <TOKEN> --app-id <APP_ID>

# 退出后再次登录（免扫码，手机确认即可）
wekeapi login quick --token <TOKEN> --app-id <APP_ID>

# 主动退出
wekeapi login logout --token <TOKEN> --app-id <APP_ID>
```

## 命令参考

> 除 `login qr` 外所有命令都需要 `--app-id`。表中"参数"列省略全局参数；**加粗**为必填。完整说明可随时用 `wekeapi <模块> <命令> --help` 查看。

### login 登录

| 命令 | 说明 | 参数 |
|---|---|---|
| `login qr` | 获取登录二维码 | **`--device-type`** Mac/iPad、**`--verify-type`** manual/auto、**`--proxy-type`** local/self/platform、**`--proxy-info`**；`--new-device` 按全新设备取码（忽略配置 appId，等价 `--app-id=`）；`--out` 保存二维码图片 |
| `login check` | 检测登录状态 | **`--qr-code-uuid`** |
| `login status` | 检查在线状态 | `--auto-connect` 掉线时是否自动重连（可选，默认不重连） |
| `login quick` | 快捷登录（免扫码） | — |
| `login logout` | 退出登录 | — |

### contact 通讯录

| 命令 | 说明 | 参数 |
|---|---|---|
| `contact list` | 获取通讯录列表（好友/群/公众号 ID） | `--contact-type` |
| `contact brief` | 批量获取简要信息 | **`--wxids`** |
| `contact detail` | 批量获取详细信息 | **`--wxids`** |
| `contact sync` | 强制同步通讯录到平台 | — |
| `contact search` | 搜索联系人 | **`--contact-info`** 微信号/手机号 |
| `contact add-friend` | 主动添加好友 | **`--scene`**、**`--v3`**、**`--v4`**、**`--verify-content`**；`--chatroom-id` |
| `contact accept-friend` | 同意好友申请 | **`--msg-base64`**（取自回调消息） |
| `contact delete-friend` | 删除好友 | **`--wxid`** |
| `contact set-remark` | 设置好友备注 | **`--wxid`**、**`--remark`** |
| `contact set-chat-only` | 设置好友仅聊天 | **`--wxid`**、**`--only-chat`** |
| `contact wework-list` | 获取企微好友列表 | — |
| `contact wework-info` | 批量获取企微好友信息 | **`--wxids`** |
| `contact wework-search` | 搜索企微联系人 | **`--contact-info`** |
| `contact wework-add` | 添加企微好友 | **`--wxid`**、**`--v1`**；`--veryfy-content` |
| `contact wework-delete` | 删除企微好友 | **`--wxid`** |
| `contact wework-set-remark` | 设置企微好友备注 | **`--wxid`**、**`--remark`** |
| `contact phone-list` | 获取手机通讯录里的微信好友 | — |
| `contact phone-upload` | 上传手机通讯录 | **`--op-code`** 1-上传/2-删除、**`--mobiles`** |

### chatroom 群聊

| 命令 | 说明 | 参数 |
|---|---|---|
| `chatroom create` | 创建微信群 | **`--wxids`**（至少 2 个） |
| `chatroom members` | 获取群成员列表 | **`--chatroom-id`** |
| `chatroom member-detail` | 获取群成员详情 | **`--chatroom-id`**、**`--member-wxids`** |
| `chatroom add-friend` | 添加群成员为好友 | **`--chatroom-id`**、**`--member-wxid`**、**`--verify-content`** |
| `chatroom invite` | 邀请好友进群 | **`--chatroom-id`**、**`--wxids`** |
| `chatroom add-member` | 添加好友进群 | **`--chatroom-id`**、**`--wxids`** |
| `chatroom scan-join` | 扫码进群 | **`--qr-code-info`** |
| `chatroom accept-join` | 同意进群 | **`--url`**（回调消息中的 URL） |
| `chatroom approve-invite` | 同意邀请进群申请（群主/管理员） | **`--msg-base64`**、**`--new-msg-id`** |
| `chatroom qrcode` | 获取群二维码 | **`--chatroom-id`**；`--out` 保存图片 |
| `chatroom transfer-owner` | 转让群主（仅群主） | **`--chatroom-id`**、**`--wxid`** |
| `chatroom add-admin` | 添加管理员（仅群主，最多 3 个） | **`--chatroom-id`**、**`--wxids`** |
| `chatroom remove-admin` | 移除管理员（仅群主） | **`--chatroom-id`**、**`--wxids`** |
| `chatroom set-announcement` | 设置群公告 | **`--chatroom-id`**、**`--content`** |
| `chatroom get-announcement` | 获取群公告 | **`--chatroom-id`** |
| `chatroom set-invite-confirm` | 开启/关闭邀请确认 | **`--chatroom-id`**、**`--invite-verify-set`** 1/0 |
| `chatroom rename` | 修改群名称 | **`--chatroom-id`**、**`--chatroom-name`** |
| `chatroom set-remark` | 修改群备注 | **`--chatroom-id`**、**`--chatroom-remark`** |
| `chatroom set-my-nickname` | 修改我在群内昵称 | **`--chatroom-id`**、**`--nick-name-in-chatroom`** |
| `chatroom pat` | 群拍一拍（对方 5 秒后收到） | **`--chatroom-id`**、**`--to-wxid`** |
| `chatroom remove-member` | 移出群成员（群主/管理员） | **`--chatroom-id`**、**`--wxids`** |
| `chatroom dissolve` | 解散群聊（仅群主） | **`--chatroom-id`** |
| `chatroom quit` | 退出群聊 | **`--chatroom-id`** |
| `chatroom save` | 保存群到通讯录 | **`--chatroom-id`** |
| `chatroom unsave` | 从通讯录移除群 | **`--chatroom-id`** |

### label 标签

| 命令 | 说明 | 参数 |
|---|---|---|
| `label list` | 获取标签列表（手机端增删需调用此接口同步） | — |
| `label add` | 添加标签 | **`--labels`** 名称列表 |
| `label delete` | 删除标签 | **`--label-id`** |
| `label set-friend` | 修改好友标签（需提交全部标签） | **`--wxid`**、**`--label-ids`** |

### message 消息

**发送**

| 命令 | 说明 | 参数 |
|---|---|---|
| `message send-text` | 发送文本 | **`--to-wxid`**、**`--content`**；`--ats`（群聊@，全部填 `all`） |
| `message send-voice` | 发送语音（wav/SILK，60 秒内） | **`--to-wxid`**、**`--file`/`--url`** |
| `message send-image` | 发送图片（≤20MB，jpg/png/bmp/webp/tiff） | **`--to-wxid`**、**`--file`/`--url`** |
| `message send-video` | 发送视频（≤200MB） | **`--to-wxid`**、**`--file`/`--url`** |
| `message send-file` | 发送文件（≤200MB） | **`--to-wxid`**、**`--file`/`--url`**、**`--file-name`** |
| `message send-link` | 发送链接 | **`--to-wxid`**、**`--title`**、**`--link-url`**、**`--file`/`--url`**（缩略图 ≤5MB）；`--desc` |
| `message send-miniapp` | 发送小程序（用接收消息的 xml） | **`--to-wxid`**、**`--xml`/`--xml-file`** |
| `message send-emoji` | 发送 Emoji | **`--to-wxid`**、**`--emoji-md5`**、**`--emoji-size`** |
| `message send-card` | 发送名片 | **`--to-wxid`**、**`--card-wxid`**、**`--card-nickname`**；`--card-alias` |
| `message send-appmsg` | 发送 AppMsg（视频号/聊天记录/音乐等） | **`--to-wxid`**、**`--xml`/`--xml-file`**；提供 ori* 参数则按转发发送 |
| `message revoke` | 撤回消息 | **`--to-wxid`**、**`--new-msg-id`**、**`--create-time`** |

**转发**（同一内容发多人时：先发送给第一人，再用转发接口，效率更高）

| 命令 | 数据来源 | 关键参数 |
|---|---|---|
| `message forward-image` | 发送图片接口的响应 | **`--to-wxid`** + CDN 字段（`--file-id` `--file-md5` `--aes-key` 尺寸等）+ ori 字段 |
| `message forward-image-msg` | 接收图片消息的 xml | **`--to-wxid`**、**`--xml`/`--xml-file`**；ori 字段可选 |
| `message forward-video` | 发送视频接口的响应 | **`--to-wxid`** + CDN 字段 + ori 字段 |
| `message forward-video-msg` | 接收视频消息的 xml | **`--to-wxid`**、**`--xml`/`--xml-file`** |
| `message forward-file` | 发送文件接口的响应 | **`--to-wxid`**、**`--file-name`** + CDN 字段 + ori 字段 |
| `message forward-file-msg` | 接收文件消息的 xml | **`--to-wxid`**、**`--xml`/`--xml-file`** |
| `message forward-link` | 发送链接接口的响应 | **`--to-wxid`**、**`--title`**、**`--link-url`** + CDN 字段 + ori 字段 |
| `message forward-link-msg` | 接收链接消息的 xml | **`--to-wxid`**、**`--xml`/`--xml-file`** |
| `message forward-miniapp` | 接收小程序消息的 xml | **`--to-wxid`**、**`--xml`/`--xml-file`** + ori 字段 |

> 转发命令参数较多，可用 `--data @resp.json` 直接透传发送接口的响应字段再补充 `--to-wxid`（具名 flag 优先）。各字段含义见 `wekeapi message forward-image --help`。

> **发送即落库**：`send-*` / `forward-*` 成功后，CLI 会自动把消息写入运行中的 `wekeapi-msg-sync`（`from` 取配置的 wxid，`to`/`new_msg_id`/`create_time` 取发送响应，`msg_type` 按命令映射），因此 `data messages` 可查到本人发出的消息（`from` = 本人 wxid）。服务未配置/未运行时仅跳过上报；上报报错只在 stderr 打 `warn:`，不影响 stdout JSON 与退出码。

**下载**（均支持 `--out` 保存文件）

| 命令 | 说明 | 参数 |
|---|---|---|
| `message download-voice` | 下载语音 | **`--new-msg-id`**、**`--xml`/`--xml-file`**、**`--format`** 1-SILK/2-WAV、**`--from-wxid`** |
| `message download-image` | 下载图片 | **`--type`** 1-原图/2-常规/3-缩略图、**`--xml`/`--xml-file`** |
| `message download-video` | 下载视频 | **`--type`** 1-原视频/2-常规/3-缩略图、**`--xml`/`--xml-file`** |
| `message download-file` | 下载文件（xml 中 type=6 的消息） | **`--xml`/`--xml-file`** |
| `message download-emoji` | 下载 Emoji（URL 7 天有效） | **`--xml`/`--xml-file`** |
| `message download-cdn` | 用 FileId/AesKey 下载（无 xml 时） | **`--type`** 1-高清图/2-常规图/3-缩略图/4-视频/5-文件、**`--file-id`**、**`--aes-key`**、**`--file-size`** |

### sns 朋友圈

命令组名 `sns`，（对应接口前缀 `/v1/sns/`）。

- **snsId（朋友圈 Id）**：发布接口返回的 `data.snsId`，或查看类接口 `snsItemList[].id`；点赞、评论、删除、改可见范围、下载都用它。
- **snsWxid（发布人 wxid）**：点赞、评论必填，取列表项的 `wxid` 字段。
- **visibility（可见范围）**：`1`-公开 `2`-私密 `3`-部分可见 `4`-不给谁看；`3` 必须给 `--allow-tag-ids` 或 `--allow-wxids`，`4` 必须给 `--disallow-tag-ids` 或 `--disallow-wxids`。
- **分页**：首次不传分页参数，第 2 页起传上次响应 `data` 中的 `firstPageMd5`、`fromId`。

**查看**

| 命令 | 说明 | 参数 |
|---|---|---|
| `sns list` | 自己的朋友圈时间线 | `--first-page-md5`、`--from-id` |
| `sns friend-list` | 某个好友的朋友圈列表 | **`--wxid`**；`--first-page-md5`、`--from-id` |
| `sns detail` | 某条朋友圈详情 | **`--sns-id`** |

**发布**（返回 `data.snsId` 与 `data.createTime`）

| 命令 | 说明 | 参数 |
|---|---|---|
| `sns send-text` | 纯文本朋友圈 | **`--content`**、**`--visibility`** |
| `sns send-image` | 图文朋友圈（≤9 张，单张 ≤5MB） | **`--images`** 本地路径（逗号分隔，自动 base64）/ **`--image-urls`**、**`--visibility`**；`--content` |
| `sns send-video` | 视频朋友圈（≤200MB；Mac ≤30 秒） | **`--file`/`--url`**、**`--visibility`**；`--content` |
| `sns send-link` | 链接朋友圈（缩略图 ≤5MB） | **`--title`**、**`--link-url`**、**`--file`/`--url`**、**`--visibility`**；`--content` |

> 发布类可选：`--allow-tag-ids`、`--allow-wxids`、`--disallow-tag-ids`、`--disallow-wxids`、`--with-wxids`（提醒谁看）、`--location`（位置 JSON，含 `poiName`/`city`/`latitude`/`longitude`/`poiClassifyId`/`poiClassifyType`）。

**转发到朋友圈**（均需 **`--visibility`**，可选 `--content` 与可见范围细节）

| 命令 | 数据来源 | 参数 |
|---|---|---|
| `sns forward-link` | 链接朋友圈的 xmlMsg | **`--xml`/`--xml-file`** |
| `sns forward-link-msg` | 链接类型聊天消息的 xmlMsg | **`--xml`/`--xml-file`** |
| `sns forward-finder` | 视频号聊天消息/视频号朋友圈的 xmlMsg | **`--xml`/`--xml-file`** |

**管理与互动**

| 命令 | 说明 | 参数 |
|---|---|---|
| `sns set-visibility` | 修改已发布朋友圈的可见范围（Mac 不支持） | **`--sns-id`**、**`--visibility`** + 可见范围细节 |
| `sns set-private` | 设为/取消私密（Mac 不支持） | **`--sns-id`**、**`--is-private=true/false`** |
| `sns delete` | 删除某条朋友圈 | **`--sns-id`** |
| `sns like` | 点赞 | **`--sns-id`**、**`--sns-wxid`** |
| `sns unlike` | 取消点赞 | **`--sns-id`** |
| `sns comment` | 发布评论（可带图、可回复他人） | **`--sns-id`**、**`--sns-wxid`**；`--content` 与 `--file`/`--url` 至少一个；回复时 `--reply-comment-id/--reply-wxid/--reply-nickname/--reply-create-time` 四项一起给 |
| `sns delete-comment` | 删除评论 | **`--sns-id`**、**`--comment-id`** |

**下载**（下载地址 7 天有效）

| 命令 | 说明 | 参数 |
|---|---|---|
| `sns download-image` | 图文朋友圈的图片地址列表；`--out-dir` 批量保存 | **`--xml`/`--xml-file`**；`--out-dir` |
| `sns download-comment-image` | 评论中的图片 | **`--url`**、`--cdn-token`/`--key`/`--enc-idx`（有则必传）；`--out` |
| `sns download-video` | 视频朋友圈的视频与封面图地址 | **`--xml`/`--xml-file`**；`--out` 存视频 |

### proxy 代理

| 命令 | 说明 | 参数 |
|---|---|---|
| `proxy provinces` | 获取平台代理可用的省份列表（内置官方对照表，本地查询，无需 token） | — |
| `proxy cities` | 获取城市代码列表（平台代理 `--proxy-info` 传城市代码），本地查询无需 token | `--province` / `--city` 过滤（精确匹配） |
| `proxy get` | 获取设备当前代理信息 | — |
| `proxy set` | 设置设备代理 | **`--proxy-type`** local/self/platform、**`--proxy-info`** |
| `proxy check-city` | 检查城市是否有可用代理 | **`--city-code`** |

### data 数据查询与服务管理

联系人、消息、群聊等数据由 `wekeapi-msg-sync` 服务统一接收回调、同步并存储；主 CLI 通过服务 HTTP API 查询，不再维护本地数据库。

| 命令 | 说明 | 参数 |
|---|---|---|
| `data contacts` | 查询联系人（昵称/备注/别名/wxid 模糊匹配） | `--keyword`、`--type` friend/chatroom/gh/wework/all、`--page`、`--limit` |
| `data messages` | 查询消息（按时间倒序） | `--conversation-id` S:<wxid> 或 R:<roomid>、`--keyword`、`--start-time`/`--end-time`、`--page`、`--limit` |
| `data rooms` | 查询群聊 | `--keyword`、`--page`、`--limit` |
| `data status` | 同步状态与数据量（status/last_sync_time/contacts_count/messages_count/rooms_count） | — |
| `data config` | 设置数据库（SQLite/MySQL）与服务端口；不带参数时只查看当前配置 | `--db` sqlite/mysql、`--db-path` SQLite 文件名（默认 `wekeapi_msg_sync.db`）、`--db-dsn` MySQL 连接串（DSN 或 `mysql://` URI）、`--port`（默认 3006） |
| `data start` | 启动服务（已运行则保持） | `--msg-sync-dir` |
| `data stop` | 停止服务 | `--msg-sync-dir` |
| `data sync` / `data restart` | 重启服务（先关闭再启动，触发一次全量同步） | `--msg-sync-dir` |

- 输出与其他命令一致：`{"code":0,"message":"OK","data":{...}}`；服务返回错误时退出码 1。
- `data.contacts[].id` 即联系人 wxid，`label_ids`/`labels` 为标签 ID 与名称。
- 服务未运行时数据查询会提示先执行 `wekeapi data start`；服务日志在 `~/.wekeapi/msg-sync/wekeapi-msg-sync.log`。
- 数据库与服务端口：默认 SQLite（`~/.wekeapi/msg-sync/wekeapi_msg_sync.db`）、端口 3006；用 `data config` 修改后需 `data sync` 重启生效（切换数据库类型不迁移旧库数据，重启后重新全量同步）。

## 常用场景示例

```bash
TOKEN=your_token; APP_ID=your_app_id

# 发文本并在群里@所有人
wekeapi message send-text --token $TOKEN --app-id $APP_ID \
  --to-wxid 12345@chatroom --content '@all 开会了' --ats all

# 发本地图片
wekeapi message send-image --token $TOKEN --app-id $APP_ID \
  --to-wxid wxid_xxx --file ./photo.jpg

# 发图片给第一人后，用响应转发给其他人（高效群发）
RESP=$(wekeapi message send-image --token $TOKEN --app-id $APP_ID --to-wxid wxid_a --file ./photo.jpg)
echo "$RESP" > send_resp.json
wekeapi message forward-image --token $TOKEN --app-id $APP_ID \
  --to-wxid wxid_b --data @send_resp.json

# 下载收到的图片原图并保存
wekeapi message download-image --token $TOKEN --app-id $APP_ID \
  --type 1 --xml '<msg><img .../></msg>' --out ./download.jpg

# 建群并拉人
wekeapi chatroom create --token $TOKEN --app-id $APP_ID --wxids wxid_a,wxid_b
wekeapi chatroom invite --token $TOKEN --app-id $APP_ID \
  --chatroom-id 12345@chatroom --wxids wxid_c

# 给好友打标签
wekeapi label set-friend --token $TOKEN --app-id $APP_ID \
  --wxid wxid_xxx --label-ids 101,102

# 发一条朋友圈（本地图片自动 base64）
wekeapi sns send-image --token $TOKEN --app-id $APP_ID \
  --visibility 1 --content '今日上新' --images ./a.jpg,./b.jpg

# 查自己的朋友圈，再给第一条点赞
wekeapi sns list --token $TOKEN --app-id $APP_ID
wekeapi sns like --token $TOKEN --app-id $APP_ID \
  --sns-id 1234567890123 --sns-wxid wxid_me

# 下载某条图文朋友圈的全部图片
wekeapi sns download-image --token $TOKEN --app-id $APP_ID \
  --xml-file ./sns.xml --out-dir ./sns_imgs

# 查询同步数据（需 wekeapi-msg-sync 已启动）
wekeapi data contacts --keyword 张三
wekeapi data messages --conversation-id 'S:wxid_xxx' --limit 20
wekeapi data status
```
