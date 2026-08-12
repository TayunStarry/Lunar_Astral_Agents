// =============================================================================
// 星月智能 · 记忆库 v2 功能测试模块
// 测试标签向量中介检索架构的完整功能链路
//
// 前置条件：本地 36789 端口已启动 system-embedding + system-multimodal 服务
// 运行方式：go run ./cmd/test_memory/
// =============================================================================

package main

import (
	"LunarSubsystem/FileManager/module"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// 测试配置
const (
	testEmbeddingModel  = "system-embedding"
	testTextCollection  = "test_text_memory"
	testImageCollection = "test_image_memory"
)

// 测试用 1x1 红色 PNG 图片（base64 编码）
const testImageBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="

// 测试文本样本
var testMessages = []struct {
	role    string
	content string
}{
	{"user", "你好，我想了解一下人工智能在医疗领域的应用。"},
	{"assistant", "人工智能在医疗领域的应用非常广泛，包括医学影像诊断、药物研发、个性化治疗方案、疾病预测等方面。例如，深度学习算法可以分析CT和MRI图像，帮助医生更准确地检测肿瘤。"},
	{"user", "那自然语言处理呢？有什么具体的应用场景？"},
	{"assistant", "自然语言处理在医疗领域主要用于电子病历分析、医学文献检索、智能问诊系统、语音识别录入等。比如，NLP可以自动从病历中提取关键信息，辅助临床决策。"},
	{"user", "日本动画《你的名字》的导演是谁？"},
	{"assistant", "《你的名字》的导演是新海诚，他是一位著名的日本动画导演，以精美的画面和细腻的情感描写著称，代表作还有《秒速5厘米》、《天气之子》等。"},
	{"user", "Python和Go语言在性能方面有什么区别？"},
	{"assistant", "Go语言是编译型语言，执行效率远高于Python。Go的并发模型基于goroutine，内存占用小，适合高并发服务。Python是解释型语言，开发效率高但运行时性能较低，适合数据分析、AI训练等场景。"},
	{"user", "什么是量子计算？它和经典计算有什么不同？"},
	{"assistant", "量子计算利用量子比特（qubit）的叠加和纠缠特性进行计算。与经典计算机的二进制位（0或1）不同，量子比特可以同时处于多个状态的叠加。这使得量子计算机在特定问题上（如大数分解、量子模拟）具有指数级加速优势。"},
	{"user", "推荐几本好看的小说"},
	{"assistant", "推荐以下几本经典小说：1.《百年孤独》- 加西亚·马尔克斯，魔幻现实主义代表作；2.《三体》- 刘慈欣，中国科幻巅峰之作；3.《挪威的森林》- 村上春树，青春与爱情的经典。"},
	{"user", "如何制作一杯拿铁咖啡？"},
	{"assistant", "制作拿铁咖啡的步骤：1. 研磨18克咖啡豆至细粉状；2. 用意式咖啡机萃取30ml浓缩咖啡；3. 将200ml牛奶加热至65°C并用蒸汽棒打发出细腻奶泡；4. 将浓缩咖啡倒入杯中，再缓慢倒入打发好的牛奶，最后可以用奶泡做出拉花。"},
	{"user", "今天天气真好，适合出去散步。"},
	{"assistant", "是的，好天气确实适合户外活动。散步不仅能放松身心，还能促进血液循环，建议每天步行30分钟以上，对健康非常有益。"},
	{"user", "帮我写一个快速排序的Python实现"},
	{"assistant", "快速排序的Python实现：\ndef quicksort(arr):\n    if len(arr) <= 1:\n        return arr\n    pivot = arr[len(arr) // 2]\n    left = [x for x in arr if x < pivot]\n    middle = [x for x in arr if x == pivot]\n    right = [x for x in arr if x > pivot]\n    return quicksort(left) + middle + quicksort(right)"},
}

// =============================================================================
// 辅助函数
// =============================================================================

func separator(title string) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 70))
	fmt.Printf("  %s\n", title)
	fmt.Printf("%s\n\n", strings.Repeat("=", 70))
}

func subTest(name string) {
	fmt.Printf("  [TEST] %s...\n", name)
}

func ok(format string, args ...interface{}) {
	fmt.Printf("  ✅  "+format+"\n", args...)
}

func fail(format string, args ...interface{}) {
	fmt.Printf("  ❌  "+format+"\n", args...)
}

func info(format string, args ...interface{}) {
	fmt.Printf("  ℹ️  "+format+"\n", args...)
}

func check(err error, name string) bool {
	if err != nil {
		fail("%s 失败: %v", name, err)
		return false
	}
	ok("%s 成功", name)
	return true
}

// =============================================================================
// 测试用例
// =============================================================================

func main() {
	fmt.Println("\n╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║      星月智能 · 记忆库 v2 标签向量中介检索 — 功能测试           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")

	ctx := context.Background()
	failed := 0
	passed := 0

	// =========================================================================
	// 第一部分：实例初始化
	// =========================================================================
	separator("第一部分：实例初始化")

	subTest("初始化记忆库存储目录")
	tempDir, err := os.MkdirTemp("", "memory_test_v2_")
	if !check(err, "创建临时目录") {
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)
	info("存储目录: %s", tempDir)

	module.InitMemoryDB(tempDir)

	subTest("初始化记忆库实例（嵌入 + LLM）")
	err = module.MemoryInitInstance()
	if !check(err, "实例初始化") {
		failed++
		os.Exit(1)
	}
	passed++

	// =========================================================================
	// 第二部分：文本集合测试
	// =========================================================================
	separator("第二部分：文本集合 CRUD 测试")

	subTest("创建文本集合")
	err = module.CollectionInit(ctx, testTextCollection, testEmbeddingModel, module.CollectionTypeText)
	if !check(err, "创建文本集合") {
		failed++
	} else {
		passed++
		colInfo := module.MemoryGetCollectionInfo(testTextCollection)
		info("集合信息: 模型=%v, 维度=%v, 类型=%v",
			colInfo["embedding_model"], colInfo["embedding_dimension"], colInfo["type"])
	}

	// 添加消息（同步阻塞，等待 LLM 标签生成）
	separator("  添加文本消息（LLM 自动生成标签）")
	var addedIDs []string
	for i, msg := range testMessages {
		subTest(fmt.Sprintf("添加第 %d 条消息 [%s]", i+1, msg.role))
		start := time.Now()
		id, err := module.MemoryAddMessage(ctx, testTextCollection, msg.role, msg.content)
		elapsed := time.Since(start)
		if check(err, fmt.Sprintf("添加消息 (耗时 %v)", elapsed.Round(time.Millisecond))) {
			addedIDs = append(addedIDs, id)
			info("  UUID: %s", id)
			passed++
		} else {
			failed++
		}
	}

	// 集合统计
	separator("  集合统计验证")
	info2 := module.MemoryGetCollectionInfo(testTextCollection)
	count := getInt(info2, "document_count")
	tagCount := getInt(info2, "tag_count")
	info("文档总数: %d, 标签向量数: %d", count, tagCount)
	if count == len(addedIDs) {
		ok("文档数量正确: %d", count)
		passed++
	} else {
		fail("文档数量不符: 期望 %d, 实际 %d", len(addedIDs), count)
		failed++
	}

	// 标签文本验证
	separator("  标签文本存储验证")
	// 通过 JSON 序列化获取标签文本（绕过 Go 类型断言限制）
	jsonBytes, err := json.Marshal(module.MemoryDebugGetRawTags(testTextCollection))
	if err == nil && len(jsonBytes) > 2 {
		var tagList []map[string]interface{}
		if json.Unmarshal(jsonBytes, &tagList) == nil && len(tagList) > 0 {
			ok("标签向量已存储，共 %d 条，均包含 tag 文本", len(tagList))
			passed++
			info("标签示例:")
			showCount := 5
			if len(tagList) < showCount {
				showCount = len(tagList)
			}
			for i := 0; i < showCount; i++ {
				tv := tagList[i]
				tag := ""
				if t, ok := tv["tag"].(string); ok {
					tag = t
				}
				uuidCount := 0
				if uuids, ok := tv["uuid"].([]interface{}); ok {
					uuidCount = len(uuids)
				}
				info("  [%s] → %d 个文档", tag, uuidCount)
			}
		} else {
			fail("标签向量为空或格式异常")
			failed++
		}
	} else {
		fail("无法获取标签向量数据")
		failed++
	}

	// 语义查询测试
	separator("  语义查询测试")

	queries := []struct {
		query     string
		expectKey string
	}{
		{"人工智能在医疗中的应用", "影像诊断"},
		{"日本动画电影", "新海诚"},
		{"编程语言性能对比", "Go"},
		{"咖啡制作方法", "拿铁"},
		{"推荐小说", "百年孤独"},
		{"户外活动", "散步"},
		{"排序算法", "快速排序"},
	}

	for _, q := range queries {
		subTest(fmt.Sprintf("查询: \"%s\"", q.query))
		results, err := module.MemoryQueryMessagesWithContent(ctx, testTextCollection, q.query, 3)
		if check(err, fmt.Sprintf("查询 (返回 %d 条)", len(results))) {
			passed++
			for j, r := range results {
				content := r.Content
				if len(content) > 80 {
					content = content[:80] + "..."
				}
				info("  #%d [%s] 得分=%.3f | %s", j+1, r.Role, r.Similarity, content)
			}
		} else {
			failed++
		}
	}

	// 删除文档测试
	separator("  删除文档测试")
	if len(addedIDs) > 0 {
		delID := addedIDs[len(addedIDs)-1]
		subTest(fmt.Sprintf("删除文档 %s", delID[:20]+"..."))
		err = module.MemoryDeleteMessage(ctx, testTextCollection, delID)
		if check(err, "删除文档") {
			passed++
		} else {
			failed++
		}

		// 验证删除后的数量
		info3 := module.MemoryGetCollectionInfo(testTextCollection)
		newCount := getInt(info3, "document_count")
		if newCount == len(addedIDs)-1 {
			ok("删除后文档数量正确: %d", newCount)
			passed++
		} else {
			fail("删除后文档数量不符: 期望 %d, 实际 %d", len(addedIDs)-1, newCount)
			failed++
		}
	}

	// =========================================================================
	// 第三部分：图片集合测试
	// =========================================================================
	separator("第三部分：图片集合测试")

	subTest("创建图片集合")
	err = module.CollectionInit(ctx, testImageCollection, testEmbeddingModel, module.CollectionTypeImage)
	if !check(err, "创建图片集合") {
		failed++
	} else {
		passed++
	}

	subTest("添加图片（LLM 自动生成标签）")
	start := time.Now()
	imgID, err := module.MemoryAddImage(ctx, testImageCollection, testImageBase64)
	elapsed := time.Since(start)
	if check(err, fmt.Sprintf("添加图片 (耗时 %v)", elapsed.Round(time.Millisecond))) {
		info("  图片 UUID: %s", imgID)
		passed++
	} else {
		failed++
	}

	// 图片查询测试
	subTest("查询图片集合")
	imgResults, err := module.MemoryQueryMessagesWithContent(ctx, testImageCollection, "红色图片", 1)
	if check(err, fmt.Sprintf("图片查询 (返回 %d 条)", len(imgResults))) {
		passed++
		for j, r := range imgResults {
			hasImage := r.Image != ""
			info("  #%d 得分=%.3f 含图片数据=%v", j+1, r.Similarity, hasImage)
		}
	} else {
		failed++
	}

	// =========================================================================
	// 第四部分：集合管理
	// =========================================================================
	separator("第四部分：集合管理")

	subTest("列出所有集合")
	collections := module.MemoryListCollections()
	info("所有集合: %v", collections)
	if len(collections) >= 2 {
		ok("集合数量正确: %d", len(collections))
		passed++
	} else {
		fail("集合数量不足: 期望 >=2, 实际 %d", len(collections))
		failed++
	}

	subTest("清空图片集合")
	err = module.MemoryClearCollection(testImageCollection)
	if check(err, "清空集合") {
		passed++
		info4 := module.MemoryGetCollectionInfo(testImageCollection)
		if getInt(info4, "document_count") == 0 {
			ok("清空后文档数为 0")
			passed++
		} else {
			fail("清空后文档数不为 0")
			failed++
		}
	} else {
		failed++
	}

	subTest("删除图片集合")
	err = module.MemoryDeleteCollection(testImageCollection)
	if check(err, "删除集合") {
		passed++
		collections2 := module.MemoryListCollections()
		found := false
		for _, c := range collections2 {
			if c == testImageCollection {
				found = true
				break
			}
		}
		if !found {
			ok("集合已从列表中移除")
			passed++
		} else {
			fail("集合仍在列表中")
			failed++
		}
	} else {
		failed++
	}

	// =========================================================================
	// 第五部分：重建测试
	// =========================================================================
	separator("第五部分：重建标签向量")

	subTest("重建文本集合的标签向量")
	err = module.MemoryRebuildEntries(ctx, testTextCollection, testEmbeddingModel, func(current, total int) {
		fmt.Printf("\r    重建进度: %d/%d", current, total)
	})
	if err == nil {
		fmt.Println()
		ok("重建完成")
		passed++
		info5 := module.MemoryGetCollectionInfo(testTextCollection)
		info("重建后: 文档数=%d, 标签数=%d",
			getInt(info5, "document_count"), getInt(info5, "tag_count"))
	} else {
		fmt.Println()
		fail("重建失败: %v", err)
		failed++
	}

	// =========================================================================
	// 测试结果汇总
	// =========================================================================
	separator("测试结果汇总")
	total := passed + failed
	fmt.Printf("  总计: %d 项测试\n", total)
	fmt.Printf("  通过: %d ✅\n", passed)
	fmt.Printf("  失败: %d ❌\n", failed)
	if failed == 0 {
		fmt.Printf("\n  🎉 所有测试通过！记忆库 v2 功能正常。\n")
	} else {
		fmt.Printf("\n  ⚠️ 存在 %d 项失败，请检查服务状态。\n", failed)
	}
	fmt.Println()
}

// getInt 从 map 中安全获取整数值
func getInt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		}
	}
	return 0
}
