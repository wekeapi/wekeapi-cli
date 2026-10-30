# wekeapi-msg-sync

wekeapi 数据同步服务：接收消息回调、增量同步联系人、存储消息，并提供 REST 查询 API。

## 环境要求

- Go 1.24+（仅源码构建需要）
- SQLite（无需安装）；
- 如果使用 MySQL 需要安装 5.7+ / 8.0+（DSN 中的库不存在时服务会自动创建，账号需具备 CREATE 权限）

## 构建

```powershell
# Windows：交叉编译 6 个平台二进制到 dist/
.\build.ps1
```

```bash
# macOS / Linux
./build.sh
```

| 产物 | 平台 |
| --- | --- |
| `dist/wekeapi-msg-sync-windows-amd64.exe` | Windows x64 |
| `dist/wekeapi-msg-sync-windows-arm64.exe` | Windows ARM64 |
| `dist/wekeapi-msg-sync-linux-amd64` | Linux x64 |
| `dist/wekeapi-msg-sync-linux-arm64` | Linux ARM64 |
| `dist/wekeapi-msg-sync-darwin-amd64` | macOS Intel |
| `dist/wekeapi-msg-sync-darwin-arm64` | macOS Apple Silicon |

## 配置

首次使用可由主 CLI 自动生成（`wekeapi login` 登录成功会写入配置文件并启动服务），也可手动复制示例配置：

```bash
mkdir -p ~/.wekeapi/msg-sync
cp wekeapi-msg-sync.example.json ~/.wekeapi/msg-sync/wekeapi-msg-sync.json
```

配置文件查找规则：

- `run --config <path>` 省略时，读取**默认位置** `~/.wekeapi/msg-sync/wekeapi-msg-sync.json`；
- `--config` 使用相对路径时按当前工作目录解析。

### SQLite（默认）

```json
{
  "token": "your-api-token",
  "appId": "your-app-id",
  "wxid": "your-wxid",
  "port": 3006,
  "db_path": "./wekeapi_msg_sync.db"
}
```

SQLite DSN 会自动追加 `busy_timeout(10000)` 与 `journal_mode(WAL)`，并自动创建数据库文件所在目录。

### MySQL（可选）

```json
{
  "token": "your-api-token",
  "appId": "your-app-id",
  "wxid": "your-wxid",
  "port": 3006,
  "db": {
    "driver": "mysql",
    "dsn": "user:password@tcp(127.0.0.1:3306)/wekeapi_msg_sync?charset=utf8mb4&parseTime=true&loc=Local",
    "max_open_conns": 10,
    "max_idle_conns": 5,
    "conn_max_lifetime_seconds": 3600
  }
}
```

使用 MySQL 时，DSN 里的数据库不存在会自动创建（utf8mb4），表也由服务启动时自动建。

```sql
CREATE DATABASE wekeapi_msg_sync DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
```

### 配置项

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `token` | 必填 | wekeapi API token |
| `appId` | 必填 | 账号 appId，同时作为数据归属键 |
| `wxid` | 必填 | 当前登录账号 wxid，用于区分消息收发（私聊会话 ID）与拆分群消息发送人 |
| `port` | `3006` | HTTP 监听端口（`--port` 优先） |
| `db_path` | `./wekeapi_msg_sync.db` | SQLite 文件路径（相对路径按配置文件所在目录解析）；与 `db` **互斥** |
| `db.driver` | `mysql` | 仅支持 `mysql`；写 `sqlite` 会提示改用 `db_path` |
| `db.dsn` | 必填 | go-sql-driver/mysql DSN |
| `db.max_open_conns` | `10` | 连接池最大连接数 |
| `db.max_idle_conns` | `5` | 连接池最大空闲连接数 |
| `db.conn_max_lifetime_seconds` | `3600` | 连接最长存活时间（秒） |


互斥规则：使用 SQLite 只配置 `db_path` 并删除 `db`；使用 MySQL 只配置 `db` 并删除 `db_path`。两者同时出现会启动失败（删除配置项不会删除已有数据库文件）。

## 与 wekeapi CLI 集成

主 CLI（`wekeapi login check` 登录成功）会自动接管本服务的配置与生命周期：

1. 把主配置的 `token`/`appId`/`wxid` 写入服务目录的 `wekeapi-msg-sync.json`（保留已有的 `port`/`db`/`db_path` 等配置）；
2. 服务已启动时先检测是否已经启动，如果没有启动才执行启动命令；
3. 服务启动后立即执行一次全量同步。

手动管理：

```bash
wekeapi data start        # 启动（已运行则保持）
wekeapi data stop         # 停止
wekeapi data sync         # 重启（先关闭再启动，触发全量同步）
wekeapi data status       # 同步状态与数据量
wekeapi data contacts     # 查询联系人
wekeapi data messages     # 查询消息
wekeapi data rooms        # 查询群聊
```

## 启动与退出

```bash
# 使用默认配置 ~/.wekeapi/msg-sync/wekeapi-msg-sync.json
./wekeapi-msg-sync run

# 指定配置与端口（--port 优先于配置文件的 port）
./wekeapi-msg-sync run --config ./wekeapi-msg-sync.json --port 3006
```

## 回调接入

在 wekeapi 平台将回调地址绑定为：

```
#公网地址可以使用ngrok等三方工具映射
http://<公网可达地址>:<port>/callback
```

服务只接收回调，不调用平台绑定接口；公网暴露 / NAT 映射由部署方完成。

回调体格式：

```json
{"appId":"...","wxid":"...","msgFlag":"AddMsg|ModContacts|DelContact|Offline","msgData":{...}}
```

## API

统一响应：成功 `{"success":true,"data":...}`；失败 `{"success":false,"error":"...","code":"INVALID_PARAMS|INTERNAL_ERROR"}`。

账号参数 `app_id` 可选，缺省使用配置的 `appId`；显式传入其他值返回 400（单账号服务）。分页参数 `page>=1`，`limit` 取值 1-100、默认 20，响应包含 `total/page/limit`。

### GET /api/contacts

| 参数 | 说明 |
| --- | --- |
| `app_id` | 可选，缺省为配置 appId |
| `type` | `friend` / `chatroom` / `gh` / `wework` / `all`，默认 `all` |
| `keyword` | 匹配昵称 / 备注 / alias / 联系人 ID |
| `label` | 按标签过滤：标签名称或标签 ID，可重复传参或逗号分隔；多个标签为「同时具备」 |
| `page` / `limit` | 分页 |

```bash
curl "http://127.0.0.1:3006/api/contacts?type=friend&keyword=张&page=1&limit=20"
curl "http://127.0.0.1:3006/api/contacts?label=客户"
curl "http://127.0.0.1:3006/api/contacts?label=客户,重点"   # 同时具备两个标签
```

`label` 传名称时在本地 `labels` 表解析（忽略大小写），名称不存在返回 400；传纯数字按标签 ID 处理。

返回项：`id, name, nickname, remark, alias, avatar, type, sex, label_ids, labels, updated_at`。

### GET /api/messages

| 参数 | 说明 |
| --- | --- |
| `app_id` | 可选 |
| `conversation_id` | 私聊 `S:<wxid>`，群聊 `R:<roomid>` |
| `keyword` | 匹配消息内容 |
| `start_time` / `end_time` | RFC3339 或 Unix 秒 |
| `page` / `limit` | 分页 |

```bash
curl "http://127.0.0.1:3006/api/messages?conversation_id=S:wxid_abc&limit=50"
curl "http://127.0.0.1:3006/api/messages?conversation_id=R:123@chatroom&start_time=1700000000&end_time=1700086400"
```

返回项：`id, new_msg_id, conversation_id, from, to, room_id, content, timestamp(RFC3339), create_time, type, msg_type, revoked`。收发方向由 `from` 是否等于本账号 wxid 判断。

`type` 映射：`1 text`、`3 image`、`34 voice`、`43 video`、`42 card`、`47 emoji`、`48 location`、`49 app`、`10000 system`、`10002 sysmsg`、其他 `other`。

### POST /api/messages

记录一条**本账号已发出**的消息（由 CLI 发送/转发成功后调用；`from_wxid` 固定取服务配置的 `wxid`）。

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `to_wxid` | 是 | 接收人 wxid 或群 ID |
| `msg_type` | 是 | 微信消息类型（1 文本、3 图片、34 语音、43 视频、42 名片、47 表情、49 应用类） |
| `new_msg_id` | 否 | 发送响应中的 `newMsgId`；缺失时服务用内容哈希 `h:<sha1>` 兜底 |
| `create_time` | 否 | 发送响应中的 `createTime`（Unix 秒）；缺失取当前时间 |
| `room_id` | 否 | 群 ID；缺省由 `to_wxid` 的 `@chatroom` 后缀推导 |
| `content` | 否 | 可检索内容（文本内容/链接标题/文件名等，不含 base64） |
| `raw_json` | 否 | 发送响应原始 JSON，便于排查（上限 1MB 请求体） |

```bash
curl -X POST "http://127.0.0.1:3006/api/messages" -H "Content-Type: application/json" \
  -d '{"new_msg_id":"1234567890","to_wxid":"wxid_abc","msg_type":1,"create_time":1700000000,"content":"你好"}'
# {"success":true,"data":{"inserted":true,"new_msg_id":"1234567890"}}
```

重复上报（同一 `wx_id + new_msg_id`）返回 `inserted=false`，不报错。

### GET /api/rooms

仅返回群聊（`contact_type=2`），支持 `keyword`（匹配群名称 / 备注 / 群 ID）与分页。

```bash
curl "http://127.0.0.1:3006/api/rooms?page=1&limit=20"
```

返回项：`id, name, remark, avatar, owner, member_count, updated_at`。平台详情接口不返回群主，`owner` 固定为空；`member_count` 由详情响应的 `chatroomMemberList` 长度入库。

### GET /api/sync-status

```bash
curl "http://127.0.0.1:3006/api/sync-status"
```

返回项：`app_id, wxid, status(idle|syncing|error), last_sync_time, last_error, contacts_count, messages_count, rooms_count, db_driver`。

### GET /healthz

```bash
curl "http://127.0.0.1:3006/healthz"
# {"success":true,"data":{"status":"ok","uptime_seconds":12}}
```

## 同步策略

1. **启动全量**：标签 → 通讯录列表 → 好友 / 群 / 公众号详情（每批 10 个、批间隔 200ms）→ 企微好友（列表自带详情）。
2. **回调增量**（实时）：`AddMsg` 落消息；`ModContacts` 新增/更新联系人并异步补拉详情；`DelContact` 删联系人；系统消息识别入群/退群/解散群。
   - 回调新增的好友/群/企微好友先写骨架并异步补拉详情；群里未收录的新群会在收到群消息时立即加入联系人。
   - 远端已删除的联系人依赖 `DelContact` 回调；**群组只新增 / 更新不删除**（接口只返回已保存到通讯录的群）。
3. 过滤系统账号（`filehelper`、`fmessage` 等）。

## 错误码

| HTTP | code | 场景 |
| --- | --- | --- |
| 400 | `INVALID_PARAMS` | 参数格式错误、`app_id` 与配置不一致、回调请求体非法 |
| 500 | `INTERNAL_ERROR` | 数据库或内部错误（回调场景平台会重试） |

