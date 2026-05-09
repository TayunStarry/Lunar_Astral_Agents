# LunarTick 示例脚本

这是一个简单的 LunarTick 示例脚本，展示了基本功能。

```LunarTick
@log "=== LunarTick 示例程序 ==="

// 设置变量
SET name "LunarTick"
SET version "1.0.0"

@log "欢迎使用 #name v#version!"

// 定义一个惰性指针
@lazy *greet
@log "你好! 这是来自指针的问候!"

// 调用指针
*greet

// 演示循环
SET count "0"
@lazy *loop
@math count #count + 1
@log "循环计数: #count"
@if "#count < 5" ? *loop : *done

@lazy *done
@log "循环完成!"

*loop

// 等待一下再结束
@sleep 2000

@log "=== 程序结束 ==="
```
