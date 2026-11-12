package viewer

import "testing"

// TestFinishComment 钉住评论长度收束：在句子边界收尾，绝不从词中间硬切。
func TestFinishComment(t *testing.T) {
	// 不超长时原样返回
	if got := finishComment("这个转场好丝滑", 30); got != "这个转场好丝滑" {
		t.Fatalf("未超长不应改写，实际 %q", got)
	}
	// 超长时退到最近的句号，保留完整句子
	long := "开头铺垫有点长，但中间那段运镜真的绝了。后面节奏又慢下来了，我差点退出。"
	got := finishComment(long, 25)
	if got != "开头铺垫有点长，但中间那段运镜真的绝了。" {
		t.Fatalf("应在句号处收尾，实际 %q", got)
	}
	// 没有句末标点时退到逗号
	got = finishComment("这个UP主讲得特别细，每一步都拆开讲，但我还是没听懂", 15)
	if got != "这个UP主讲得特别细，" {
		t.Fatalf("应在逗号处收尾，实际 %q", got)
	}
	// 首尾引号会被清理
	if got := finishComment(`“太真实了”`, 30); got != "太真实了" {
		t.Fatalf("应清理首尾引号，实际 %q", got)
	}
}
