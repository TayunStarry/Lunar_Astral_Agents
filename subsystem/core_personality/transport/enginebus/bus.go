package enginebus

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"LunarSubsystem/LoggerGeneral"
)

// NewBus 创建引擎消息总线实例
func NewBus() *Bus {
	return &Bus{
		methods: make(map[string]reflect.Value),
		owners:  make(map[string]string),
	}
}

// SetSender 注入透传发送函数
func (b *Bus) SetSender(s Sender) {
	b.mu.Lock()
	b.sender = s
	b.mu.Unlock()
}

// Subscribe 反射扫描观察者的 On<Type> 方法并建立 type → 方法索引。
// 返回成功索引的方法数量。方法签名要求为 func(raw []byte)，返回值被忽略。
func (b *Bus) Subscribe(observer Observer) int {
	t := reflect.TypeOf(observer)
	v := reflect.ValueOf(observer)
	if t == nil {
		return 0
	}

	count := 0
	rawType := reflect.TypeOf([]byte(nil))
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if !strings.HasPrefix(m.Name, MethodPrefix) || m.Name == MethodPrefix {
			continue
		}
		// 只接受 func(receiver, []byte)：绑定方法含接收者，故形参数为 2
		if m.Type.NumIn() != 2 || m.Type.NumOut() != 0 || m.Type.In(1) != rawType {
			continue
		}
		key := SnakeCase(strings.TrimPrefix(m.Name, MethodPrefix))
		if key == "" {
			continue
		}
		b.mu.Lock()
		if prev, ok := b.owners[key]; ok {
			LoggerGeneral.Warn("EngineBus", "消息类型 [%s] 的观察者由 [%s] 覆盖为 [%s]", key, prev, observer.Name())
		}
		b.methods[key] = v.Method(i)
		b.owners[key] = observer.Name()
		b.mu.Unlock()
		count++
	}
	if count > 0 {
		LoggerGeneral.Info("EngineBus", "观察者 [%s] 已索引 %d 类引擎消息", observer.Name(), count)
	}
	return count
}

// Publish 处理一条引擎消息：反射分派给观察者（只读观测），随后由调用方/本函数完成透传。
// 返回被成功观测的消息 type 列表（未识别或观测异常时不阻塞透传）。
func (b *Bus) Publish(raw []byte) []string {
	observed := []string{}

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		LoggerGeneral.Warn("EngineBus", "引擎消息解析失败（已跳过观测，仍透传）: %v", err)
		return observed
	}
	if env.Type == "" {
		LoggerGeneral.Warn("EngineBus", "引擎消息缺少 type 字段（已跳过观测，仍透传）")
		return observed
	}

	b.mu.RLock()
	method, ok := b.methods[env.Type]
	owner := b.owners[env.Type]
	b.mu.RUnlock()
	if !ok {
		return observed
	}

	if err := safeCall(method, raw); err != nil {
		LoggerGeneral.Error("EngineBus", "观察者 [%s] 处理 [%s] 失败: %v", owner, env.Type, err)
		return observed
	}
	return append(observed, env.Type)
}

// Emit 由服务端主动构造并广播一条引擎消息（服务端保留的发送能力）
func (b *Bus) Emit(msgType string, payload any) error {
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化引擎消息负载失败: %w", err)
	}
	data, err := json.Marshal(Envelope{Type: msgType, Payload: payloadRaw})
	if err != nil {
		return fmt.Errorf("序列化引擎消息信封失败: %w", err)
	}

	b.mu.RLock()
	sender := b.sender
	b.mu.RUnlock()
	if sender == nil {
		return fmt.Errorf("引擎总线未注入发送函数")
	}
	sender(data)
	return nil
}

// safeCall 反射调用观察者方法并捕获 panic，保证观测异常不影响透传
func safeCall(method reflect.Value, raw []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	method.Call([]reflect.Value{reflect.ValueOf(raw)})
	return nil
}

// SnakeCase 把驼峰/帕斯卡命名转换为下划线命名（AnimationList → animation_list）
func SnakeCase(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				// 大写字母前插入下划线，但连续大写（如 LTP9）不拆分
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					sb.WriteRune('_')
				} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					sb.WriteRune('_')
				}
			}
			sb.WriteRune(unicode.ToLower(r))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
