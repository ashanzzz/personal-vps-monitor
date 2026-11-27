package api

import "github.com/ashanzzz/personal-vps-monitor/pkg/providers"

// redactInstanceSecrets 生成实例列表的脱敏副本。
//
// 安全边界：
//   - HTTP 响应层永不下发明文 SSH / Root 密码，任何角色（含 admin）一律脱敏；
//   - 是否曾持久化密码通过 HasPassword 布尔位告知前端，供详情抽屉按需单独取用；
//   - 磁盘缓存与内部 SSH 拨号链路使用原始数据，不经过本函数。
func redactInstanceSecrets(in []providers.Instance) []providers.Instance {
	out := make([]providers.Instance, len(in))
	copy(out, in)
	for i := range out {
		out[i].HasPassword = out[i].SSHPassword != "" || out[i].RootPassword != ""
		out[i].SSHPassword = ""
		out[i].RootPassword = ""
	}
	return out
}
