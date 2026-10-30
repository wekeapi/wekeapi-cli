// 账号信息记录：登录成功 / 在线状态检测返回个人信息时，把 wxid（及 appId）写入
// ~/.wekeapi/config.json；联系人、消息等业务数据统一由 wekeapi-msg-sync 服务
// 存储与查询，主 CLI 不再维护本地业务数据库。
// login check 登录成功后由 CLI 自动补调一次 status 接口（refreshAccountInfo），
// 保证个人信息统一采集。
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"wekeapi-cli/internal/client"
	"wekeapi-cli/internal/runner"
)

// accountInfo 当前登录账号的个人信息（仅用于更新配置中的账号标识）。
type accountInfo struct {
	WxID       string
	Nickname   string
	Alias      string
	Mobile     string
	Uin        string
	HeadImgURL string
}

// extractAccountInfo 从在线状态检测（login status）响应 data 提取账号个人信息。
// 响应为顶层扁平字段：onlineStatus/wxid/nickname/alias/headImgUrl（wxid 仅在线时返回）。
// 无 wxid（未登录/离线等场景）时返回 nil。
func extractAccountInfo(data map[string]any) *accountInfo {
	wxid := anyString(data["wxid"])
	if wxid == "" {
		return nil
	}
	return &accountInfo{
		WxID:       wxid,
		Nickname:   anyString(data["nickname"]),
		Alias:      anyString(data["alias"]),
		Mobile:     anyString(data["mobile"]),
		Uin:        anyString(data["uin"]),
		HeadImgURL: anyString(data["headImgUrl"]),
	}
}

// refreshAccountInfo 主动调用一次在线状态检测接口（checkOnlineStatus），
// 把响应中的账号信息记录到配置（wxid/appId）。
// 用于 login check 登录成功后统一采集个人信息；失败仅 stderr 警告，不影响调用方主结果。
func refreshAccountInfo(ctx context.Context, opts *runner.Options) error {
	cl := &client.Client{
		BaseURL: strings.TrimRight(opts.BaseURL, "/"),
		Token:   opts.Token,
		HTTP:    &http.Client{Timeout: opts.Timeout},
	}
	resp, _, err := cl.Do(ctx, "/v1/login/checkOnlineStatus", map[string]any{
		"appId":       opts.AppID,
		"autoConnect": false,
	})
	if err != nil {
		fmt.Fprintf(opts.Stderr, "warn: 登录状态检测失败，跳过账号信息记录: %v\n", err)
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(resp.Data))
	dec.UseNumber()
	var data map[string]any
	if err := dec.Decode(&data); err != nil {
		fmt.Fprintf(opts.Stderr, "warn: 解析登录状态响应失败，跳过账号信息记录: %v\n", err)
		return nil
	}
	return recordAccountInfo(opts, data)
}

// recordAccountInfo 把响应中的账号标识记录到配置文件：
// 更新 wxid（以及调用时的 appId，配置为空或不同时补齐）。
// 配置无变化时静默；有变化时提示。
func recordAccountInfo(opts *runner.Options, data map[string]any) error {
	acc := extractAccountInfo(data)
	if acc == nil {
		return nil // 响应无个人信息（如离线状态），跳过
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	changed := false
	if cfg.WxID != acc.WxID {
		cfg.WxID = acc.WxID
		changed = true
	}
	if opts.AppID != "" && cfg.AppID != opts.AppID {
		cfg.AppID = opts.AppID
		changed = true
	}
	if !changed {
		return nil
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	path, _ := configPath()
	fmt.Fprintf(opts.Stderr, "账号信息已记录：wxid=%s appId=%s（已保存到 %s）\n", acc.WxID, cfg.AppID, path)
	return nil
}
