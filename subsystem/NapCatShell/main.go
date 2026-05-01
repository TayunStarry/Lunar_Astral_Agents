package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"

	"github.com/webview/webview_go"
)

type Config struct {
	NapCatShell struct {
		NapcatWinBootMainPath string `json:"napcat_win_boot_main_path"`
	} `json:"nap_cat_shell"`
}

func main() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("获取当前程序路径失败: %v\n", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "local_data", "lunar_config.json")

	configFile, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("读取配置文件失败: %v\n", err)
		return
	}

	var config Config
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		fmt.Printf("解析配置文件失败: %v\n", err)
		return
	}

	napcatPath := config.NapCatShell.NapcatWinBootMainPath
	if napcatPath == "" {
		fmt.Println("配置文件中未指定 NapCatWinBootMain.exe 路径")
		return
	}

	napcatDir := filepath.Dir(napcatPath)
	cmd := exec.Command(napcatPath)
	cmd.Dir = napcatDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("创建标准输出管道失败: %v\n", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("创建标准错误管道失败: %v\n", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		fmt.Printf("启动 NapCatWinBootMain.exe 失败: %v\n", err)
		return
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	var wg sync.WaitGroup
	urlChan := make(chan string, 1)
	urlFound := false
	urlRegex := regexp.MustCompile(`WebUi User Panel Url: (http://[^\s]+)`)

	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			if !urlFound {
				if matches := urlRegex.FindStringSubmatch(line); matches != nil {
					urlChan <- matches[1]
					urlFound = true
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			if !urlFound {
				if matches := urlRegex.FindStringSubmatch(line); matches != nil {
					urlChan <- matches[1]
					urlFound = true
				}
			}
		}
	}()

	var webUiUrl string
	select {
	case webUiUrl = <-urlChan:
		fmt.Printf("找到 WebUi 地址: %s\n", webUiUrl)
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() {
			cmd.Wait()
			close(ch)
		}()
		return ch
	}():
		fmt.Println("NapCatWinBootMain.exe 已退出")
		return
	}

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("NapCat Shell")
	w.SetSize(1280, 720, webview.HintNone)
	w.Navigate(webUiUrl)
	w.Bind("exit", func() {
		w.Terminate()
	})

	go func() {
		wg.Wait()
		cmd.Wait()
		w.Terminate()
	}()

	w.Run()

	if cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
}
