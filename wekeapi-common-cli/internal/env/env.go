// Package env 保存编译期确定的运行环境配置（线上）。
//
// 本包是地址的唯一来源，代码中不得硬编码。默认编译即线上环境，无需任何开关；
// 团队内部测试构建可用 `go build -ldflags "-X ..."` 覆盖下面的字符串变量
// （Name / BaseURL / SiteURL / ProxyDownloadBase），源码中不提供环境切换开关。
package env

// ProxyDownload 一个平台的代理工具下载信息。
type ProxyDownload struct {
	Platform string `json:"platform"`       // 平台描述（Windows x64 / Windows ARM64 / macOS Intel / macOS ARM / Linux）
	URL      string `json:"url"`            // 下载地址（Linux 为安装脚本地址）
	Note     string `json:"note,omitempty"` // 附加说明（Linux 为一键安装命令）
}

var (
	// Name 环境名（默认线上 prod）。
	Name = "prod"
	// BaseURL API 基础地址默认值。
	BaseURL = "https://api.wekeapi.com/api"
	// SiteURL 官网地址（申请 AccessToken 入口）。
	SiteURL = "https://www.wekeapi.com"
	// ProxyDownloadBase 本地代理工具下载根地址（编译期可覆盖）。
	ProxyDownloadBase = "https://support.wekeapi.com/release/wekeapi-proxy"
	// ProxyDownloads 各平台本地代理工具下载地址。
	ProxyDownloads = proxyDownloads(ProxyDownloadBase)
)

// proxyDownloads 按下载根地址拼接各平台代理工具下载信息。
func proxyDownloads(base string) []ProxyDownload {
	return []ProxyDownload{
		{Platform: "Windows x64", URL: base + "/wekeapi-proxy_v1.0.2_windows_amd64.zip"},
		{Platform: "Windows ARM64", URL: base + "/wekeapi-proxy_v1.0.2_windows_arm64.zip"},
		{Platform: "macOS Intel", URL: base + "/wekeapi-proxy_v1.0.1_mac_amd64.zip"},
		{Platform: "macOS ARM", URL: base + "/wekeapi-proxy_v1.0.1_mac_arm64.zip"},
		{Platform: "Linux", URL: base + "/install.sh", Note: "curl -sSL " + base + "/install.sh | sudo bash"},
	}
}
