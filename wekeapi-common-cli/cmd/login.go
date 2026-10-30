package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/runner"
)

// anyInt64 把 PostRun data 中的数字字段（json.Number/float64）转为 int64。
func anyInt64(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	}
	return 0
}

// anyString 把 PostRun data 中的字段（string/json.Number）转为 string。
func anyString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	}
	return ""
}

var loginSpecs = []runner.CmdSpec{
	{
		Use:   "qr",
		Path:  "/v1/login/getLoginQrCode",
		Short: "绑定微信账号的第一步，调用此接口获取登录二维码",
		Long: `绑定微信账号的第一步，调用此接口获取登录二维码；将二维码展示给用户，由用户使用手机微信扫码登录。
扫码期间定时调用“检测登录状态”接口获取扫码结果，推荐时间间隔5秒。
appId 相当于设备ID：新账号第一次获取二维码时传空，系统会自动生成，请做好记录；
账号退出登录或掉线后再次获取二维码，务必使用上次的 appId，否则会作为新设备登录，容易触发风控。
本机配置中已有 appId 时默认复用它；确需按全新设备登录（重新生成 appId），用 --new-device
（等价于显式空值 --app-id=，但无需 shell 传空字符串，可规避 Windows PowerShell 丢弃 "" 参数的问题），
并先向用户确认（视为新设备登录，有风控风险）。

deviceType 登录设备类型，值为 Mac/iPad：
Mac 登录过程更简便，遇到登录需要验证的情况支持自动验证，
但会占用 PC 登录名额，无法在电脑上再登录该账号；
iPad 只占用 Pad 登录名额，不影响您在电脑上登录该账号。
注意：设备类型以 appId 首次生成时绑定的类型为准；携带已有 appId 再次获取二维码时，
--device-type 不改变该设备的类型。 appId 仅在登录成功（login check status=2）后自动保存到配置；
登录成功前请自行记录 appId，并在后续命令显式传 --app-id。

二维码有效期只有 3 分多钟，多次获取二维码或登录失败可能使账号处于高风险，
请在条件具备（登录参数已确认）后再取码；新设备首次登录建议由人工完成。

verifyType 登录验证方式，值为 manual/auto：
仅设备类型为 Mac 时支持自动验证，验证成功率高，但未达到 100%；
如果连续 2 次自动验证失败，请勿继续使用自动验证，改用手动验证再次尝试；
手动验证需使用专用扫码验证工具，请提前联系客服(weke_820528)准备好。

proxyInfo 与代理类型对应传值：
  local：传本地代理的代理ID
  self：传代理的 Url
  platform：传账号所在城市代码
城市代码参见文档：https://doc.wekeapi.com/%E7%99%BB%E5%BD%95%E6%A8%A1%E5%9D%97%E5%8F%82%E8%80%83-8859316m0`,
		OptionalAppID: true,
		NewDeviceFlag: true,
		Flags: []runner.FlagSpec{
			{Name: "device-type", BodyKey: "deviceType", Kind: runner.KindString, Required: true, Usage: "登录设备类型，可选值 Mac/iPad（Mac 更简便但占用 PC 登录名额；iPad 只占 Pad 名额，不影响电脑登录）"},
			{Name: "verify-type", BodyKey: "verifyType", Kind: runner.KindString, Required: true, Usage: "登录验证方式，可选值 manual/auto（仅 Mac 支持 auto；手动验证需专用扫码验证工具）"},
			{Name: "proxy-type", BodyKey: "proxyType", Kind: runner.KindString, Required: true, Usage: "代理类型，可选值 local（本地代理，推荐）/self（自有代理）/platform（平台代理）"},
			{Name: "proxy-info", BodyKey: "proxyInfo", Kind: runner.KindString, Required: true, Usage: "代理信息，与代理类型对应传值：local 传代理ID，self 传代理Url，platform 传城市代码"},
		},
		OutBase64Key: "qrCodeImgData",
	},
	{
		Use:   "check",
		Path:  "/v1/login/checkLogin",
		Short: "获取登录二维码或快捷登录后，定时检测用户登录状态",
		Long: `获取登录二维码或调用快捷登录后，定时调用本接口检测用户登录状态，推荐时间间隔5秒。
登录状态：0-未扫码，1-已扫码未登录，2-已登录，3-登录失败，10-已扫码需要验证，11-自动验证已通过需用户在手机微信点击“确定”完成登录。
状态10表示触发身份验证，接口会返回验证用二维码，需账号持有人使用专用扫码验证工具验证。
状态2（成功）或3（失败）表示本次登录已结束，请停止定时检测；状态3同时会返回错误信息。
登录成功后本次使用的 appId 自动保存到 ~/.wekeapi/config.json；CLI 随后自动调用一次在线状态检测，
把返回的账号个人信息（wxid/昵称/别名/头像）统一写入配置与该账号分库（~/.wekeapi/data-<wxid>.db）的 accounts 表。`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "qr-code-uuid", BodyKey: "qrCodeUUID", Kind: runner.KindString, Required: true, Usage: "二维码对应的UUID"},
		},
		// status=2（登录成功）时：appId 先落盘配置，再补调一次在线状态检测采集个人信息，
		// 最后把凭证写入 wekeapi-msg-sync.json 并确保服务运行（已运行且凭证未变则不重启）。
		PostRun: func(cmd *cobra.Command, opts *runner.Options, _ map[string]any, data map[string]any) error {
			return notifyLoginResult(cmd.Context(), opts, data)
		},
	},
	{
		Use:   "status",
		Path:  "/v1/login/checkOnlineStatus",
		Short: "检查账号在线状态（1-在线，-1-离线，0-未登录）",
		Long: `检查账号在线状态，返回的在线状态：1-在线，-1-离线，0-未登录。
请求体固定携带 autoConnect（API 必填字段）：默认 false 不自动重连；显式传 --auto-connect 时，
检测到掉线会尝试自动重连，然后返回重连后的在线状态，可用于账号状态的日常巡检。
在线时响应携带账号个人信息（wxid/昵称/别名/头像，顶层字段）：CLI 会把 wxid/appId 更新到 ~/.wekeapi/config.json，
并确保 wekeapi-msg-sync 数据同步服务已按最新凭证运行（未运行时自动启动）；离线（-1）只更新配置，不启动服务。
本地还没有 wekeapi-msg-sync.json 时，流程与登录成功后一致：先创建配置再启动服务（触发全量同步），
交互终端会先询问数据库类型（回车默认 SQLite，也可选 MySQL 并输入连接串）；
非交互场景（脚本/skill）不提示，按默认 SQLite 创建（之后可用 wekeapi data config 修改）。`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "auto-connect", BodyKey: "autoConnect", Kind: runner.KindBool, Default: false, Usage: "掉线时是否自动重连（默认 false，不重连；请求体固定携带该字段）"},
		},
		// 响应携带个人信息时：wxid/appId 更新到配置，并确保数据同步服务运行。
		// 在线（onlineStatus=1）且本地尚无服务配置时，与登录成功后一致：
		// 首次创建 wekeapi-msg-sync.json（交互终端会先询问数据库类型，默认 SQLite）并启动服务。
		PostRun: func(cmd *cobra.Command, opts *runner.Options, _ map[string]any, data map[string]any) error {
			if err := recordAccountInfo(opts, data); err != nil {
				return err
			}
			if anyInt64(data["onlineStatus"]) != 1 {
				return nil
			}
			if anyString(data["wxid"]) == "" {
				return nil
			}
			return syncMsgService(cmd.Context(), opts)
		},
	},
	{
		Use:   "quick",
		Path:  "/v1/login/dialogLogin",
		Short: "快捷登录：不用扫码，只需在手机弹出页面上点击确认登录",
		Long: `账号在手机端或通过 API 主动退出登录后，再次登录时可调用此接口，不用扫码，只需在手机弹出页面上点击确认登录。
调用后使用响应中的 qrCodeUUID，定时调用“检测登录状态”接口获取登录状态。
如果返回 -24（登录失败），需重新走获取二维码、扫码登录、检测登录状态的流程。
以下情况无法使用本接口：新设备登录后掉线、官方风控下线等被动退出登录的场景。`,
		NeedsAppID: true,
	},
	{
		Use:        "logout",
		Path:       "/v1/login/logout",
		Short:      "主动退出登录状态",
		NeedsAppID: true,
	},
}
