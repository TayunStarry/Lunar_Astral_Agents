package main

import "strings"

// repeatableFlag 支持重复出现并累积取值的命令行参数
// （用于 -wait-file 与 -restart-arg：一个开关可携带多个值）。
type repeatableFlag []string

// String 返回累积值的逗号拼接（flag.Value 接口要求）
func (r *repeatableFlag) String() string { return strings.Join(*r, ",") }

// Set 追加一个取值（flag.Value 接口要求）
func (r *repeatableFlag) Set(v string) error {
	*r = append(*r, v)
	return nil
}
