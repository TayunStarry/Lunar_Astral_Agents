package sound

// variable.go 提示音文件、冷却时间与播放状态。

import (
	"sync"
	"time"
)

const (
	// failureFile 失败提示音（警告及以上、启动失败、端口被占用）
	failureFile = "cartoon-fail.mp3"
	// notifyFile 资源产出完成提示音（图像生成 / 上传 / 打包归档）
	notifyFile = "prompt-tone.mp3"
	// failureCooldown 失败提示音最小间隔：日志可能出现连续告警，避免刷屏式播放
	failureCooldown = 10 * time.Second
	// notifyCooldown 完成提示音最小间隔
	notifyCooldown = 3 * time.Second
)

// 提示音种类
const (
	KindFailure Kind = iota
	KindNotify
)

var (
	// stateMu 保护播放状态与上次播放时间
	stateMu sync.Mutex
	// playing 各提示音是否正在播放（异步播放期间不重复触发）
	playing [2]bool
	// lastPlayed 各提示音上次触发时间（冷却判断）
	lastPlayed [2]time.Time
)
