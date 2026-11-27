package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

func TestRedactInstanceSecretsClearsPasswordsAndSetsFlag(t *testing.T) {
	in := []providers.Instance{
		{ProviderID: "lazycat", ID: "1", SSHPassword: "s3cret-a", RootPassword: "s3cret-a"},
		{ProviderID: "akile", ID: "2", SSHPassword: "", RootPassword: "s3cret-b"},
		{ProviderID: "acck", ID: "3"},
	}

	out := redactInstanceSecrets(in)

	if in[0].SSHPassword != "s3cret-a" {
		t.Fatalf("redactInstanceSecrets 必须返回副本，不得修改入参")
	}
	for _, inst := range out {
		if inst.SSHPassword != "" || inst.RootPassword != "" {
			t.Fatalf("实例 %s 仍残留明文密码", inst.ID)
		}
	}
	if !out[0].HasPassword || !out[1].HasPassword || out[2].HasPassword {
		t.Fatalf("HasPassword 标记错误: %+v", []bool{out[0].HasPassword, out[1].HasPassword, out[2].HasPassword})
	}

	// 序列化后也不得出现任何密码字段值
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"s3cret-a", "s3cret-b", "ssh_password", "root_password"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("序列化结果仍包含敏感内容 %q: %s", secret, raw)
		}
	}
}

func TestRedactInstanceSecretsHandlesEmpty(t *testing.T) {
	out := redactInstanceSecrets(nil)
	if len(out) != 0 {
		t.Fatalf("nil 输入应返回空切片")
	}
	if out == nil {
		t.Fatalf("应返回可安全序列化为 [] 的非 nil 切片")
	}
}
