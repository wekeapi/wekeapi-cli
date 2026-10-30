package cmd

import "wekeapi-cli/internal/runner"

var chatroomSpecs = []runner.CmdSpec{
	{
		Use:        "create",
		Path:       "/v1/chatroom/createChatroom",
		Short:      "创建新的微信群",
		Long:       "创建新的微信群。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "群成员wxid列表（逗号分隔），至少有两个"},
		},
	},
	{
		Use:        "members",
		Path:       "/v1/chatroom/getChatroomMemberList",
		Short:      "获取群成员列表",
		Long:       "获取群成员列表。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
		},
	},
	{
		Use:   "member-detail",
		Path:  "/v1/chatroom/getChatroomMemberDetail",
		Short: "获取群成员详情",
		Long: `获取群成员详情。

- 此接口仅支持获取群内微信成员详情；企微成员可直接调用通讯录模块的“获取企微好友信息”接口进行获取`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "member-wxids", BodyKey: "memberWxids", Kind: runner.KindStringList, Required: true, Usage: "群成员wxid列表（逗号分隔）"},
		},
	},
	{
		Use:        "add-friend",
		Path:       "/v1/chatroom/addMemberToContact",
		Short:      "添加群成员为好友，发出添加申请",
		Long:       "添加群成员为好友，发出添加申请。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "member-wxid", BodyKey: "memberWxid", Kind: runner.KindString, Required: true, Usage: "待添加的群成员wxid"},
			{Name: "verify-content", BodyKey: "verifyContent", Kind: runner.KindString, Required: true, Usage: "添加好友时的招呼语（验证信息）"},
		},
	},
	{
		Use:        "invite",
		Path:       "/v1/chatroom/inviteMember",
		Short:      "邀请微信好友加入群聊",
		Long:       "邀请微信好友加入群聊。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "邀请成员wxid列表（逗号分隔）"},
		},
	},
	{
		Use:        "add-member",
		Path:       "/v1/chatroom/addMember",
		Short:      "添加微信好友加入群聊",
		Long:       "添加微信好友加入群聊；响应 data 为成功添加的群成员wxid列表。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "添加成员wxid列表（逗号分隔）"},
		},
	},
	{
		Use:        "scan-join",
		Path:       "/v1/chatroom/joinChatroomWithQrCode",
		Short:      "扫二维码进群",
		Long:       "扫二维码进群。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "qr-code-info", BodyKey: "qrCodeInfo", Kind: runner.KindString, Required: true, Usage: "二维码对应的信息，即解析二维码得到的URL"},
		},
	},
	{
		Use:        "accept-join",
		Path:       "/v1/chatroom/agreeToJoinChatroom",
		Short:      "同意加入群聊",
		Long:       "同意加入群聊。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "url", BodyKey: "url", Kind: runner.KindString, Required: true, Usage: "邀请进群回调消息中的URL"},
		},
	},
	{
		Use:   "approve-invite",
		Path:  "/v1/chatroom/approveAddMember",
		Short: "同意群成员邀请其他成员进群的申请",
		Long: `开启群聊邀请确认后，群主或群管理员同意群成员邀请其他成员进群的申请。

- 必须是群主或群管理员才能操作`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "msg-base64", BodyKey: "msgBase64", Kind: runner.KindString, Required: true, Usage: "邀请进群回调消息中xml的Base64编码"},
			{Name: "new-msg-id", BodyKey: "newMsgId", Kind: runner.KindInt, Required: true, Usage: "邀请进群回调消息的newMsgId"},
		},
	},
	{
		Use:        "qrcode",
		Path:       "/v1/chatroom/getChatroomQrCode",
		Short:      "获取某个群的群二维码",
		Long:       "获取某个群的群二维码；响应 data.qrCodeImgData 为 base64 编码的二维码图片数据，可用 --out 保存为图片文件。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
		},
		OutBase64Key: "qrCodeImgData",
	},
	{
		Use:   "transfer-owner",
		Path:  "/v1/chatroom/transferChatroomOwner",
		Short: "将群主管理权转让给其他群成员",
		Long: `将群主管理权转让给其他群成员。

- 只有群主有权限操作`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "新群主的wxid"},
		},
	},
	{
		Use:   "add-admin",
		Path:  "/v1/chatroom/addChatroomAdmin",
		Short: "添加群管理员",
		Long: `添加群管理员。

- 只有群主有权限操作
- 最多可以添加3个管理员`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "待添加为管理员的wxid列表（逗号分隔），最多可以添加3个管理员"},
		},
	},
	{
		Use:   "remove-admin",
		Path:  "/v1/chatroom/deleteChatroomAdmin",
		Short: "移除群管理员",
		Long: `移除群管理员。

- 只有群主有权限操作`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "待删除管理员的wxid列表（逗号分隔），最多可以删除3个管理员"},
		},
	},
	{
		Use:        "set-announcement",
		Path:       "/v1/chatroom/setChatroomAnnouncement",
		Short:      "设置群公告",
		Long:       "设置群公告。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Required: true, Usage: "群公告内容"},
		},
	},
	{
		Use:        "get-announcement",
		Path:       "/v1/chatroom/getChatroomAnnouncement",
		Short:      "获取群公告",
		Long:       "获取群公告。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
		},
	},
	{
		Use:   "set-invite-confirm",
		Path:  "/v1/chatroom/setInviteVerify",
		Short: "开启/关闭群聊邀请确认",
		Long: `开启/关闭群聊邀请确认。

- inviteVerifySet 取值：1-开启 0-关闭`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "invite-verify-set", BodyKey: "inviteVerifySet", Kind: runner.KindInt, Required: true, Usage: "群聊邀请确认设置：1-开启 0-关闭"},
		},
	},
	{
		Use:        "rename",
		Path:       "/v1/chatroom/modifyChatroomName",
		Short:      "修改群名称",
		Long:       "修改群名称。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "chatroom-name", BodyKey: "chatroomName", Kind: runner.KindString, Required: true, Usage: "新的群名称"},
		},
	},
	{
		Use:        "set-remark",
		Path:       "/v1/chatroom/modifyChatroomRemark",
		Short:      "修改群备注",
		Long:       "修改群备注。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "chatroom-remark", BodyKey: "chatroomRemark", Kind: runner.KindString, Required: true, Usage: "新的群备注"},
		},
	},
	{
		Use:        "set-my-nickname",
		Path:       "/v1/chatroom/modifyNickNameInChatroom",
		Short:      "修改我在群内的昵称",
		Long:       "修改我在群内的昵称。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "nick-name-in-chatroom", BodyKey: "nickNameInChatroom", Kind: runner.KindString, Required: true, Usage: "我在群内的昵称"},
		},
	},
	{
		Use:   "pat",
		Path:  "/v1/chatroom/sendPat",
		Short: "群拍一拍",
		Long: `群拍一拍。

- 请求发出后对方会在5秒后收到`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "to-wxid", BodyKey: "toWxid", Kind: runner.KindString, Required: true, Usage: "被拍的群成员wxid"},
		},
	},
	{
		Use:   "remove-member",
		Path:  "/v1/chatroom/deleteMember",
		Short: "将成员移出群",
		Long: `将成员移出群；响应 data 为成功移出的群成员wxid列表。

- 必须是群主或群管理员才能操作`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
			{Name: "wxids", BodyKey: "wxids", Kind: runner.KindStringList, Required: true, Usage: "删除成员wxid列表（逗号分隔）"},
		},
	},
	{
		Use:   "dissolve",
		Path:  "/v1/chatroom/dismissChatroom",
		Short: "解散群聊",
		Long: `解散群聊。

- 只有群主有权限操作`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
		},
	},
	{
		Use:        "quit",
		Path:       "/v1/chatroom/quitChatroom",
		Short:      "退出群聊",
		Long:       "退出群聊。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "群ID"},
		},
	},
	{
		Use:        "save",
		Path:       "/v1/chatroom/saveToContact",
		Short:      "保存群到通讯录",
		Long:       "保存群到通讯录。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "待加入通讯录的群ID"},
		},
	},
	{
		Use:        "unsave",
		Path:       "/v1/chatroom/removeFromContact",
		Short:      "从通讯录移除群",
		Long:       "从通讯录移除群。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "chatroom-id", BodyKey: "chatroomId", Kind: runner.KindString, Required: true, Usage: "待移出通讯录的群ID"},
		},
	},
}
