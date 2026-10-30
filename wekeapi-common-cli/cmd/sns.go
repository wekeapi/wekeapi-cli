package cmd

import (
	"github.com/spf13/cobra"

	"wekeapi-cli/internal/runner"
)

// snsSpecs 朋友圈（/v1/sns/*）模块：列表/详情、发布、转发、可见范围、点赞评论与媒体下载。
var snsSpecs = []runner.CmdSpec{
	{
		Use:   "list",
		Path:  "/v1/sns/getSnsTimeline",
		Short: "获取自己的朋友圈列表（时间线）",
		Long: `获取自己朋友圈的时间线列表。
分页：首次不传分页参数；第 2 页起传上次响应 data 中的 firstPageMd5 与 fromId。
返回的 snsItemList[].id 即朋友圈 Id（后续点赞、评论、删除、下载用），createTime 为发布时间（秒）。`,
		NeedsAppID: true,
		Flags:      snsPageFlags(),
	},
	{
		Use:   "friend-list",
		Path:  "/v1/sns/getContactSns",
		Short: "获取某个好友的朋友圈列表",
		Long: `获取某个好友的朋友圈主页信息列表。
分页：首次不传分页参数；第 2 页起传上次响应 data 中的 firstPageMd5 与 fromId。
响应 data.snsTips 可能为「朋友仅展示最近一个月的朋友圈」之类的提示，此时列表不完整属正常现象。`,
		NeedsAppID: true,
		Flags: append([]runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "好友（朋友圈发布人）wxid"},
		}, snsPageFlags()...),
	},
	{
		Use:        "detail",
		Path:       "/v1/sns/getSnsDetail",
		Short:      "获取某条朋友圈详情",
		Long:       "获取某条朋友圈的详情；返回结构与列表接口相同，常用于查看自己发布朋友圈的点赞与评论。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id（列表/发布接口返回）"},
		},
	},
	{
		Use:   "send-text",
		Path:  "/v1/sns/sendTextSns",
		Short: "发布纯文本朋友圈",
		Long: `发布一条纯文本朋友圈。
可见范围 visibility：1-公开 2-私密 3-部分可见 4-不给谁看；3 必须同时给出 --allow-tag-ids 或 --allow-wxids，4 必须给出 --disallow-tag-ids 或 --disallow-wxids。`,
		NeedsAppID: true,
		Flags: append(append([]runner.FlagSpec{
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Required: true, Usage: "朋友圈文本内容"},
			snsVisibilityFlag(),
		}, snsAudienceFlags()...), snsLocationFlag()),
	},
	{
		Use:   "send-image",
		Path:  "/v1/sns/sendImageSns",
		Short: "发布图文朋友圈",
		Long: `发布图文朋友圈，最多 9 张图片，单张不超过 5M（jpg/png/webp/bmp/tiff）。
--images 传本地图片路径（自动 base64），--image-urls 传图片 URL，二选一。
可见范围 visibility：1-公开 2-私密 3-部分可见 4-不给谁看；3 必须同时给出 --allow-tag-ids 或 --allow-wxids，4 必须给出 --disallow-tag-ids 或 --disallow-wxids。`,
		NeedsAppID: true,
		Flags: append(append([]runner.FlagSpec{
			{Name: "images", BodyKey: "imageList", Kind: runner.KindFileList, Required: true, Usage: "本地图片路径，多个用英文逗号分隔（自动 base64），与 --image-urls 二选一"},
			{Name: "image-urls", BodyKey: "imageList", Kind: runner.KindStringList, Required: true, Usage: "图片 URL 列表，多个用英文逗号分隔（http/https 开头），与 --images 二选一"},
			snsVisibilityFlag(),
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "朋友圈文本内容"},
		}, snsAudienceFlags()...), snsLocationFlag()),
	},
	{
		Use:   "send-video",
		Path:  "/v1/sns/sendVideoSns",
		Short: "发布视频朋友圈",
		Long: `发布视频朋友圈，单个视频不超过 200M；Mac 版发送视频时长不超过 30 秒。
--file 传本地视频（自动 base64），--url 传视频 URL，二选一。
可见范围 visibility：1-公开 2-私密 3-部分可见 4-不给谁看；3 必须同时给出 --allow-tag-ids 或 --allow-wxids，4 必须给出 --disallow-tag-ids 或 --disallow-wxids。`,
		NeedsAppID: true,
		Flags: append(append(append([]runner.FlagSpec{
			snsVisibilityFlag(),
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "朋友圈文本内容"},
		}, snsMediaFlags("videoData", "视频", true)...), snsAudienceFlags()...), snsLocationFlag()),
	},
	{
		Use:   "send-link",
		Path:  "/v1/sns/sendLinkSns",
		Short: "发布链接朋友圈",
		Long: `发布链接朋友圈，需提供链接标题、链接地址与缩略图（缩略图 ≤5M）。
--file 传本地缩略图（自动 base64），--url 传缩略图 URL，二选一。
可见范围 visibility：1-公开 2-私密 3-部分可见 4-不给谁看；3 必须同时给出 --allow-tag-ids 或 --allow-wxids，4 必须给出 --disallow-tag-ids 或 --disallow-wxids。`,
		NeedsAppID: true,
		Flags: append(append(append([]runner.FlagSpec{
			{Name: "title", BodyKey: "title", Kind: runner.KindString, Required: true, Usage: "链接显示标题"},
			{Name: "link-url", BodyKey: "linkUrl", Kind: runner.KindString, Required: true, Usage: "链接对应的 Url"},
			snsVisibilityFlag(),
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "朋友圈文本内容"},
		}, snsMediaFlags("imageData", "缩略图", true)...), snsAudienceFlags()...), snsLocationFlag()),
	},
	{
		Use:   "forward-link",
		Path:  "/v1/sns/forwardLinkSns",
		Short: "用链接朋友圈的 xmlMsg 转发到朋友圈",
		Long: `使用收到的链接朋友圈中的 xmlMsg 转发链接朋友圈。
可见范围规则同 send-text（3/4 必须给出对应标签或联系人）。`,
		NeedsAppID: true,
		Flags: append(append(append([]runner.FlagSpec{
			snsVisibilityFlag(),
		}, snsXMLFlags("链接朋友圈")...), append([]runner.FlagSpec{
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "朋友圈文本内容"},
		}, snsAudienceFlags()...)...), snsLocationFlag()),
	},
	{
		Use:   "forward-link-msg",
		Path:  "/v1/sns/forwardLinkMsgToSns",
		Short: "用链接类型聊天消息的 xmlMsg 转发到朋友圈",
		Long: `使用收到的链接类型聊天消息中的 xmlMsg 转发链接朋友圈（聊天消息 → 朋友圈）。
可见范围规则同 send-text（3/4 必须给出对应标签或联系人）。`,
		NeedsAppID: true,
		Flags: append(append(append([]runner.FlagSpec{
			snsVisibilityFlag(),
		}, snsXMLFlags("链接消息")...), append([]runner.FlagSpec{
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "朋友圈文本内容"},
		}, snsAudienceFlags()...)...), snsLocationFlag()),
	},
	{
		Use:   "forward-finder",
		Path:  "/v1/sns/forwardFinderSns",
		Short: "转发视频号消息/视频号朋友圈到朋友圈",
		Long: `使用接收到的视频号聊天消息或视频号朋友圈中的 xmlMsg 发视频号朋友圈。
可见范围规则同 send-text（3/4 必须给出对应标签或联系人）。`,
		NeedsAppID: true,
		Flags: append(append(append([]runner.FlagSpec{
			snsVisibilityFlag(),
		}, snsXMLFlags("视频号消息或视频号朋友圈")...), append([]runner.FlagSpec{
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "朋友圈文本内容"},
		}, snsAudienceFlags()...)...), snsLocationFlag()),
	},
	{
		Use:        "set-private",
		Path:       "/v1/sns/setSnsPrivate",
		Short:      "设置某条朋友圈为私密/公开",
		Long:       "为某条已发布的朋友圈设置私密状态；设备类型为 Mac 时不支持此操作。\n--is-private 必填且需显式指定：--is-private=true（设为私密）或 --is-private=false（取消私密）。",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
			{Name: "is-private", BodyKey: "isPrivate", Kind: runner.KindBool, Required: true, Usage: "是否设置为私密：true-私密 false-取消私密"},
		},
	},
	{
		Use:   "set-visibility",
		Path:  "/v1/sns/modifySnsVisibility",
		Short: "修改已发布朋友圈的可见范围",
		Long: `修改某条已发布朋友圈的可见范围；设备类型为 Mac 时不支持此操作。
visibility：1-公开 2-私密 3-部分可见 4-不给谁看；3 必须同时给出 --allow-tag-ids 或 --allow-wxids，4 必须给出 --disallow-tag-ids 或 --disallow-wxids。`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
			snsVisibilityFlag(),
			{Name: "allow-tag-ids", BodyKey: "allowTagIdList", Kind: runner.KindIntList, Usage: "部分可见的标签 Id 列表（visibility=3 时有效），多个用英文逗号分隔"},
			{Name: "allow-wxids", BodyKey: "allowUserList", Kind: runner.KindStringList, Usage: "部分可见的联系人 wxid 列表（visibility=3 时有效），多个用英文逗号分隔"},
			{Name: "disallow-tag-ids", BodyKey: "disallowTagIdList", Kind: runner.KindIntList, Usage: "不给谁看的标签 Id 列表（visibility=4 时有效），多个用英文逗号分隔"},
			{Name: "disallow-wxids", BodyKey: "disallowUserList", Kind: runner.KindStringList, Usage: "不给谁看的联系人 wxid 列表（visibility=4 时有效），多个用英文逗号分隔"},
		},
	},
	{
		Use:        "delete",
		Path:       "/v1/sns/delSnsPost",
		Short:      "删除某条已发布的朋友圈",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
		},
	},
	{
		Use:        "like",
		Path:       "/v1/sns/addSnsLike",
		Short:      "对某条朋友圈点赞",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
			{Name: "sns-wxid", BodyKey: "snsWxid", Kind: runner.KindString, Required: true, Usage: "朋友圈发布人 wxid"},
		},
	},
	{
		Use:        "unlike",
		Path:       "/v1/sns/delSnsLike",
		Short:      "取消对某条朋友圈的点赞",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
		},
	},
	{
		Use:   "comment",
		Path:  "/v1/sns/sendSnsComment",
		Short: "发布朋友圈评论（可带图、可回复他人）",
		Long: `发布朋友圈评论，--content 与图片（--file/--url，≤5M）至少提供一个；可回复他人评论。
回复他人评论时 --reply-comment-id 以及 --reply-wxid、--reply-nickname、--reply-create-time 都必须给出（取自列表接口 commentList）。`,
		NeedsAppID: true,
		Flags: append([]runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
			{Name: "sns-wxid", BodyKey: "snsWxid", Kind: runner.KindString, Required: true, Usage: "朋友圈发布人 wxid"},
			{Name: "content", BodyKey: "content", Kind: runner.KindString, Usage: "评论内容，与图片至少提供一个"},
			{Name: "reply-comment-id", BodyKey: "retplyCommentId", Kind: runner.KindInt, Usage: "所回复评论的 Id（接口字段名为 retplyCommentId）；有值时后三项也必填"},
			{Name: "reply-wxid", BodyKey: "replyCommentWxid", Kind: runner.KindString, Usage: "所回复评论的发布人 wxid"},
			{Name: "reply-nickname", BodyKey: "replyCommentNickname", Kind: runner.KindString, Usage: "所回复评论的发布人昵称"},
			{Name: "reply-create-time", BodyKey: "replyCommentCreateTime", Kind: runner.KindInt, Usage: "所回复评论的创建时间"},
		}, snsMediaFlags("imageData", "评论图片", false)...),
	},
	{
		Use:        "delete-comment",
		Path:       "/v1/sns/delSnsComment",
		Short:      "删除某条朋友圈评论",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "sns-id", BodyKey: "snsId", Kind: runner.KindInt, Required: true, Usage: "朋友圈 Id"},
			{Name: "comment-id", BodyKey: "commentId", Kind: runner.KindInt, Required: true, Usage: "评论 Id"},
		},
	},
	{
		Use:   "download-image",
		Path:  "/v1/sns/downloadSnsImage",
		Short: "下载图文朋友圈中的图片",
		Long: `下载某条图文朋友圈中的图片，响应 data.imageList 给出每张图的下载地址（7 天有效）。
支持 --out-dir 把全部图片下载到本地目录（按序保存为 1.jpg、2.png …）。`,
		NeedsAppID:    true,
		OutURLListKey: "imageList",
		Flags:         snsXMLFlags("图文朋友圈"),
	},
	{
		Use:   "download-comment-image",
		Path:  "/v1/sns/downloadSnsCommentImage",
		Short: "下载朋友圈评论中的图片",
		Long: `下载某条朋友圈评论中的图片，参数取自列表接口 commentList[].imageList。
--url 必填；--cdn-token/--key/--enc-idx 如评论图片信息中存在则一并传入。
支持 --out 将图片保存为本地文件（下载地址 7 天有效）。`,
		NeedsAppID: true,
		OutURLKey:  "imageUrl",
		Flags: []runner.FlagSpec{
			{Name: "url", BodyKey: "url", Kind: runner.KindString, Required: true, Usage: "评论图片信息中的 imageUrl"},
			{Name: "cdn-token", BodyKey: "token", Kind: runner.KindString, Usage: "评论图片信息中的 token（有则必传）"},
			{Name: "key", BodyKey: "key", Kind: runner.KindString, Usage: "评论图片信息中的 key（有则必传）"},
			{Name: "enc-idx", BodyKey: "encIdx", Kind: runner.KindString, Usage: "评论图片信息中的 encIdx（有则必传）"},
		},
	},
	{
		Use:   "download-video",
		Path:  "/v1/sns/downloadSnsVideo",
		Short: "下载视频朋友圈中的视频与封面图",
		Long: `下载某条视频朋友圈中的视频和视频封面图，响应 data 含 videoUrl/playLength/thumbUrl（均 7 天有效）。
支持 --out 保存视频；封面图地址见响应 thumbUrl。`,
		NeedsAppID: true,
		OutURLKey:  "videoUrl",
		Flags:      snsXMLFlags("视频朋友圈"),
	},
}

// newSnsGroup 生成朋友圈模块分组；命令组名 sns 对应接口路径前缀 /v1/sns。
func newSnsGroup(opts *runner.Options) *cobra.Command {
	return runner.NewGroup("sns", "朋友圈模块", snsSpecs, opts)
}

// snsPageFlags 朋友圈列表分页参数：首次不传，第 2 页起传上次响应中的 firstPageMd5 与 fromId。
func snsPageFlags() []runner.FlagSpec {
	return []runner.FlagSpec{
		{Name: "first-page-md5", BodyKey: "firstPageMd5", Kind: runner.KindString, Usage: "首次留空；第 2 页起传上次响应中的 firstPageMd5"},
		{Name: "from-id", BodyKey: "fromId", Kind: runner.KindInt, Usage: "首次传 0；第 2 页起传上次响应中的 fromId"},
	}
}

// snsVisibilityFlag 朋友圈可见范围（发布/转发/修改可见范围必填）。
func snsVisibilityFlag() runner.FlagSpec {
	return runner.FlagSpec{Name: "visibility", BodyKey: "visibility", Kind: runner.KindInt, Required: true, Usage: "可见范围：1-公开 2-私密 3-部分可见 4-不给谁看（3/4 必须给出对应的标签或联系人）"}
}

// snsAudienceFlags 可见范围细节（部分可见/不给谁看）与提醒谁看。
func snsAudienceFlags() []runner.FlagSpec {
	return []runner.FlagSpec{
		{Name: "allow-tag-ids", BodyKey: "allowTagIdList", Kind: runner.KindIntList, Usage: "部分可见的标签 Id 列表（visibility=3 时有效），多个用英文逗号分隔"},
		{Name: "allow-wxids", BodyKey: "allowUserList", Kind: runner.KindStringList, Usage: "部分可见的联系人 wxid 列表（visibility=3 时有效），多个用英文逗号分隔"},
		{Name: "disallow-tag-ids", BodyKey: "disallowTagIdList", Kind: runner.KindIntList, Usage: "不给谁看的标签 Id 列表（visibility=4 时有效），多个用英文逗号分隔"},
		{Name: "disallow-wxids", BodyKey: "disallowUserList", Kind: runner.KindStringList, Usage: "不给谁看的联系人 wxid 列表（visibility=4 时有效），多个用英文逗号分隔"},
		{Name: "with-wxids", BodyKey: "withUserList", Kind: runner.KindStringList, Usage: "提醒谁看的 wxid 列表，多个用英文逗号分隔"},
	}
}

// snsLocationFlag 位置信息（复杂对象，整体走 JSON）。
func snsLocationFlag() runner.FlagSpec {
	return runner.FlagSpec{
		Name:    "location",
		BodyKey: "locationInfo",
		Kind:    runner.KindJSON,
		Usage:   `位置信息 JSON（字符串或 @文件路径），如 {"poiName":"西湖","city":"杭州","latitude":30.25,"longitude":120.15,"poiClassifyId":"xxx","poiClassifyType":1}`,
	}
}

// snsXMLFlags 转发/下载类命令的 xmlMsgBase64 参数：--xml 直传 或 --xml-file 读文件（均自动 base64）。
func snsXMLFlags(what string) []runner.FlagSpec {
	return []runner.FlagSpec{
		{Name: "xml", BodyKey: "xmlMsgBase64", Kind: runner.KindXML, Required: true, XMLBase64: true, Usage: what + "的 xmlMsg 内容（自动 base64），与 --xml-file 二选一"},
		{Name: "xml-file", BodyKey: "xmlMsgBase64", Kind: runner.KindXMLFile, XMLBase64: true, Usage: "从文件读取 " + what + " 的 xmlMsg（自动 base64），与 --xml 二选一"},
	}
}

// snsMediaFlags 媒体参数：--file 本地文件（自动 base64）或 --url http(s) 地址，二选一。
func snsMediaFlags(bodyKey, what string, required bool) []runner.FlagSpec {
	return []runner.FlagSpec{
		{Name: "file", BodyKey: bodyKey, Kind: runner.KindFile, Required: required, Usage: what + "本地文件路径（自动 base64），与 --url 二选一"},
		{Name: "url", BodyKey: bodyKey, Kind: runner.KindString, Required: required, Usage: what + " URL（http/https 开头），与 --file 二选一"},
	}
}
