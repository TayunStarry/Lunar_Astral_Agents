# LunarTick - 编程语言解释器

> 🐚 **LunarTick** 是一款轻量级的自定义编程语言及其解释器，旨在提供简洁、高效的脚本执行能力。

---

## 🏗️ 架构设计

### 模块结构

```
subsystem/LunarTick/
├── main.go           # 主程序入口
├── lexer.go          # 词法分析器
├── interpreter.go    # 解释器核心
├── types.go          # 类型系统定义
├── variables.go       # 变量管理
├── pointers.go       # 指针支持
├── websocket.go      # WebSocket通信
├── client.html       # Web客户端
├── README.md         # 说明文档
└── go.mod
```

---

## 📝 语言特性

### 基本语法

```
// 变量声明
let name = "月华"
let age = 18
let pi = 3.14159

// 输出
print("Hello, " + name)

// 条件语句
if age >= 18 {
    print("成年人")
} else {
    print("未成年")
}

// 循环语句
for i = 0; i < 5; i = i + 1 {
    print(i)
}

// 函数定义
func greet(name) {
    return "Hello, " + name
}
```

### 支持的数据类型

| 类型 | 说明 | 示例 |
|------|------|------|
| `number` | 数值类型 | `let x = 42` |
| `string` | 字符串 | `let s = "hello"` |
| `boolean` | 布尔类型 | `let flag = true` |
| `array` | 数组 | `let arr = [1, 2, 3]` |
| `object` | 对象 | `let obj = {name: "test"}` |
| `pointer` | 指针 | `ptr @variable` |

### 运算符

#### 算术运算符
| 运算符 | 说明 |
|--------|------|
| `+` | 加法 |
| `-` | 减法 |
| `*` | 乘法 |
| `/` | 除法 |
| `%` | 取模 |

#### 比较运算符
| 运算符 | 说明 |
|--------|------|
| `==` | 等于 |
| `!=` | 不等于 |
| `<` | 小于 |
| `>` | 大于 |
| `<=` | 小于等于 |
| `>=` | 大于等于 |

#### 逻辑运算符
| 运算符 | 说明 |
|--------|------|
| `and` | 逻辑与 |
| `or` | 逻辑或 |
| `not` | 逻辑非 |

---

## 🔧 使用方式

### 命令行使用

```powershell
# 执行脚本文件
.\lunartick.exe script.lt

# 调试模式
.\lunartick.exe -debug script.lt

# 帮助信息
.\lunartick.exe -help
```

### WebSocket模式

启动WebSocket服务器进行远程脚本执行：

```powershell
.\lunartick.exe -ws :8080
```

### Go语言集成

```go
import "LunarTick"

// 创建解释器
interpreter := lunartick.NewInterpreter()

// 执行脚本
result, err := interpreter.Execute(`
    let message = "Hello from LunarTick"
    print(message)
    return message
`)

if err != nil {
    log.Fatal(err)
}

fmt.Println("Result:", result)
```

---

## 📡 WebSocket接口

### 连接

```
ws://localhost:8080/lunartick
```

### 发送消息

```json
{
  "type": "execute",
  "code": "print('Hello')"
}
```

### 接收响应

```json
{
  "type": "result",
  "success": true,
  "output": "Hello\n",
  "result": null
}
```

---

## 🔌 内置函数

| 函数 | 说明 | 示例 |
|------|------|------|
| `print()` | 打印输出 | `print("hello")` |
| `input()` | 获取输入 | `let name = input("Name: ")` |
| `len()` | 获取长度 | `len("hello")` |
| `type()` | 获取类型 | `type(x)` |
| `str()` | 转换为字符串 | `str(123)` |
| `num()` | 转换为数值 | `num("42")` |
| `read()` | 读取文件 | `read("file.txt")` |
| `write()` | 写入文件 | `write("file.txt", "content")` |
| `exit()` | 退出程序 | `exit(0)` |

---

## 📁 文件格式

LunarTick脚本文件使用 `.lt` 扩展名：

```lunar
// hello.lt
func main() {
    let names = ["月华", "琉璃", "蔷薇"]
    for name in names {
        print("Hello, " + name)
    }
}

main()
```

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [星图·月华 文档](../luna_astral.md)
- [星图·琉璃 文档](../crystal_astral.md)
- [LunarTick设计文档](../../subsystem/LunarTick/编程语言设计文档.md)