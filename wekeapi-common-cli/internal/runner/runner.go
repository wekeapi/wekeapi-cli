// Package runner 将声明式命令规格（CmdSpec）转换为 cobra.Command，
// 统一处理 flag 绑定、body 构建、文件 base64、XML 处理、--out 下载与输出/退出码契约。
package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/client"
	"wekeapi-cli/internal/env"
)

// FlagKind 标记 flag 的值类型与进入 body 的方式。
type FlagKind int

const (
	KindString     FlagKind = iota // 字符串原样进入 body
	KindInt                        // int64
	KindFloat                      // float64
	KindBool                       // bool
	KindStringList                 // 逗号分隔字符串 -> []string
	KindIntList                    // 逗号分隔数字 -> []int64
	KindFile                       // 本地文件路径 -> 读取并 base64 后进入 body
	KindFileList                   // 逗号分隔的本地文件路径 -> 逐个读取并 base64 后作为 []string 进入 body
	KindXML                        // XML 字符串 -> 按 XMLBase64 决定是否 base64
	KindXMLFile                    // 从文件读取 XML -> 按 XMLBase64 决定是否 base64
	KindJSON                       // JSON 字符串或 @文件 -> 解析后原样进入 body（用于数组/对象字段）
)

// FlagSpec 描述一个命令 flag 及其对应的 body 字段。
type FlagSpec struct {
	Name      string   // flag 名，如 to-wxid
	BodyKey   string   // JSON body 字段名，如 toWxid
	Kind      FlagKind // 值类型，默认 KindString
	Required  bool     // body 中必须存在该字段（可由 --data 提供）
	Usage     string   // 参数说明（写入 --help）
	XMLBase64 bool     // 仅 KindXML/KindXMLFile：是否对 XML 做 base64

	// Default 为 flag 未显式传入时写入 body 的默认值（nil 表示不写入）。
	// 用于 API 必填但 CLI 可缺省的字段（如 login status 的 autoConnect 固定传 false）。
	// --data 中已显式提供的同名字段优先，默认值仅补缺失。
	Default any
}

// PostRunFunc API 成功（code==0）后的副作用回调：body 为请求体，data 为响应 data。
// data 中数字为 json.Number（保留大整数精度，如 newMsgId）。
// 用于副作用（如 login check 保存 appId、message send 消息落库）；失败仅警告，不影响 stdout JSON 与退出码。
type PostRunFunc func(cmd *cobra.Command, opts *Options, body, data map[string]any) error

// CmdSpec 描述一个 API 端点对应的命令。
type CmdSpec struct {
	Use   string // 命令用法，如 "send-text"
	Path  string // API 路径，如 "/v1/message/sendText"
	Short string // 中文简述
	Long  string // 可选：详细说明（写入 --help）

	NeedsAppID    bool       // body 必须有 appId 字段（取全局 --app-id，未提供则报错）
	OptionalAppID bool       // body 可携带 appId：仅当全局 --app-id 提供时写入
	Flags         []FlagSpec // 命令参数

	// DeriveOri 为转发类命令（使用发送响应转发）开启：从 --data 的发送响应中
	// 自动补全 API 必需的 ori 字段——oriMsgId<-newMsgId、oriMsgSourceBase64<-msgSourceBase64、
	// oriMsgTo<-toWxid（原接收人）、oriMsgFrom<-当前账号 wxid。仅补缺失字段，显式参数优先。
	DeriveOri bool

	// NewDeviceFlag 为 true 时该命令额外提供 --new-device 开关：忽略配置与 --app-id 中的值，
	// 本次请求按全新设备处理（body 中 appId 显式传空，由服务端生成新 appId）。
	// 用于「新设备登录」：不依赖 shell 传空字符串（Windows PowerShell 5.1 会丢弃 "" 参数）。
	NewDeviceFlag bool

	// --out 支持（三选一）：设置后命令自动获得 --out flag。
	OutBase64Key string // data 中包含 base64（可为 data URI）的字段名，--out 时解码保存
	OutURLKey    string // data 中包含下载 URL 的字段名，--out 时下载保存
	// OutURLListKey 用于响应中是「多个下载地址」的字段（数组，元素含 imageUrl/url/videoUrl 与可选 format）：
	// 命令自动获得 --out-dir flag，按序把每个地址下载到该目录（1.jpg、2.png …）。
	OutURLListKey string

	// PostRun 可选钩子：API 成功（code==0）后执行，用于保存 appId、发送消息落库等副作用。
	PostRun PostRunFunc
}

// Options 全局选项（由根命令 flag 填充）。
type Options struct {
	Token    string
	AppID    string
	AppIDSet bool // 是否显式提供过 --app-id（显式传空如 --app-id= 时 AppID 为空但 AppIDSet 为 true）
	BaseURL  string
	Timeout  time.Duration
	Pretty   bool
	Data     string // 透传 JSON body（字符串或 @文件路径）
	WxID     string // 当前登录账号 wxid（来自配置 ~/.wekeapi/config.json），用于转发类命令补全 oriMsgFrom

	Stdout io.Writer
	Stderr io.Writer
}

// ExitError 携带退出码的错误。
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

// Build 根据规格生成 cobra.Command。
func Build(spec CmdSpec, opts *Options) *cobra.Command {
	c := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  buildLong(spec),
		// 关闭 cobra 默认的错误用法输出，保持 stderr 契约干净
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	for _, f := range spec.Flags {
		usage := f.Usage
		if f.Required {
			usage += "（必填）"
		}
		switch f.Kind {
		case KindInt:
			c.Flags().Int64(f.Name, 0, usage)
		case KindIntList:
			c.Flags().String(f.Name, "", usage)
		case KindFloat:
			c.Flags().Float64(f.Name, 0, usage)
		case KindBool:
			c.Flags().Bool(f.Name, false, usage)
		default:
			c.Flags().String(f.Name, "", usage)
		}
	}
	if spec.OutBase64Key != "" || spec.OutURLKey != "" {
		c.Flags().String("out", "", "将结果中的文件保存到本地路径")
	}
	if spec.OutURLListKey != "" {
		c.Flags().String("out-dir", "", "将结果中的多个文件按序下载保存到该本地目录")
	}
	if spec.NewDeviceFlag {
		c.Flags().Bool("new-device", false, "按全新设备处理：忽略配置中的 appId，用空 appId 请求（服务端会生成新 appId，有风控风险）")
	}

	c.RunE = func(cmd *cobra.Command, args []string) error {
		return run(cmd, spec, opts)
	}
	return c
}

// NewGroup 生成模块分组命令，如 login / message。
func NewGroup(use, short string, specs []CmdSpec, opts *Options) *cobra.Command {
	g := &cobra.Command{
		Use:           use,
		Short:         short,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	for _, s := range specs {
		g.AddCommand(Build(s, opts))
	}
	return g
}

func run(cmd *cobra.Command, spec CmdSpec, opts *Options) error {
	if opts.Token == "" {
		err := fmt.Errorf("缺少 AccessToken：请到官网 %s 申请，然后运行 wekeapi init --token <AccessToken> 保存配置（保存后可免传 --token）", env.SiteURL)
		fmt.Fprintf(opts.Stderr, "error: %v\n", err)
		return &ExitError{Code: 2, Err: err}
	}

	body, err := buildBody(cmd, spec, opts)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "error: %v\n", err)
		return &ExitError{Code: 2, Err: err}
	}

	cl := &client.Client{
		BaseURL: strings.TrimRight(opts.BaseURL, "/"),
		Token:   opts.Token,
		HTTP:    &http.Client{Timeout: opts.Timeout},
		OnSyncRetry: func(attempt int, wait time.Duration) {
			fmt.Fprintf(opts.Stderr, "通讯录同步进行中，等待 %s 后重试（第 %d 次）...\n", wait, attempt)
		},
	}

	resp, raw, err := cl.Do(cmd.Context(), spec.Path, body)
	if err != nil {
		// 业务错误：stdout 仍输出原始 JSON，stderr 输出摘要，退出码 1
		var bizErr *client.BizError
		if ok := asError(err, &bizErr); ok {
			writeJSON(opts.Stdout, bizErr.Raw, opts.Pretty)
			fmt.Fprintf(opts.Stderr, "api error: code=%d message=%s\n", bizErr.Code, bizErr.Message)
			return &ExitError{Code: 1, Err: bizErr}
		}
		fmt.Fprintf(opts.Stderr, "error: %v\n", err)
		return &ExitError{Code: 2, Err: err}
	}

	// --out：保存文件到本地
	if out := cmd.Flags().Lookup("out"); out != nil && out.Changed {
		outPath := out.Value.String()
		if outPath == "" {
			fmt.Fprintf(opts.Stderr, "error: --out 路径不能为空\n")
			return &ExitError{Code: 2, Err: fmt.Errorf("--out 路径不能为空")}
		}
		if err := saveOut(cmd.Context(), cl, resp, spec, outPath); err != nil {
			fmt.Fprintf(opts.Stderr, "error: %v\n", err)
			return &ExitError{Code: 2, Err: err}
		}
	}

	// --out-dir：批量保存下载地址列表
	if outDir := cmd.Flags().Lookup("out-dir"); outDir != nil && outDir.Changed {
		dir := outDir.Value.String()
		if dir == "" {
			fmt.Fprintf(opts.Stderr, "error: --out-dir 路径不能为空\n")
			return &ExitError{Code: 2, Err: fmt.Errorf("--out-dir 路径不能为空")}
		}
		if err := saveOutDir(cmd.Context(), cl, resp, spec, dir); err != nil {
			fmt.Fprintf(opts.Stderr, "error: %v\n", err)
			return &ExitError{Code: 2, Err: err}
		}
	}

	// PostRun 钩子：成功响应后的副作用（如保存 appId、消息落库）；失败仅警告。
	if spec.PostRun != nil {
		var data map[string]any
		if resp.Data != nil {
			dec := json.NewDecoder(bytes.NewReader(resp.Data))
			dec.UseNumber() // 保留大整数精度（如 newMsgId 超出 float64 安全范围）
			if err := dec.Decode(&data); err != nil {
				fmt.Fprintf(opts.Stderr, "warn: parse data for post-run: %v\n", err)
			}
		}
		if err := spec.PostRun(cmd, opts, body, data); err != nil {
			fmt.Fprintf(opts.Stderr, "warn: %v\n", err)
		}
	}

	writeJSON(opts.Stdout, raw, opts.Pretty)
	return nil
}

func buildBody(cmd *cobra.Command, spec CmdSpec, opts *Options) (map[string]any, error) {
	body := map[string]any{}

	// 1. --data 透传 body（JSON 字符串或 @文件）
	if opts.Data != "" {
		raw := opts.Data
		if strings.HasPrefix(raw, "@") {
			b, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
			if err != nil {
				return nil, fmt.Errorf("读取 --data 文件失败: %w", err)
			}
			raw = string(b)
		}
		// 用 json.Number 保留大整数精度（如 newMsgId、createTime 超出 float64 安全范围）
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			hint := ""
			if !strings.Contains(raw, `"`) {
				// 典型的 Windows PowerShell 5.1 行为：调用原生 exe 时剥离参数中的双引号，
				// 导致原本合法的 JSON 到达 CLI 时已无引号。
				hint = "；参数中的双引号疑似被 shell 剥离（Windows PowerShell 5.1 常见），Windows 下建议改用 --data @文件路径"
			}
			return nil, fmt.Errorf("--data 不是合法 JSON object: %w%s", err, hint)
		}
		if body == nil {
			body = map[string]any{}
		}
		// 若是完整的 API 响应包裹（含 code，且 data 为对象），自动解包出 data，
		// 便于直接把发送接口的 stdout 响应文件用于转发命令（--data @resp.json）。
		if _, hasCode := body["code"]; hasCode {
			if inner, ok := body["data"].(map[string]any); ok {
				body = inner
			}
		}
	}

	// 1b. 转发类命令：从发送响应补全 API 必需的 ori 字段（仅补缺失，显式参数优先）
	if spec.DeriveOri {
		deriveOri(body, opts)
	}

	// 2. appId。显式空值（AppIDSet 为 true 且值为空）表示「按新设备处理」，需把空值写入 body：
	//    服务端实测接受空 appId 并自动生成（login qr 新设备、proxy check-city 首次登录前检测均走此路径）。
	//    --new-device 与 --app-id= 等效，但前者不依赖 shell 传空字符串（PowerShell 5.1 会丢弃 "" 参数）。
	appID, appIDSet := opts.AppID, opts.AppIDSet
	if fl := cmd.Flags().Lookup("new-device"); fl != nil && fl.Changed {
		appID, appIDSet = "", true
	}
	if _, ok := body["appId"]; !ok {
		switch {
		case spec.NeedsAppID:
			if !appIDSet && appID == "" {
				return nil, fmt.Errorf("该命令需要 --app-id（设备 appId）：请传 --app-id，或先运行 wekeapi init 保存配置")
			}
			body["appId"] = appID
		case spec.OptionalAppID:
			if appIDSet || appID != "" {
				body["appId"] = appID
			}
		}
	}

	// 3. 具名 flag（优先于 --data）
	setBy := map[string]string{} // bodyKey -> flagName，用于冲突检测
	for _, f := range spec.Flags {
		fl := cmd.Flags().Lookup(f.Name)
		if fl == nil || !fl.Changed {
			// 未显式传入：写入声明的默认值（--data 已提供的字段优先）
			if f.Default != nil {
				if _, ok := body[f.BodyKey]; !ok {
					body[f.BodyKey] = f.Default
				}
			}
			continue
		}
		if prev, ok := setBy[f.BodyKey]; ok {
			return nil, fmt.Errorf("--%s 与 --%s 不能同时指定（对应同一字段 %s）", prev, f.Name, f.BodyKey)
		}
		setBy[f.BodyKey] = f.Name

		v, err := flagValue(cmd, f)
		if err != nil {
			return nil, err
		}
		body[f.BodyKey] = v
	}

	// 4. 必填校验
	for _, f := range spec.Flags {
		if !f.Required {
			continue
		}
		v, ok := body[f.BodyKey]
		if !ok || v == nil || v == "" {
			return nil, fmt.Errorf("缺少必填参数 --%s（%s）", f.Name, f.BodyKey)
		}
	}
	return body, nil
}

// deriveOri 为转发类命令从发送响应补全 API 必需的 ori 字段；仅当目标字段缺失时写入，
// 因此显式 --ori-* 参数（或 --data 中已给出的同名字段）始终优先。
func deriveOri(body map[string]any, opts *Options) {
	if _, ok := body["oriMsgId"]; !ok {
		if v, ok := body["newMsgId"]; ok {
			body["oriMsgId"] = v
		}
	}
	if _, ok := body["oriMsgSourceBase64"]; !ok {
		if v, ok := body["msgSourceBase64"]; ok {
			body["oriMsgSourceBase64"] = v
		}
	}
	if _, ok := body["oriMsgTo"]; !ok {
		if v, ok := body["toWxid"]; ok {
			body["oriMsgTo"] = v
		}
	}
	if _, ok := body["oriMsgFrom"]; !ok && opts.WxID != "" {
		body["oriMsgFrom"] = opts.WxID
	}
}

func flagValue(cmd *cobra.Command, f FlagSpec) (any, error) {
	switch f.Kind {
	case KindInt:
		return cmd.Flags().GetInt64(f.Name)
	case KindFloat:
		return cmd.Flags().GetFloat64(f.Name)
	case KindBool:
		return cmd.Flags().GetBool(f.Name)
	case KindStringList:
		s, _ := cmd.Flags().GetString(f.Name)
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	case KindIntList:
		s, _ := cmd.Flags().GetString(f.Name)
		var out []int64
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			var n int64
			if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
				return nil, fmt.Errorf("--%s 含非法数字 %q", f.Name, p)
			}
			out = append(out, n)
		}
		return out, nil
	case KindFile:
		path, _ := cmd.Flags().GetString(f.Name)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取文件失败 --%s %s: %w", f.Name, path, err)
		}
		return base64.StdEncoding.EncodeToString(b), nil
	case KindFileList:
		s, _ := cmd.Flags().GetString(f.Name)
		out := []string{}
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("读取文件失败 --%s %s: %w", f.Name, p, err)
			}
			out = append(out, base64.StdEncoding.EncodeToString(b))
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("--%s 未包含任何文件路径", f.Name)
		}
		return out, nil
	case KindXML:
		s, _ := cmd.Flags().GetString(f.Name)
		if f.XMLBase64 {
			return base64.StdEncoding.EncodeToString([]byte(s)), nil
		}
		return s, nil
	case KindXMLFile:
		path, _ := cmd.Flags().GetString(f.Name)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取文件失败 --%s %s: %w", f.Name, path, err)
		}
		if f.XMLBase64 {
			return base64.StdEncoding.EncodeToString(b), nil
		}
		return string(b), nil
	case KindJSON:
		s, _ := cmd.Flags().GetString(f.Name)
		if strings.HasPrefix(s, "@") {
			b, err := os.ReadFile(strings.TrimPrefix(s, "@"))
			if err != nil {
				return nil, fmt.Errorf("读取文件失败 --%s %s: %w", f.Name, s, err)
			}
			s = string(b)
		}
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil, fmt.Errorf("--%s 不是合法 JSON: %w", f.Name, err)
		}
		return v, nil
	default: // KindString
		return cmd.Flags().GetString(f.Name)
	}
}

func saveOut(ctx context.Context, cl *client.Client, resp *client.Response, spec CmdSpec, outPath string) error {
	var data map[string]any
	if resp.Data != nil {
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			return fmt.Errorf("解析响应 data 失败: %w", err)
		}
	}

	var content []byte
	switch {
	case spec.OutBase64Key != "":
		s, _ := data[spec.OutBase64Key].(string)
		if s == "" {
			return fmt.Errorf("响应中缺少字段 %s，无法保存", spec.OutBase64Key)
		}
		// 兼容 data URI 前缀，如 data:image/jpg;base64,...
		if i := strings.Index(s, ";base64,"); i >= 0 {
			s = s[i+len(";base64,"):]
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return fmt.Errorf("base64 解码 %s 失败: %w", spec.OutBase64Key, err)
		}
		content = b
	case spec.OutURLKey != "":
		url, _ := data[spec.OutURLKey].(string)
		if url == "" {
			return fmt.Errorf("响应中缺少字段 %s，无法下载", spec.OutURLKey)
		}
		b, err := cl.Download(ctx, url)
		if err != nil {
			return err
		}
		content = b
	default:
		return fmt.Errorf("该命令不支持 --out")
	}

	if err := os.WriteFile(outPath, content, 0o644); err != nil {
		return fmt.Errorf("写入文件 %s 失败: %w", outPath, err)
	}
	return nil
}

// saveOutDir 用于响应 data 中给出「多个下载地址」的接口（如朋友圈图片列表）：
// 逐个下载后保存为 <序号><扩展名>（1.jpg、2.png …），目录不存在时自动创建。
func saveOutDir(ctx context.Context, cl *client.Client, resp *client.Response, spec CmdSpec, dir string) error {
	var data map[string]any
	if resp.Data != nil {
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			return fmt.Errorf("解析响应 data 失败: %w", err)
		}
	}
	items, _ := data[spec.OutURLListKey].([]any)
	if len(items) == 0 {
		return fmt.Errorf("响应中缺少字段 %s（或为空），无法保存", spec.OutURLListKey)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
	}
	saved := 0
	for _, it := range items {
		item, _ := it.(map[string]any)
		url, format := itemURL(item)
		if url == "" {
			continue
		}
		b, err := cl.Download(ctx, url)
		if err != nil {
			return err
		}
		saved++
		name := filepath.Join(dir, fmt.Sprintf("%d%s", saved, fileExt(format, url)))
		if err := os.WriteFile(name, b, 0o644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", name, err)
		}
	}
	if saved == 0 {
		return fmt.Errorf("响应中 %s 没有可下载的地址", spec.OutURLListKey)
	}
	return nil
}

// itemURL 从下载列表元素中取出下载地址与文件格式（各接口字段名不统一：imageUrl/url/videoUrl…）。
func itemURL(item map[string]any) (url, format string) {
	for _, k := range []string{"imageUrl", "url", "videoUrl", "fileUrl", "thumbUrl"} {
		if v, ok := item[k].(string); ok && v != "" {
			url = v
			break
		}
	}
	if v, ok := item["format"].(string); ok {
		format = v
	}
	return url, format
}

// fileExt 生成文件扩展名：优先用接口返回的 format，其次取 URL 路径后缀。
func fileExt(format, url string) string {
	if format != "" {
		return "." + strings.TrimPrefix(format, ".")
	}
	path := url
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if i := strings.LastIndexByte(path, '.'); i >= 0 && len(path)-i <= 6 {
		return path[i:]
	}
	return ""
}

func writeJSON(w io.Writer, raw []byte, pretty bool) {
	if pretty {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err == nil {
			raw = buf.Bytes()
		}
	}
	fmt.Fprintln(w, string(raw))
}

func buildLong(spec CmdSpec) string {
	var sb strings.Builder
	if spec.Long != "" {
		sb.WriteString(spec.Long)
		sb.WriteString("\n\n")
	}
	fmt.Fprintf(&sb, "API: POST %s", spec.Path)
	return sb.String()
}

// asError 类似 errors.As，避免在此包中引入 errors 包装判断的样板。
func asError(err error, target **client.BizError) bool {
	for err != nil {
		if e, ok := err.(*client.BizError); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
