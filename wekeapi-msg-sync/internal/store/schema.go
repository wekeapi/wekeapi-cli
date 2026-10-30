package store

// sqliteMessagesTableDDL 消息表建表语句（迁移旧库时复用）。
const sqliteMessagesTableDDL = `CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  wx_id TEXT NOT NULL,
  new_msg_id TEXT NOT NULL,
  room_id TEXT NOT NULL DEFAULT '',
  create_time INTEGER NOT NULL DEFAULT 0,
  msg_type INTEGER NOT NULL DEFAULT 0,
  from_wxid TEXT NOT NULL DEFAULT '',
  to_wxid TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  raw_json TEXT NOT NULL DEFAULT '',
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(wx_id, new_msg_id)
)`

// sqliteDDL SQLite 建表语句（逐条执行）。SQLite 不支持字段注释，注释见 MySQL DDL。
var sqliteDDL = []string{
	`CREATE TABLE IF NOT EXISTS contacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  wx_id TEXT NOT NULL,
  contact_id TEXT NOT NULL,
  contact_type INTEGER NOT NULL,
  nickname TEXT NOT NULL DEFAULT '',
  alias TEXT NOT NULL DEFAULT '',
  avatar TEXT NOT NULL DEFAULT '',
  remark TEXT NOT NULL DEFAULT '',
  sex INTEGER NOT NULL DEFAULT 0,
  signature TEXT NOT NULL DEFAULT '',
  labels TEXT NOT NULL DEFAULT '',
  wording_id TEXT NOT NULL DEFAULT '',
  wording_name TEXT NOT NULL DEFAULT '',
  member_count INTEGER NOT NULL DEFAULT 0,
  raw_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(wx_id, contact_type, contact_id)
)`,
	`CREATE INDEX IF NOT EXISTS idx_contacts_owner ON contacts(wx_id, contact_type)`,
	sqliteMessagesTableDDL,
	`CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(wx_id, room_id, create_time)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_time ON messages(wx_id, create_time)`,
	`CREATE TABLE IF NOT EXISTS labels (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  wx_id TEXT NOT NULL,
  label_id INTEGER NOT NULL,
  label_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(wx_id, label_id)
)`,
	`CREATE TABLE IF NOT EXISTS sync_state (
  wx_id TEXT NOT NULL PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'idle',
  last_sync_at TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
)`,
}

// mysqlDDL MySQL 建表语句（逐条执行；要求 DSN 中的数据库已存在）。
// 每个字段与表都带 COMMENT，便于在客户端或文档中直接查看含义。
var mysqlDDL = []string{
	`CREATE TABLE IF NOT EXISTS contacts (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键',
  wx_id VARCHAR(64) NOT NULL COMMENT '账号 appId，数据归属键',
  contact_id VARCHAR(191) NOT NULL COMMENT '联系人 ID：好友/公众号 wxid、群 roomid、企微 ID',
  contact_type INT NOT NULL COMMENT '联系人类型：1 好友、2 群聊、3 公众号、4 企微好友',
  nickname VARCHAR(255) NOT NULL DEFAULT '' COMMENT '昵称或群名称',
  alias VARCHAR(191) NOT NULL DEFAULT '' COMMENT '微信号（alias）',
  avatar VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '头像 URL',
  remark VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '备注',
  sex INT NOT NULL DEFAULT 0 COMMENT '性别：0 未知、1 男、2 女',
  signature VARCHAR(2048) NOT NULL DEFAULT '' COMMENT '个性签名',
  labels VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '标签 ID 列表，逗号分隔',
  wording_id VARCHAR(191) NOT NULL DEFAULT '' COMMENT '企微好友所属企业 ID',
  wording_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '企微好友所属企业名称',
  member_count INT NOT NULL DEFAULT 0 COMMENT '群成员数',
  raw_json LONGTEXT NULL COMMENT '回调或接口返回的原始 JSON',
  created_at VARCHAR(32) NOT NULL COMMENT '创建时间（UTC，2006-01-02 15:04:05）',
  updated_at VARCHAR(32) NOT NULL COMMENT '更新时间（UTC，2006-01-02 15:04:05）',
  PRIMARY KEY (id),
  UNIQUE KEY uk_contacts (wx_id, contact_type, contact_id),
  KEY idx_contacts_owner (wx_id, contact_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信联系人：好友、群聊、公众号、企微好友'`,
	`CREATE TABLE IF NOT EXISTS messages (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键',
  wx_id VARCHAR(64) NOT NULL COMMENT '账号 appId，数据归属键',
  new_msg_id VARCHAR(191) NOT NULL COMMENT '微信消息 ID；回调缺失时用内容哈希 h:<sha1> 兜底，与 wx_id 组成唯一键用于去重',
  room_id VARCHAR(191) NOT NULL DEFAULT '' COMMENT '群 ID；私聊消息为空',
  create_time BIGINT NOT NULL DEFAULT 0 COMMENT '消息时间（Unix 秒）',
  msg_type INT NOT NULL DEFAULT 0 COMMENT '微信消息类型：1 文本、3 图片、34 语音、43 视频、42 名片、47 表情、48 位置、49 应用、10000 系统通知、10002 sysmsg',
  from_wxid VARCHAR(191) NOT NULL DEFAULT '' COMMENT '发送人 wxid；群消息为真实发言人，等于本账号 wxid 即为自己发出',
  to_wxid VARCHAR(191) NOT NULL DEFAULT '' COMMENT '接收人 wxid 或群 ID',
  content LONGTEXT NULL COMMENT '消息内容；群消息已去掉“发送人:”前缀',
  raw_json LONGTEXT NULL COMMENT '回调原始 JSON（已移除 img_buf/ImgBuf 缩略图）',
  revoked TINYINT NOT NULL DEFAULT 0 COMMENT '是否已撤回：0 否、1 是',
  created_at VARCHAR(32) NOT NULL COMMENT '创建时间（UTC，2006-01-02 15:04:05）',
  updated_at VARCHAR(32) NOT NULL COMMENT '更新时间（UTC，2006-01-02 15:04:05）',
  PRIMARY KEY (id),
  UNIQUE KEY uk_messages_msgid (wx_id, new_msg_id),
  KEY idx_messages_room (wx_id, room_id, create_time),
  KEY idx_messages_time (wx_id, create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信消息：收发统一存储，收发方向由 from_wxid 是否等于本账号 wxid 判断'`,
	`CREATE TABLE IF NOT EXISTS labels (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键',
  wx_id VARCHAR(64) NOT NULL COMMENT '账号 appId，数据归属键',
  label_id BIGINT NOT NULL COMMENT '微信标签 ID',
  label_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '标签名称',
  created_at VARCHAR(32) NOT NULL COMMENT '创建时间（UTC，2006-01-02 15:04:05）',
  updated_at VARCHAR(32) NOT NULL COMMENT '更新时间（UTC，2006-01-02 15:04:05）',
  PRIMARY KEY (id),
  UNIQUE KEY uk_labels (wx_id, label_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信标签'`,
	`CREATE TABLE IF NOT EXISTS sync_state (
  wx_id VARCHAR(64) NOT NULL COMMENT '账号 appId，数据归属键',
  status VARCHAR(32) NOT NULL DEFAULT 'idle' COMMENT '同步状态：idle 空闲、syncing 同步中、error 失败',
  last_sync_at VARCHAR(32) NOT NULL DEFAULT '' COMMENT '最近一次同步结束时间（UTC）',
  last_error TEXT NULL COMMENT '最近一次同步的错误信息',
  updated_at VARCHAR(32) NOT NULL COMMENT '更新时间（UTC，2006-01-02 15:04:05）',
  PRIMARY KEY (wx_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='联系人同步状态'`,
}
