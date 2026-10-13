package enginebus

// ==== 反射索引约定 ====

// MethodPrefix 观察者方法前缀：On + 驼峰类型名
const MethodPrefix = "On"

// ==== 全局总线实例 ====

// Instance 全局引擎总线（装配期赋值）
var Instance *Bus
