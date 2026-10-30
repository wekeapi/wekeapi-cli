package syncer

import (
	"reflect"
	"testing"
)

func TestDiff(t *testing.T) {
	remote := []string{"a", "b", "c"}
	local := []string{"b", "c", "d"}

	add, del := diff(remote, local, true)
	if !reflect.DeepEqual(add, []string{"a"}) {
		t.Errorf("add = %v, want [a]", add)
	}
	if !reflect.DeepEqual(del, []string{"d"}) {
		t.Errorf("del = %v, want [d]", del)
	}
}

func TestDiffChatroomNeverDeletes(t *testing.T) {
	// 群聊接口只返回已保存到通讯录的群，本地多出的群不能删。
	add, del := diff([]string{"room_new"}, []string{"room_old"}, false)
	if !reflect.DeepEqual(add, []string{"room_new"}) {
		t.Errorf("add = %v, want [room_new]", add)
	}
	if len(del) != 0 {
		t.Errorf("del = %v, want empty", del)
	}
}

func TestDiffNoChanges(t *testing.T) {
	add, del := diff([]string{"a", "b"}, []string{"b", "a"}, true)
	if len(add) != 0 || len(del) != 0 {
		t.Errorf("add=%v del=%v, want empty", add, del)
	}
}

func TestDiffEmpty(t *testing.T) {
	add, del := diff([]string{}, []string{}, true)
	if len(add) != 0 || len(del) != 0 {
		t.Errorf("add=%v del=%v, want empty", add, del)
	}
}

func TestFilterSystem(t *testing.T) {
	ids := []string{"wxid_a", "filehelper", "fmessage", "wxid_b", "weixin"}
	got := filterSystem(ids)
	if !reflect.DeepEqual(got, []string{"wxid_a", "wxid_b"}) {
		t.Errorf("filterSystem = %v, want [wxid_a wxid_b]", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate 短串 = %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc..." {
		t.Errorf("truncate 长串 = %q, want abc...", got)
	}
}
