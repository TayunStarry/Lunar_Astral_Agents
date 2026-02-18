package processor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// GenerateImage 生成图片
func (class *Handle) GenerateImage(prompt, negativePrompt string, useReference bool, strength, cfgScale float64) (string, error) {
	// 获取生成的图片列表
	fileListURL := class.baseURL + "/file_list/generated"
	fileListResp, err := http.Get(fileListURL)
	if err != nil {
		return "", fmt.Errorf("获取文件列表失败: %v", err)
	}
	defer fileListResp.Body.Close()

	// 解析文件列表
	var fileList []struct {
		Path         string `json:"path"`
		LastModified string `json:"lastModified"`
	}
	if decodeErr := json.NewDecoder(fileListResp.Body).Decode(&fileList); decodeErr != nil {
		return "", fmt.Errorf("解析文件列表失败: %v", decodeErr)
	}

	// 排序文件列表, 取最新生成的图片
	var imageUrl string
	if len(fileList) > 0 {
		// 按最后修改时间排序
		for i := 0; i < len(fileList)-1; i++ {
			for j := i + 1; j < len(fileList); j++ {
				if fileList[i].LastModified < fileList[j].LastModified {
					fileList[i], fileList[j] = fileList[j], fileList[i]
				}
			}
		}
		imageUrl = fileList[0].Path
	}

	// 定义图片生成数据
	generateData := map[string]any{
		"prompt":          prompt,
		"negative_prompt": negativePrompt,
		"batch_size":      1,
		"width":           512,
		"height":          512,
		"steps":           20,
		"seed":            time.Now().UnixNano() % 1000000000,
		"cfg_scale":       cfgScale,
		"init_img":        nil,
		"strength":        strength,
	}

	// 如果使用参考图片
	if useReference && imageUrl != "" {
		generateData["init_img"] = imageUrl
	}

	// 编码请求体为JSON
	body, err := json.Marshal(generateData)
	if err != nil {
		return "", fmt.Errorf("编码请求体失败: %v", err)
	}

	// 发送POST请求
	generateURL := class.baseURL + "/generate"
	req, err := http.NewRequest("POST", generateURL, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("尝试画图失败了, 失败原因是: %s", string(respBody))
	}

	// 获取图片生成任务ID
	var taskResp struct {
		TaskID string `json:"task_id"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&taskResp); decodeErr != nil {
		return "", fmt.Errorf("解析任务ID失败: %v", decodeErr)
	}

	// 轮询查询图片生成状态
	isSuccess, imageUrl, err := class.pollImageGenerationStatus(taskResp.TaskID)
	if err != nil {
		return "", err
	}

	if !isSuccess {
		return "图片生成失败，请向用户说明情况（例如：画笔暂时无法使用）", nil
	}

	// 发送图片到当前处理的群聊
	if class.currentGroupID != 0 && imageUrl != "" {
		if err := class.SendGroupImageMsg(class.currentGroupID, imageUrl); err != nil {
			log.Printf("发送群图片消息失败: %v", err)
		}
	}

	// 返回生成结果
	return fmt.Sprintf("图片绘制完成！这是你的正面提示词: [ %s ] 负面提示词: [ %s ] 请你简要描述一下画面内容，让用户更好地理解这幅画", prompt, negativePrompt), nil
}

// pollImageGenerationStatus 使用新的 /generate/wait 接口等待图片生成完成
func (class *Handle) pollImageGenerationStatus(taskID string) (bool, string, error) {
	// 构建 /generate/wait 接口的 URL
	waitURL := class.baseURL + "/generate/wait?task_id=" + taskID

	// 创建 HTTP GET 请求
	req, reqErr := http.NewRequest("GET", waitURL, nil)
	if reqErr != nil {
		return false, "", fmt.Errorf("创建请求失败: %v", reqErr)
	}

	// 设置请求头，指定接受事件流
	req.Header.Set("Accept", "text/event-stream")

	// 发送请求
	client := &http.Client{
		Timeout: 5 * time.Minute, // 设置 5 分钟超时
	}
	resp, clientErr := client.Do(req)
	if clientErr != nil {
		return false, "", fmt.Errorf("请求 /generate/wait 接口失败: %v", clientErr)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	// 读取响应体（事件流）
	reader := bufio.NewReader(resp.Body)
	for {
		// 读取一行
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				return false, "", fmt.Errorf("连接被关闭")
			}
			return false, "", fmt.Errorf("读取响应失败: %v", readErr)
		}

		// 跳过空行
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查是否是 data 行
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			// 解析 JSON 数据
			var response struct {
				Status   string `json:"status"`
				ReadPath string `json:"read_path"`
				Error    string `json:"error"`
			}

			unmarshalErr := json.Unmarshal([]byte(data), &response)
			if unmarshalErr != nil {
				return false, "", fmt.Errorf("解析响应数据失败: %v", unmarshalErr)
			}

			// 检查任务状态
			switch response.Status {
			case "completed":
				if response.ReadPath != "" {
					return true, response.ReadPath, nil
				}
				return false, "", fmt.Errorf("任务完成但未返回图片路径")
			case "failed":
				if response.Error != "" {
					return false, "", fmt.Errorf("图片绘制失败: %s", response.Error)
				}
				return false, "", fmt.Errorf("图片绘制失败")
			}
		}
	}
}
