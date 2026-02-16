package handlers

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入配置包，用于获取本地目录配置
	"encoding/base64"                   // 导入base64包，用于解码文件名
	"encoding/json"                     // 导入json包，用于解析请求体
	"fmt"                               // 导入fmt包，用于格式化输出
	"io"                                // 导入io包，用于文件操作
	"log"                               // 导入log包，用于记录日志信息
	"net/http"                          // 导入net/http包，用于处理HTTP请求
	"os"                                // 导入os包，用于文件操作
	"path/filepath"                     // 导入path/filepath包，用于文件路径操作
	"strconv"                           // 导入strconv包，用于字符串和其他类型的转换
	"strings"                           // 导入strings包，用于字符串操作
	"time"                              // 导入time包，用于时间操作
)

// SaveHandler 处理文件保存的HTTP请求
func SaveHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为POST，若不是则返回错误
	if r.Method != "POST" {
		http.Error(w, "Save请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	// 从请求头中获取编码后的文件名
	encodedName := r.Header.Get("X-File-Name")
	// 检查文件名是否为空，若为空则返回错误
	if encodedName == "" {
		http.Error(w, "Save请求[ERROR] -> 缺少文件名", http.StatusBadRequest)
		return
	}
	// 对编码后的文件名进行解码
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedName)
	// 检查解码是否出错，若出错则返回错误
	if err != nil {
		http.Error(w, "Save请求[ERROR] -> 文件名解码错误", http.StatusBadRequest)
		return
	}
	// 将解码后的字节转换为字符串
	fileName := string(decodedBytes)
	// 再次检查文件名是否为空，若为空则返回错误
	if fileName == "" {
		http.Error(w, "Save请求[ERROR] -> 文件名解码后为空", http.StatusBadRequest)
		return
	}
	// 检查文件名是否为 "." 或 ".."，防止目录遍历攻击，若为则返回错误
	if fileName == "." || fileName == ".." {
		http.Error(w, "Save请求[ERROR] -> 无效的文件名", http.StatusForbidden)
		return
	}
	// 初始化覆盖标志，默认为不覆盖
	overwrite := false
	// 从请求头中获取覆盖标志
	if ow := r.Header.Get("X-Overwrite"); ow != "" {
		// 尝试将覆盖标志转换为布尔值
		if val, parseErr := strconv.ParseBool(ow); parseErr == nil {
			overwrite = val
		}
	}
	// 拼接文件的完整路径
	fullPath := filepath.Join(config.LocalDir, fileName)
	// 创建文件所在的目录，若创建失败则返回错误
	if mkdirErr := os.MkdirAll(filepath.Dir(fullPath), 0755); mkdirErr != nil {
		http.Error(w, fmt.Sprintf("Save请求[ERROR] -> 创建失败: %s", mkdirErr), http.StatusInternalServerError)
		return
	}
	// 获取文件锁并加锁，确保文件操作的原子性
	fileLock := getFileLock(fullPath)
	fileLock.Lock()
	defer fileLock.Unlock()
	// 检查文件是否存在
	_, err = os.Stat(fullPath)
	// 如果获取文件状态时发生了其他错误
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("Save请求[ERROR] -> 获取失败: %s", err), http.StatusInternalServerError)
		return
	} else {
		// 如果不允许覆盖且文件存在
		if !overwrite && !os.IsNotExist(err) {
			// 为文件名添加时间戳，创建新版本
			timestamp := time.Now().Format("20060102-150405")
			// 提取文件扩展名
			ext := filepath.Ext(fileName)
			// 提取文件名（不包含扩展名）
			name := strings.TrimSuffix(filepath.Base(fileName), ext)
			// 构建新的文件名，包含时间戳
			fileName = filepath.Join(filepath.Dir(fileName), fmt.Sprintf("%s_%s%s", name, timestamp, ext))
			// 更新文件的完整路径
			fullPath = filepath.Join(config.LocalDir, fileName)
		}
	}
	// 创建文件，若创建失败则返回错误
	file, err := os.Create(fullPath)
	// 检查文件创建是否出错
	if err != nil {
		http.Error(w, fmt.Sprintf("Save请求[ERROR] -> 创建失败: %s", err), http.StatusInternalServerError)
		return
	}
	// 关闭文件，确保资源释放
	defer file.Close()
	// 将请求体中的内容复制到文件中，若复制失败则返回错误
	if _, err := io.Copy(file, r.Body); err != nil {
		http.Error(w, fmt.Sprintf("Save请求[ERROR] -> 保存失败: %s", err), http.StatusInternalServerError)
		return
	}
	// 同步文件内容到磁盘，若同步失败则记录日志
	if err := file.Sync(); err != nil {
		log.Printf("Save请求[ERROR] -> 同步失败: %s, %v", fullPath, err)
	}
	// 设置响应状态码为200
	w.WriteHeader(http.StatusOK)
	// 构建响应数据
	response := map[string]string{
		"filename":  fileName,                      // 上传的文件名
		"path":      fullPath,                      // 文件的完整路径
		"overwrite": strconv.FormatBool(overwrite), // 是否覆盖 existing 文件
	}
	// 将响应数据编码为JSON并发送给客户端
	json.NewEncoder(w).Encode(response)
	// 记录保存成功日志
	if *config.DevMode {
		log.Printf("%s", strings.Repeat("-=", 28))
		log.Printf("Save请求 -> 成功保存文件: %s, 覆盖: %t", fullPath, overwrite)
		log.Printf("%s", strings.Repeat("-=", 28))
	}
}
