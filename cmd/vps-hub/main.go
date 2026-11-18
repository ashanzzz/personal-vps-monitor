package main

import (
	// 注册已启用的 VPS Provider (需要添加或移除某个厂商，直接在此增删空白导入即可)
	_ "github.com/ashanzzz/personal-vps-monitor/pkg/providers/acck"
	_ "github.com/ashanzzz/personal-vps-monitor/pkg/providers/akile"
	_ "github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
)

func main() {
	Execute()
}
