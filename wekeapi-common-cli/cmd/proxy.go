package cmd

import "wekeapi-cli/internal/runner"

var proxySpecs = []runner.CmdSpec{
	{
		Use:        "get",
		Path:       "/v1/proxy/getProxy",
		Short:      "获取某个设备当前代理信息",
		NeedsAppID: true,
	},
	{
		Use:   "set",
		Path:  "/v1/proxy/setProxy",
		Short: "设置某个设备的代理；代理与之前不同，相当于修改代理",
		Long: `设置某个设备的代理；代理与之前不同，相当于修改代理。
如果设备上登录的账号当前在线，新代理立即生效；不在线，则等下次登录或重连后生效。
如果新代理网络不通，会返回 -51: 网络连接异常。

proxyType 代理类型：local-本地代理，self-自有代理，platform-平台代理。
proxyInfo 与代理类型对应传值：
  local：传本地代理的代理ID
  self：传代理的Url
  platform：传账号所在城市代码
城市代码参见文档：https://doc.wekeapi.com/%E7%99%BB%E5%BD%95%E6%A8%A1%E5%9D%97%E5%8F%82%E8%80%83-8859316m0`,
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "proxy-type", BodyKey: "proxyType", Kind: runner.KindString, Required: true, Usage: "代理类型，local-本地代理，self-自有代理，platform-平台代理"},
			{Name: "proxy-info", BodyKey: "proxyInfo", Kind: runner.KindString, Required: true, Usage: "代理信息，与代理类型对应传值：local 传本地代理ID，self 传代理Url，platform 传账号所在城市代码"},
		},
	},
	{
		Use:        "check-city",
		Path:       "/v1/proxy/checkCityProxy",
		Short:      "检查某个城市当前是否有可用代理",
		NeedsAppID: true,
		Flags: []runner.FlagSpec{
			{Name: "city-code", BodyKey: "cityCode", Kind: runner.KindString, Required: true, Usage: "城市代码，参见文档：https://doc.wekeapi.com/%E7%99%BB%E5%BD%95%E6%A8%A1%E5%9D%97%E5%8F%82%E8%80%83-8859316m0"},
		},
	},
}
