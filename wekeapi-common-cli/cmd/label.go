package cmd

import "wekeapi-cli/internal/runner"

var labelSpecs = []runner.CmdSpec{
	{
		Use:        "list",
		Path:       "/v1/label/fetchLabelList",
		Short:      "获取微信标签列表",
		Long:       "获取微信标签列表；手机上增删标签不会自动同步到平台，如果在手机上加了新标签，请主动调用此接口进行同步",
		NeedsAppID: true,
		Flags:      []runner.FlagSpec{},
	},
	{
		Use:        "add",
		Path:       "/v1/label/addLabel",
		Short:      "添加微信标签",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "labels", BodyKey: "labels", Kind: runner.KindStringList, Required: true, Usage: "新增标签列表，格式为字符串数组，多个用英文逗号分隔"},
		},
	},
	{
		Use:        "delete",
		Path:       "/v1/label/deleteLabel",
		Short:      "删除微信标签",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "label-id", BodyKey: "labelId", Kind: runner.KindInt, Required: true, Usage: "待删除的标签ID"},
		},
	},
	{
		Use:        "set-friend",
		Path:       "/v1/label/modifyFriendLabel",
		Short:      "修改好友的微信标签",
		Long:       "修改好友的微信标签；修改时，请将所有微信标签放在请求里一起提交",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "wxid", BodyKey: "wxid", Kind: runner.KindString, Required: true, Usage: "待修改好友的wxid"},
			{Name: "label-ids", BodyKey: "labelIds", Kind: runner.KindIntList, Required: true, Usage: "该好友的所有标签，格式为int数组，多个用英文逗号分隔"},
		},
	},
}
