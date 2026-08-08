package lunar_chromedp

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"time"

	"github.com/chromedp/chromedp"
)

// =============================================================================
// 分页截图 — 滚动截图，每屏一张，最大 10 页
// =============================================================================

// CapturePageScreenshots 对当前页面执行分页截图
// 使用 window.scrollBy(0, innerHeight) 逐屏滚动，每屏截一张
// maxPages 为最大截图数，达到上限或滚动到底部时停止
// ctx 应为浏览器上下文（chromedp.NewContext 返回的 context）
func CapturePageScreenshots(ctx context.Context, maxPages int) ([]PageScreenshot, error) {
	if maxPages <= 0 {
		maxPages = MaxScreenshotsPerPage
	}

	// 获取页面总高度和视口高度
	var pageHeight, viewportHeight float64
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.body.scrollHeight`, &pageHeight),
		chromedp.Evaluate(`window.innerHeight`, &viewportHeight),
	); err != nil {
		return nil, fmt.Errorf("获取页面尺寸失败: %w", err)
	}

	// 获取视口宽度
	var viewportWidth float64
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`window.innerWidth`, &viewportWidth),
	); err != nil {
		viewportWidth = 1920
	}

	fmt.Printf("[%s] 页面总高度=%.0fpx 视口高度=%.0fpx 最大截图=%d\n",
		ModuleName, pageHeight, viewportHeight, maxPages)

	var screenshots []PageScreenshot

	for page := 0; page < maxPages; page++ {
		// 检查是否已到底部
		var currentScrollY float64
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollY`, &currentScrollY),
		); err != nil {
			// 获取滚动位置失败，尝试继续
			currentScrollY = float64(page) * viewportHeight
		}

		// 如果当前位置超过页面高度，停止
		if currentScrollY >= pageHeight-10 {
			fmt.Printf("[%s] 已到达页面底部 (scrollY=%.0f, pageHeight=%.0f)，停止截图\n",
				ModuleName, currentScrollY, pageHeight)
			break
		}

		// 截取当前视口
		screenshot, err := captureViewport(ctx, page+1)
		if err != nil {
			return screenshots, fmt.Errorf("第 %d 页截图失败: %w", page+1, err)
		}
		screenshots = append(screenshots, screenshot)

		fmt.Printf("[%s] 第 %d 页截图完成 (%dx%d) scrollY=%.0f\n",
			ModuleName, page+1, screenshot.Width, screenshot.Height, currentScrollY)

		// 滚动到下一屏
		if err := scrollOnePage(ctx, viewportHeight); err != nil {
			fmt.Printf("[%s] 滚动失败（可能已到底部）: %v\n", ModuleName, err)
			break
		}

		// 等待懒加载内容渲染
		if err := waitForRender(ctx); err != nil {
			fmt.Printf("[%s] 等待渲染超时: %v\n", ModuleName, err)
			// 不阻断，继续截图
		}

		// 检查滚动后是否到达底部（位置未变化）
		var newScrollY float64
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollY`, &newScrollY),
		); err == nil {
			if newScrollY <= currentScrollY+5 {
				fmt.Printf("[%s] 滚动位置未变化 (%.0f→%.0f)，已到页面底部\n",
					ModuleName, currentScrollY, newScrollY)
				break
			}
		}
	}

	fmt.Printf("[%s] 分页截图完成，共 %d 页\n", ModuleName, len(screenshots))
	return screenshots, nil
}

// captureViewport 截取当前视口并编码为 PNG
func captureViewport(ctx context.Context, pageNum int) (PageScreenshot, error) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return PageScreenshot{}, err
	}

	// 解码以获取实际尺寸
	img, err := png.Decode(bytes.NewReader(buf))
	if err != nil {
		return PageScreenshot{}, fmt.Errorf("解码截图失败: %w", err)
	}

	bounds := img.Bounds()

	return PageScreenshot{
		ImageData:  buf,
		PageNumber: pageNum,
		Width:      bounds.Dx(),
		Height:     bounds.Dy(),
	}, nil
}

// scrollOnePage 向下滚动一屏高度
// 使用 window.scrollBy(0, innerHeight) 精确滚动一个视口高度
func scrollOnePage(ctx context.Context, pageHeight float64) error {
	return chromedp.Run(ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`window.scrollBy({top: %f, behavior: 'instant'})`, pageHeight),
			nil,
		),
	)
}

// waitForRender 等待页面渲染完成（懒加载图片、动态内容等）
// 策略：等待网络空闲 + 额外固定延迟，确保异步内容加载
func waitForRender(ctx context.Context) error {
	// 等待网络请求基本完成（最多 3 秒）
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 检查页面是否仍在加载中，使用 document.readyState
	var readyState string
	if err := chromedp.Run(waitCtx,
		chromedp.Evaluate(`document.readyState`, &readyState),
	); err != nil {
		return err
	}

	if readyState == "loading" {
		// 等待 readyState 变为 complete
		time.Sleep(500 * time.Millisecond)
		if err := chromedp.Run(waitCtx,
			chromedp.Evaluate(`document.readyState`, &readyState),
		); err != nil {
			return err
		}
	}

	// 等待所有 img 标签的 loading 完成
	var loadingImages int
	chromedp.Run(waitCtx,
		chromedp.Evaluate(
			`document.querySelectorAll('img[loading="lazy"]:not([complete])').length`,
			&loadingImages,
		),
	)

	// 给懒加载图片额外的渲染时间
	if loadingImages > 0 {
		time.Sleep(1 * time.Second)
	} else {
		time.Sleep(300 * time.Millisecond)
	}

	return nil
}

// CaptureFullPageScreenshot 截取单个页面视口截图（不滚动）
// 用于需要快速获取页面截图的场景（如搜索结果页预览）
func CaptureFullPageScreenshot(ctx context.Context) ([]byte, error) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return nil, fmt.Errorf("截图失败: %w", err)
	}
	return buf, nil
}
