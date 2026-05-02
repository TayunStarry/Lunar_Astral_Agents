
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"time"

	"github.com/webview/webview_go"
)

func main() {
	cmd := exec.Command(".\\python\\python.exe", "app.pyc")
	cmd.Dir = "."

	env := os.Environ()
	env = append(env, "PYTHONUSERBASE=.\\python\\Lib\\site-packages")
	env = append(env, "PYTHONPATH=.\\python\\Lib\\site-packages")
	pathEnv := os.Getenv("PATH")
	env = append(env, "PATH="+pathEnv+";.\\python\\Scripts")
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Error creating stdout pipe: %v\n", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("Error creating stderr pipe: %v\n", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		fmt.Printf("Error starting Python process: %v\n", err)
		return
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGTERM)
			time.Sleep(500 * time.Millisecond)
			cmd.Process.Kill()
		}
	}()

	urlChan := make(chan string, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		re := regexp.MustCompile(`Uvicorn running on (https?://[^\s]+)`)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			matches := re.FindStringSubmatch(line)
			if matches != nil {
				urlChan <- matches[1]
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		re := regexp.MustCompile(`Uvicorn running on (https?://[^\s]+)`)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			matches := re.FindStringSubmatch(line)
			if matches != nil {
				urlChan <- matches[1]
			}
		}
	}()

	var url string
	startTime := time.Now()
	for {
		select {
		case url = <-urlChan:
			goto found
		default:
			if time.Since(startTime) > 30*time.Second {
				fmt.Println("Timeout waiting for server to start")
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

found:
	fmt.Printf("Opening webview at: %s\n", url)

	w := webview.New(true)
	defer w.Destroy()

	w.SetTitle("MOSS-TTS-NANO")
	w.SetSize(1280, 1280, webview.HintNone)
	w.Navigate(url)

	w.Run()

	if cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(1 * time.Second)
		cmd.Process.Kill()
	}
}
