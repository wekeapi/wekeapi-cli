package runner

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newOpts(stdout, stderr *bytes.Buffer, baseURL string) *Options {
	return &Options{
		Token:   "tok",
		BaseURL: baseURL,
		Timeout: 5 * time.Second,
		Stdout:  stdout,
		Stderr:  stderr,
	}
}

// captureServer 返回一个 httptest server 和接收到的请求 body。
func captureServer(t *testing.T, respBody string) (*httptest.Server, *map[string]any) {
	t.Helper()
	got := &map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, got)
		w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func exec(t *testing.T, spec CmdSpec, opts *Options, args ...string) error {
	t.Helper()
	cmd := Build(spec, opts)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestRunSuccessBodyBuilding(t *testing.T) {
	srv, got := captureServer(t, `{"code":0,"message":"OK","data":{"newMsgId":1}}`)

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	opts.AppID = "app1"
	opts.Data = `{"extra":"from-data","content":"from-data"}`

	spec := CmdSpec{
		Use:        "send-text",
		Path:       "/v1/message/sendText",
		NeedsAppID: true,
		Flags: []FlagSpec{
			{Name: "to-wxid", BodyKey: "toWxid", Required: true},
			{Name: "content", BodyKey: "content", Required: true},
		},
	}
	err := exec(t, spec, opts, "--to-wxid", "wxid_a", "--content", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}

	// appId 注入、--data 合并、具名 flag 覆盖 --data
	if (*got)["appId"] != "app1" {
		t.Errorf("appId = %v", (*got)["appId"])
	}
	if (*got)["extra"] != "from-data" {
		t.Errorf("extra = %v", (*got)["extra"])
	}
	if (*got)["content"] != "hello" {
		t.Errorf("content = %v, want flag 覆盖 --data", (*got)["content"])
	}
	if !bytes.Contains(out.Bytes(), []byte(`"code":0`)) {
		t.Errorf("stdout = %s", out.String())
	}
}

func TestRunFlagDefaultFill(t *testing.T) {
	srv, got := captureServer(t, `{"code":0,"message":"OK","data":null}`)

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	opts.AppID = "app1"
	opts.Data = `{"autoConnect":true}` // --data 显式提供的字段优先于默认值

	spec := CmdSpec{
		Use:        "status",
		Path:       "/v1/login/checkOnlineStatus",
		NeedsAppID: true,
		Flags: []FlagSpec{
			{Name: "auto-connect", BodyKey: "autoConnect", Kind: KindBool, Default: false},
		},
	}

	// 未传 flag：默认值 false 补入 body（API 必填字段）；--data 显式提供的字段优先于默认值
	if err := exec(t, spec, opts); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}
	if v, ok := (*got)["autoConnect"].(bool); !ok || !v {
		t.Errorf("autoConnect = %v, want --data 提供的 true 优先", (*got)["autoConnect"])
	}

	// 不带 --data：写入默认值 false
	opts2 := newOpts(&out, &errOut, srv.URL)
	opts2.AppID = "app1"
	if err := exec(t, spec, opts2); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}
	if v, ok := (*got)["autoConnect"].(bool); !ok || v {
		t.Errorf("autoConnect = %v, want 默认 false", (*got)["autoConnect"])
	}

	// 显式传 --auto-connect：flag 值优先生效
	opts3 := newOpts(&out, &errOut, srv.URL)
	opts3.AppID = "app1"
	if err := exec(t, spec, opts3, "--auto-connect"); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}
	if v, ok := (*got)["autoConnect"].(bool); !ok || !v {
		t.Errorf("autoConnect = %v, want 显式 flag true", (*got)["autoConnect"])
	}
}

func TestRunBizErrorExitCode1(t *testing.T) {
	srv, _ := captureServer(t, `{"code":-1,"message":"fail","data":null}`)

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{Use: "x", Path: "/v1/x"}

	err := exec(t, spec, opts)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.Code)
	}
	// 业务错误时 stdout 仍输出原始 JSON
	if !bytes.Contains(out.Bytes(), []byte(`"code":-1`)) {
		t.Errorf("stdout = %s", out.String())
	}
}

func TestRunMissingTokenExitCode2(t *testing.T) {
	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, "http://unused")
	opts.Token = ""
	spec := CmdSpec{Use: "x", Path: "/v1/x"}

	err := exec(t, spec, opts)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("want ExitError code 2, got %v", err)
	}
}

func TestRunMissingRequiredFlag(t *testing.T) {
	srv, _ := captureServer(t, `{"code":0}`)
	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:   "x",
		Path:  "/v1/x",
		Flags: []FlagSpec{{Name: "to-wxid", BodyKey: "toWxid", Required: true}},
	}
	err := exec(t, spec, opts)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("want ExitError code 2, got %v", err)
	}
}

func TestRunConflictingFlags(t *testing.T) {
	srv, _ := captureServer(t, `{"code":0}`)
	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:  "send-image",
		Path: "/v1/message/sendImage",
		Flags: []FlagSpec{
			{Name: "file", BodyKey: "imageData", Kind: KindFile, Required: true},
			{Name: "url", BodyKey: "imageData", Required: true},
		},
	}
	err := exec(t, spec, opts, "--file", "x.jpg", "--url", "http://a/b.jpg")
	if err == nil {
		t.Fatal("want conflict error")
	}
}

func TestRunFileFlagBase64(t *testing.T) {
	srv, got := captureServer(t, `{"code":0,"message":"OK","data":{}}`)
	tmp := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(tmp, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:   "send-file",
		Path:  "/v1/message/sendFile",
		Flags: []FlagSpec{{Name: "file", BodyKey: "fileData", Kind: KindFile, Required: true}},
	}
	if err := exec(t, spec, opts, "--file", tmp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("hi"))
	if (*got)["fileData"] != want {
		t.Errorf("fileData = %v, want %v", (*got)["fileData"], want)
	}
}

func TestRunXMLFlagBase64(t *testing.T) {
	srv, got := captureServer(t, `{"code":0,"message":"OK","data":{}}`)
	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:  "download-image",
		Path: "/v1/message/downloadImage",
		Flags: []FlagSpec{
			{Name: "xml", BodyKey: "xmlMsgBase64", Kind: KindXML, XMLBase64: true, Required: true},
		},
	}
	if err := exec(t, spec, opts, "--xml", "<msg><img/></msg>"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("<msg><img/></msg>"))
	if (*got)["xmlMsgBase64"] != want {
		t.Errorf("xmlMsgBase64 = %v, want %v", (*got)["xmlMsgBase64"], want)
	}
}

func TestRunOutDownload(t *testing.T) {
	// 文件服务器
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("img-bytes"))
	}))
	defer fileSrv.Close()

	srv, _ := captureServer(t, `{"code":0,"message":"OK","data":{"imageUrl":"`+fileSrv.URL+`/img"}}`)

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:       "download-image",
		Path:      "/v1/message/downloadImage",
		OutURLKey: "imageUrl",
	}
	outPath := filepath.Join(t.TempDir(), "img.jpg")
	if err := exec(t, spec, opts, "--out", outPath); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("out file not written: %v", err)
	}
	if string(b) != "img-bytes" {
		t.Errorf("out content = %q", b)
	}
}

func TestRunOutBase64DataURI(t *testing.T) {
	imgB64 := base64.StdEncoding.EncodeToString([]byte("qr-bytes"))
	srv, _ := captureServer(t, `{"code":0,"message":"OK","data":{"qrCodeImgData":"data:image/jpg;base64,`+imgB64+`"}}`)

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:          "qr",
		Path:         "/v1/login/getLoginQrCode",
		OutBase64Key: "qrCodeImgData",
	}
	outPath := filepath.Join(t.TempDir(), "qr.jpg")
	if err := exec(t, spec, opts, "--out", outPath); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("out file not written: %v", err)
	}
	if string(b) != "qr-bytes" {
		t.Errorf("out content = %q", b)
	}
}

// TestAppIDExplicitEmpty：显式空 appId（--app-id=）与 --new-device 都必须把空值写进 body，
// 供服务端按新设备处理（配置里的 appId 不再参与）。
func TestAppIDExplicitEmpty(t *testing.T) {
	t.Run("OptionalAppID with AppIDSet empty", func(t *testing.T) {
		srv, got := captureServer(t, `{"code":0,"message":"OK","data":{}}`)
		var out, errOut bytes.Buffer
		opts := newOpts(&out, &errOut, srv.URL)
		opts.AppID = "" // 显式空值
		opts.AppIDSet = true
		spec := CmdSpec{
			Use:           "qr",
			Path:          "/v1/login/getLoginQrCode",
			OptionalAppID: true,
			NewDeviceFlag: true,
		}
		if err := exec(t, spec, opts); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
		}
		v, ok := (*got)["appId"]
		if !ok || v != "" {
			t.Errorf("appId = %v (present=%v), want explicit empty string", v, ok)
		}
	})

	t.Run("OptionalAppID omits appId when not set and empty", func(t *testing.T) {
		srv, got := captureServer(t, `{"code":0,"message":"OK","data":{}}`)
		var out, errOut bytes.Buffer
		opts := newOpts(&out, &errOut, srv.URL)
		spec := CmdSpec{Use: "qr", Path: "/v1/login/getLoginQrCode", OptionalAppID: true}
		if err := exec(t, spec, opts); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
		}
		if _, ok := (*got)["appId"]; ok {
			t.Errorf("appId should be omitted, got %v", (*got)["appId"])
		}
	})

	t.Run("new-device forces empty appId over config value", func(t *testing.T) {
		srv, got := captureServer(t, `{"code":0,"message":"OK","data":{}}`)
		var out, errOut bytes.Buffer
		opts := newOpts(&out, &errOut, srv.URL)
		opts.AppID = "cfg-appid" // 配置中的 appId
		spec := CmdSpec{
			Use:           "qr",
			Path:          "/v1/login/getLoginQrCode",
			OptionalAppID: true,
			NewDeviceFlag: true,
		}
		if err := exec(t, spec, opts, "--new-device"); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
		}
		if v := (*got)["appId"]; v != "" {
			t.Errorf("appId = %v, want empty", v)
		}
	})

	t.Run("NeedsAppID explicit empty writes empty instead of erroring", func(t *testing.T) {
		srv, got := captureServer(t, `{"code":0,"message":"OK","data":{"data":true}}`)
		var out, errOut bytes.Buffer
		opts := newOpts(&out, &errOut, srv.URL)
		opts.AppIDSet = true
		spec := CmdSpec{
			Use:        "check-city",
			Path:       "/v1/proxy/checkCityProxy",
			NeedsAppID: true,
			Flags:      []FlagSpec{{Name: "city-code", BodyKey: "cityCode", Required: true}},
		}
		if err := exec(t, spec, opts, "--city-code", "3402"); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
		}
		v, ok := (*got)["appId"]
		if !ok || v != "" {
			t.Errorf("appId = %v (present=%v), want explicit empty string", v, ok)
		}
	})

	t.Run("NeedsAppID without app-id still errors locally", func(t *testing.T) {
		var out, errOut bytes.Buffer
		opts := newOpts(&out, &errOut, "http://unused")
		spec := CmdSpec{Use: "x", Path: "/v1/x", NeedsAppID: true}
		err := exec(t, spec, opts)
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 2 {
			t.Fatalf("want ExitError code 2, got %v", err)
		}
	})
}

// TestRunFileListFlagBase64：--images 传多个本地文件时，body 中应得到逐个 base64 后的字符串数组
// （朋友圈图文发布 sendImageSns 的 imageList 用法）。
func TestRunFileListFlagBase64(t *testing.T) {
	srv, got := captureServer(t, `{"code":0,"message":"OK","data":{"snsId":1}}`)

	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.jpg")
	p2 := filepath.Join(dir, "b.png")
	if err := os.WriteFile(p1, []byte("img-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("img-b"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:  "send-image",
		Path: "/v1/sns/sendImageSns",
		Flags: []FlagSpec{
			{Name: "images", BodyKey: "imageList", Kind: KindFileList, Required: true},
		},
	}
	if err := exec(t, spec, opts, "--images", p1+","+p2); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}

	list, ok := (*got)["imageList"].([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("imageList = %v, want 2 items", (*got)["imageList"])
	}
	for i, want := range []string{"img-a", "img-b"} {
		wantB64 := base64.StdEncoding.EncodeToString([]byte(want))
		if list[i] != wantB64 {
			t.Errorf("imageList[%d] = %v, want %s", i, list[i], wantB64)
		}
	}
}

// TestRunOutDirDownloads：响应给出多个下载地址（朋友圈图片列表）时，--out-dir 按序落盘为 1.jpg、2.png。
func TestRunOutDirDownloads(t *testing.T) {
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bytes" + r.URL.Path))
	}))
	defer fileSrv.Close()

	srv, _ := captureServer(t, `{"code":0,"message":"OK","data":{"imageList":[`+
		`{"imageUrl":"`+fileSrv.URL+`/a","format":"jpg"},`+
		`{"imageUrl":"`+fileSrv.URL+`/b","format":"png"}]}}`)

	var out, errOut bytes.Buffer
	opts := newOpts(&out, &errOut, srv.URL)
	spec := CmdSpec{
		Use:           "download-image",
		Path:          "/v1/sns/downloadSnsImage",
		OutURLListKey: "imageList",
	}
	dir := filepath.Join(t.TempDir(), "imgs")
	if err := exec(t, spec, opts, "--out-dir", dir); err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut.String())
	}

	for name, want := range map[string]string{"1.jpg": "bytes/a", "2.png": "bytes/b"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
		if string(b) != want {
			t.Errorf("%s content = %q, want %q", name, b, want)
		}
	}
}
