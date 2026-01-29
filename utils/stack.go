package utils

import (
	"runtime"
	"strings"
)

// GetStack 获取panic的栈信息，返回格式化的字符串
func GetStack() string {
	var buf [4096]byte
	n := runtime.Stack(buf[:], false)
	// 过滤掉GetStack自身的栈帧，只保留业务代码栈信息
	stack := string(buf[:n])
	lines := strings.Split(stack, "\n")
	if len(lines) > 2 {
		stack = strings.Join(lines[2:], "\n") // 去掉runtime.Stack和GetStack自身的行
	}
	return stack
}
