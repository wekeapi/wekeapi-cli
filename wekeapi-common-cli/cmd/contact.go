package cmd

import "wekeapi-cli/internal/runner"

var contactSpecs = []runner.CmdSpec{
	{
		Use:   "list",
		Path:  "/v1/contact/fetchContactList",
		Short: "获取通讯录列表（好友/群/公众号，仅返回 ID）",
		Long: `获取微信通讯录列表，包括好友、微信群和关注的公众号，只返回 ID。
本接口返回的群聊仅为保存到通讯录中的群聊；未保存通讯录的群，需要根据消息推送自行判断并保存。
平台缓存读取：账号登录后平台自动同步一次通讯录并缓存，本接口从缓存获取；
如遇"通讯录同步进行中"错误，请等待后再次调用。`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "contact-type", BodyKey: "contactType", Kind: runner.KindInt, Usage: "联系人类型：1-好友 2-群 3-公众号；省略返回全部"},
		},
	},
	{
		Use:        "brief",
		Path:       "/v1/contact/getBriefInfo",
		Short:      "批量获取联系人/群/公众号简要信息",
		Long:       "批量获取联系人/群/公众号的简要信息（昵称/头像/性别/地区/备注等）；从平台缓存读取，遇\"通讯录同步进行中\"错误请稍后再试。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "待查询的 wxid 列表，多个用英文逗号分隔"},
		},
	},
	{
		Use:        "detail",
		Path:       "/v1/contact/getDetailInfo",
		Short:      "批量获取联系人/群/公众号详细信息",
		Long:       "批量获取联系人/群/公众号详细信息（在简要信息基础上含标签/签名/群成员列表等）；列表接口一次最多 20 个 wxid。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "待查询的 wxid 列表，多个用英文逗号分隔"},
		},
	},
	{
		Use:   "sync",
		Path:  "/v1/contact/updateContactList",
		Short: "强制同步通讯录到平台并更新缓存",
		Long: `启动微信通讯录同步并更新平台缓存，相当于强制更新微信通讯录到平台。
微信上新增的联系人在平台没有更新时，调用本接口强制更新。`,
		NeedsAppID: true,
	},
	{
		Use:   "search",
		Path:  "/v1/contact/searchContact",
		Short: "搜索联系人（结果用于添加好友）",
		Long: `搜索微信联系人；搜索结果可在"添加好友"（contact add-friend）中使用。
若搜索的联系人已经是好友，响应中的 V3 即为其 wxid。`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "contact-info", BodyKey: "contactInfo", Kind: runner.KindString, Required: true, Usage: "微信号/手机号"},
		},
	},
	{
		Use:   "add-friend",
		Path:  "/v1/contact/addContact",
		Short: "主动添加好友",
		Long: `主动添加好友；好友添加成功后，会通过回调消息推送一条包含 V3 的消息，可用于判断是否添加成功。
建议账号在线 3 天后再调用。v3/v4 通过搜索联系人（contact search）或群成员详情（chatroom member-detail）得到。`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "scene", BodyKey: "scene", Kind: runner.KindInt, Required: true, Usage: "添加来源：1-QQ 2-邮箱 3-微信号 4-QQ好友 13-通讯录 14-群聊 15-手机号 30-二维码"},
			{Name: "v3", BodyKey: "v3", Kind: runner.KindString, Required: true, Usage: "V3 信息（搜索好友或群成员详情得到）"},
			{Name: "v4", BodyKey: "v4", Kind: runner.KindString, Required: true, Usage: "V4 信息（搜索好友或群成员详情得到）"},
			{Name: "verify-content", BodyKey: "verifyContent", Kind: runner.KindString, Required: true, Usage: "添加好友时的招呼语（验证信息）"},
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Usage: "通过群聊添加好友时群的 id"},
		},
	},
	{
		Use:        "accept-friend",
		Path:       "/v1/contact/verifyContact",
		Short:      "同意好友添加申请",
		Long:       "同意好友添加申请；好友发送添加申请后会收到一条回调消息，请求参数从回调消息中获取。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "msg-base64", BodyKey: "msgBase64", Kind: runner.KindString, Required: true, Usage: "好友申请回调消息内容（base64）"},
		},
	},
	{
		Use:        "delete-friend",
		Path:       "/v1/contact/deleteContact",
		Short:      "删除好友",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "待删除好友的 wxid"},
		},
	},
	{
		Use:        "set-remark",
		Path:       "/v1/contact/setContactRemark",
		Short:      "设置好友备注",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "好友 wxid"},
			{Name: "remark", BodyKey: "remark", Kind: runner.KindString, Required: true, Usage: "待设置的备注"},
		},
	},
	{
		Use:        "set-chat-only",
		Path:       "/v1/contact/setContactPermission",
		Short:      "设置好友仅聊天",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "好友 wxid"},
			{Name: "only-chat", BodyKey: "onlyChat", Kind: runner.KindBool, Required: true, Usage: "是否仅聊天（true/false）"},
		},
	},
	{
		Use:        "wework-list",
		Path:       "/v1/contact/fetchQWContactList",
		Short:      "获取企微好友列表",
		NeedsAppID: true,
	},
	{
		Use:        "wework-info",
		Path:       "/v1/contact/getQWContactInfo",
		Short:      "批量获取企微好友信息",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "企微好友 ID 列表，多个用英文逗号分隔"},
		},
	},
	{
		Use:        "wework-search",
		Path:       "/v1/contact/searchQWContact",
		Short:      "搜索企微联系人（结果用于添加企微好友）",
		Long:       "搜索企微联系人；搜索结果可在\"添加企微好友\"（contact wework-add）中使用。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "contact-info", BodyKey: "contactInfo", Kind: runner.KindString, Required: true, Usage: "企微联系人搜索关键字"},
		},
	},
	{
		Use:        "wework-add",
		Path:       "/v1/contact/addQWContact",
		Short:      "添加企微好友",
		Long:       "申请添加企微联系人为好友；wxid 与 v1 均通过搜索企微联系人（contact wework-search）得到。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "企微联系人 ID"},
			{Name: "v1", BodyKey: "v1", Kind: runner.KindString, Required: true, Usage: "v1 信息（搜索企微联系人得到）"},
			{Name: "veryfy-content", BodyKey: "veryfyContent", Kind: runner.KindString, Usage: "添加好友时的招呼语（验证信息）"},
		},
	},
	{
		Use:        "wework-delete",
		Path:       "/v1/contact/deleteQWContact",
		Short:      "删除企微好友",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "企微好友 ID"},
		},
	},
	{
		Use:        "wework-set-remark",
		Path:       "/v1/contact/setQWContactRemark",
		Short:      "设置企微好友备注",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "企微好友 ID"},
			{Name: "remark", BodyKey: "remark", Kind: runner.KindString, Required: true, Usage: "待设置的企微好友备注"},
		},
	},
	{
		Use:        "phone-list",
		Path:       "/v1/contact/getMobileContact",
		Short:      "获取手机通讯录里的微信好友",
		NeedsAppID: true,
	},
	{
		Use:        "phone-upload",
		Path:       "/v1/contact/uploadMobileContact",
		Short:      "上传手机通讯录",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "op-code", BodyKey: "opCode", Kind: runner.KindInt, Required: true, Usage: "操作类型：1-上传 2-删除"},
			{Name: "mobiles", BodyKey: "mobiles", Kind: runner.KindStringList, Required: true, Usage: "待操作的手机号列表，多个用英文逗号分隔"},
		},
	},
}
