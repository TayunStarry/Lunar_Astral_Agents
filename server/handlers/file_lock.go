package handlers

// 导入 sync 包，用于处理并发同步相关操作
import "sync"

// FileLocks 是一个 sync.Map 类型的变量，用于存储文件路径对应的互斥锁。
// 每个文件路径对应一个 *sync.Mutex，用于保证对该文件的并发操作是安全的。
var FileLocks sync.Map

// getFileLock 根据给定的文件路径获取对应的互斥锁。
// 如果该文件路径对应的锁不存在，则会创建一个新的互斥锁并存储到 FileLocks 中。
// 参数 filePath 是要获取锁的文件路径。
// 返回值是该文件路径对应的互斥锁指针。
func getFileLock(filePath string) *sync.Mutex {
	// 使用 LoadOrStore 方法获取文件路径对应的锁。
	// 如果锁不存在，则会创建一个新的互斥锁并存储到 FileLocks 中。
	// 返回值是该文件路径对应的锁指针和一个布尔值，布尔值表示是否是新创建的锁。
	lock, _ := FileLocks.LoadOrStore(filePath, &sync.Mutex{})
	// 断言锁指针的类型为 *sync.Mutex，确保返回的锁指针类型正确。
	// 若断言失败，会触发 panic 异常，提示类型断言失败。
	// 若断言成功，返回锁指针。
	return lock.(*sync.Mutex)
}
