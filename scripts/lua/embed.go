// Package lua 内嵌 Redis Lua 脚本。
package lua

import _ "embed"

//go:embed seckill.lua
var SeckillScript string
