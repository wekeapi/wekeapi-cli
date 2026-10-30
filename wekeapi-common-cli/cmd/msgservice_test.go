package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestPromptMsgSyncDatabase 首次创建服务配置时的数据库类型询问：默认 SQLite、选 2 走 MySQL。
func TestPromptMsgSyncDatabase(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantDB  string
		wantDSN string
		wantErr bool
	}{
		{name: "回车默认SQLite", input: "\n", wantDB: "sqlite"},
		{name: "选1", input: "1\n", wantDB: "sqlite"},
		{name: "选2带连接串", input: "2\nmysql://u:p@127.0.0.1:3306/w\n", wantDB: "mysql", wantDSN: "mysql://u:p@127.0.0.1:3306/w"},
		{name: "选2未给连接串", input: "2\n\n", wantErr: true},
		{name: "非法选项", input: "3\n", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, err := promptMsgSyncDatabase(&stderr, bufio.NewReader(strings.NewReader(c.input)))
			if c.wantErr {
				if err == nil {
					t.Fatalf("应返回错误，实际得到 %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if got == nil || got.DB != c.wantDB {
				t.Fatalf("数据库类型不符: %+v", got)
			}
			if got.DSN != c.wantDSN {
				t.Fatalf("连接串不符: %q", got.DSN)
			}
			if !strings.Contains(stderr.String(), "SQLite") {
				t.Fatalf("提示信息应写入 stderr: %q", stderr.String())
			}
		})
	}
}
