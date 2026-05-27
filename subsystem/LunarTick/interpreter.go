package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"logger"
	"math"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Interpreter LunarTick 解释器
type Interpreter struct {
	variables     *VariableManager
	pointers      *PointerManager
	blocks        []*CodeBlock
	pendingBlocks []*CodeBlock // 下一 tick 要注入的代码块
	tickInterval  time.Duration
	currentTick   int64
	running       bool
	mu            sync.Mutex
	wg            sync.WaitGroup
	stopCh        chan struct{}
	messageCh     chan RuntimeMessage
	blockID       int
}

// NewInterpreter 创建新的解释器
func NewInterpreter() *Interpreter {
	return &Interpreter{
		variables:    NewVariableManager(),
		pointers:     NewPointerManager(),
		tickInterval: 100 * time.Millisecond,
		messageCh:    make(chan RuntimeMessage, 100),
		stopCh:       make(chan struct{}),
	}
}

// SetTickInterval 设置 tick 间隔
func (i *Interpreter) SetTickInterval(ms int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if ms < 10 {
		ms = 10
	}
	if ms > 5000 {
		ms = 5000
	}
	i.tickInterval = time.Duration(ms) * time.Millisecond
	i.variables.Set("#TICK_MS", strconv.Itoa(ms))
}

// GetMessageChannel 获取消息通道
func (i *Interpreter) GetMessageChannel() <-chan RuntimeMessage {
	return i.messageCh
}

// sendMessage 发送运行时消息
func (i *Interpreter) sendMessage(msgType MessageType, content string) {
	select {
	case i.messageCh <- RuntimeMessage{
		Type:    msgType,
		Content: content,
		Time:    time.Now().UnixMilli(),
	}:
	default:
	}
}

// log 输出日志
func (i *Interpreter) log(content string) {
	fmt.Println(content)
	i.sendMessage(MsgLog, content)
}

// error 输出错误
func (i *Interpreter) error(content string) {
	logger.Error("LunarTick", "%s", content)
	i.sendMessage(MsgError, content)
}

// generateBlockID 生成代码块 ID
func (i *Interpreter) generateBlockID() string {
	i.blockID++
	return fmt.Sprintf("block_%d", i.blockID)
}

// LoadMarkdown 从 Markdown 加载代码
func (i *Interpreter) LoadMarkdown(md string) {
	lines := strings.Split(md, "\n")
	inCodeBlock := false
	var currentBlock []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```LunarTick") {
			inCodeBlock = true
			currentBlock = nil
		} else if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock && len(currentBlock) > 0 {
				i.LoadBlock(currentBlock)
			}
			inCodeBlock = false
		} else if inCodeBlock {
			currentBlock = append(currentBlock, line)
		}
	}
}

// LoadJSON 从 JSON 加载代码
func (i *Interpreter) LoadJSON(jsonData []byte) error {
	var blocks [][]string
	if err := json.Unmarshal(jsonData, &blocks); err != nil {
		return err
	}

	for _, block := range blocks {
		i.LoadBlock(block)
	}

	return nil
}

// LoadBlock 加载单个代码块
func (i *Interpreter) LoadBlock(lines []string) {
	// 检查是否是 @lazy 块
	var nonEmptyLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	if len(nonEmptyLines) > 0 {
		firstLine := strings.TrimSpace(nonEmptyLines[0])
		if strings.HasPrefix(firstLine, "@lazy") {
			// 提取指针名
			parts := SplitArgs(firstLine)
			if len(parts) >= 2 {
				pointerName := strings.TrimPrefix(parts[1], "*")
				// 保存为 class 指针（包含除 @lazy 外的所有行）
				i.pointers.SetClass(pointerName, lines)
			}
			return
		}
	}

	// 正常代码块，加入就绪列表
	block := &CodeBlock{
		ID:    i.generateBlockID(),
		Lines: lines,
		PC:    0,
		State: StateReady,
	}
	i.blocks = append(i.blocks, block)
}

// Inject 动态注入代码块
func (i *Interpreter) Inject(lines []string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// 检查是否是 @lazy 块
	var nonEmptyLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	if len(nonEmptyLines) > 0 {
		firstLine := strings.TrimSpace(nonEmptyLines[0])
		if strings.HasPrefix(firstLine, "@lazy") {
			// 提取指针名
			parts := SplitArgs(firstLine)
			if len(parts) >= 2 {
				pointerName := strings.TrimPrefix(parts[1], "*")
				// 保存为 class 指针
				i.pointers.SetClass(pointerName, lines)
			}
			return
		}
	}

	// 正常代码块，加入待注入列表
	block := &CodeBlock{
		ID:    i.generateBlockID(),
		Lines: lines,
		PC:    0,
		State: StateReady,
	}
	i.pendingBlocks = append(i.pendingBlocks, block)
}

// Invoke 调用指针
func (i *Interpreter) Invoke(pointerName string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if ptr, ok := i.pointers.Get(pointerName); ok {
		if ptr.Type == PointerClass {
			block := &CodeBlock{
				ID:    i.generateBlockID(),
				Lines: ptr.Lines,
				PC:    0,
				State: StateReady,
			}
			i.pendingBlocks = append(i.pendingBlocks, block)
		} else if ptr.Type == PointerSnapshot {
			i.variables.RestoreAll(ptr.Snapshot.Variables)
			i.log(fmt.Sprintf("Restored snapshot %s from tick %d", pointerName, ptr.Snapshot.Tick))
		}
	}
}

// Start 启动解释器
func (i *Interpreter) Start() {
	i.mu.Lock()
	if i.running {
		i.mu.Unlock()
		return
	}
	i.running = true
	i.currentTick = 0
	i.stopCh = make(chan struct{})
	i.mu.Unlock()

	i.wg.Add(1)
	go i.tickLoop()
}

// Stop 停止解释器
func (i *Interpreter) Stop() {
	i.mu.Lock()
	if !i.running {
		i.mu.Unlock()
		return
	}
	i.running = false
	close(i.stopCh)
	i.mu.Unlock()

	i.wg.Wait()
}

// tickLoop tick 循环
func (i *Interpreter) tickLoop() {
	defer i.wg.Done()

	ticker := time.NewTicker(i.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-i.stopCh:
			return
		case <-ticker.C:
			i.doTick()
		}
	}
}

// doTick 执行一个 tick
func (i *Interpreter) doTick() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.currentTick++
	i.variables.Set("#TICK", strconv.FormatInt(i.currentTick, 10))

	// 1. 注入待处理的代码块
	if len(i.pendingBlocks) > 0 {
		i.blocks = append(i.blocks, i.pendingBlocks...)
		i.pendingBlocks = nil
	}

	// 检查是否有活跃代码块
	hasActiveBlocks := false
	for _, block := range i.blocks {
		if block.State != StateTerminated {
			hasActiveBlocks = true
			break
		}
	}

	if !hasActiveBlocks && len(i.pendingBlocks) == 0 {
		return
	}

	// 2. 执行就绪的代码块
	for _, block := range i.blocks {
		if block.State == StateReady {
			i.executeBlock(block)
		}
	}

	// 3. 检查等待条件，唤醒就绪的代码块
	for _, block := range i.blocks {
		if block.State == StateWaiting {
			i.checkWaitCondition(block)
		}
	}

	// 4. 清理已终止的代码块
	var activeBlocks []*CodeBlock
	for _, block := range i.blocks {
		if block.State != StateTerminated {
			activeBlocks = append(activeBlocks, block)
		}
	}
	i.blocks = activeBlocks
}

// executeBlock 执行代码块的当前行
func (i *Interpreter) executeBlock(block *CodeBlock) {
	// 检查超时
	if block.LimitTime != nil && time.Now().After(*block.LimitTime) {
		i.handleBlockError(block, "Timeout exceeded")
		return
	}

	// 找到下一条有效指令
	for block.PC < len(block.Lines) {
		line := block.Lines[block.PC]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			block.PC++
			continue
		}

		// 执行指令
		if err := i.executeInstruction(block, line); err != nil {
			i.handleBlockError(block, err.Error())
			return
		}

		block.PC++

		// 检查是否进入等待状态
		if block.State == StateWaiting {
			return
		}

		// 每个 tick 只执行一条指令
		break
	}

	// 检查是否执行完毕
	if block.PC >= len(block.Lines) {
		block.State = StateTerminated
	}
}

// handleBlockError 处理代码块错误
func (i *Interpreter) handleBlockError(block *CodeBlock, errMsg string) {
	if block.RetryConfig != nil && block.RetryConfig.CurrentRetry < block.RetryConfig.MaxRetries {
		// 记录错误
		if block.RetryConfig.ErrorVar != "" {
			i.variables.Add(block.RetryConfig.ErrorVar, errMsg+"\n")
		}

		block.RetryConfig.CurrentRetry++
		i.log(fmt.Sprintf("Retrying block %s (%d/%d)", block.ID, block.RetryConfig.CurrentRetry, block.RetryConfig.MaxRetries))

		// 冷却后重试
		block.PC = 0
		block.Lines = block.RetryConfig.OriginalLines
		block.State = StateWaiting
		block.WaitCond = WaitCondition{
			Type:       WaitSleep,
			SleepUntil: time.Now().Add(time.Duration(block.RetryConfig.CooldownMs) * time.Millisecond),
		}
	} else {
		block.State = StateTerminated
		i.error(fmt.Sprintf("Block %s error: %s", block.ID, errMsg))
	}
}

// checkWaitCondition 检查等待条件
func (i *Interpreter) checkWaitCondition(block *CodeBlock) {
	switch block.WaitCond.Type {
	case WaitVariable:
		if val := i.variables.Get(block.WaitCond.VariableName); val != "" {
			block.State = StateReady
			block.WaitCond = WaitCondition{Type: WaitNone}
		}
	case WaitSleep:
		if time.Now().After(block.WaitCond.SleepUntil) {
			block.State = StateReady
			block.WaitCond = WaitCondition{Type: WaitNone}
		}
	case WaitProcess:
		select {
		case <-block.WaitCond.ProcessExitCh:
			block.State = StateReady
			block.WaitCond = WaitCondition{Type: WaitNone}
		default:
		}
	}
}

// executeInstruction 执行一条指令
func (i *Interpreter) executeInstruction(block *CodeBlock, line string) error {
	directive, args := ParseLine(line)

	// 检查是否是指针调用
	if strings.HasPrefix(strings.TrimSpace(line), "*") {
		pointerName := strings.TrimPrefix(strings.TrimSpace(line), "*")
		return i.executePointerCall(pointerName)
	}

	// 处理没有 @ 前缀的 SET 指令
	if directive == "" && len(args) >= 2 {
		firstArg := strings.TrimSpace(args[0])
		if strings.EqualFold(firstArg, "SET") || strings.EqualFold(firstArg, "#SET") {
			return i.executeSet(args[1:])
		}
	}

	switch directive {
	case "@log":
		return i.executeLog(args)
	case "@set", "SET":
		return i.executeSet(args)
	case "@add":
		return i.executeAdd(args)
	case "@wrt":
		return i.executeWrt(args)
	case "@ron":
		return i.executeRon(args)
	case "@unl":
		return i.executeUnl(args)
	case "@wait":
		return i.executeWait(block, args)
	case "@sleep":
		return i.executeSleep(block, args)
	case "@run":
		return i.executeRun(block, args)
	case "@catch":
		return i.executeCatch(block, args)
	case "@if":
		return i.executeIf(args)
	case "@cycle":
		return i.executeCycle(args)
	case "@build":
		return i.executeBuild(block, args)
	case "@def":
		return i.executeDef(args)
	case "@stop":
		block.State = StateTerminated
		return nil
	case "@limit":
		return i.executeLimit(block, args)
	case "@write":
		return i.executeWrite(args)
	case "@read":
		return i.executeRead(args)
	case "@web":
		return i.executeWeb(args)
	case "@retry":
		return i.executeRetry(block, args)
	case "@filter":
		return i.executeFilter(args)
	case "@math":
		return i.executeMath(args)
	case "@lazy":
		// @lazy 只在加载时处理，运行时忽略
		return nil
	default:
		if directive != "" {
			return fmt.Errorf("unknown directive: %s", directive)
		}
	}

	return nil
}

// executeLog 执行 @log
func (i *Interpreter) executeLog(args []string) error {
	if len(args) == 0 {
		return nil
	}

	content := i.evalExpression(strings.Join(args, " "))
	i.log(content)
	return nil
}

// executeSet 执行 SET
func (i *Interpreter) executeSet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("SET requires at least 2 arguments")
	}

	varName := strings.TrimPrefix(args[0], "#")
	expr := strings.Join(args[1:], " ")
	value := i.evalExpression(expr)
	value = UnquoteString(value)
	i.variables.Set(varName, value)
	return nil
}

// executeAdd 执行 ADD
func (i *Interpreter) executeAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("ADD requires at least 2 arguments")
	}

	varName := strings.TrimPrefix(args[0], "#")
	expr := strings.Join(args[1:], " ")
	value := i.evalExpression(expr)
	i.variables.Add(varName, value)
	return nil
}

// executeWrt 执行 WRT
func (i *Interpreter) executeWrt(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("WRT requires at least 2 arguments")
	}

	varName := strings.TrimPrefix(args[0], "#")
	expr := strings.Join(args[1:], " ")
	value := i.evalExpression(expr)
	i.variables.SetMode(varName, value, VarModeWriteOnly)
	return nil
}

// executeRon 执行 RON
func (i *Interpreter) executeRon(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("RON requires at least 2 arguments")
	}

	varName := strings.TrimPrefix(args[0], "#")
	expr := strings.Join(args[1:], " ")
	value := i.evalExpression(expr)
	i.variables.SetMode(varName, value, VarModeReadOnly)
	return nil
}

// executeUnl 执行 UNL
func (i *Interpreter) executeUnl(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("UNL requires at least 2 arguments")
	}

	varName := strings.TrimPrefix(args[0], "#")
	expr := strings.Join(args[1:], " ")
	value := i.evalExpression(expr)
	i.variables.Unlock(varName, value)
	return nil
}

// executeWait 执行 @wait
func (i *Interpreter) executeWait(block *CodeBlock, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("@wait requires a variable argument")
	}

	varName := strings.TrimPrefix(args[0], "#")
	block.State = StateWaiting
	block.WaitCond = WaitCondition{
		Type:         WaitVariable,
		VariableName: varName,
	}
	return nil
}

// executeSleep 执行 @sleep
func (i *Interpreter) executeSleep(block *CodeBlock, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("@sleep requires milliseconds argument")
	}

	ms, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	block.State = StateWaiting
	block.WaitCond = WaitCondition{
		Type:       WaitSleep,
		SleepUntil: time.Now().Add(time.Duration(ms) * time.Millisecond),
	}
	return nil
}

// executeRun 执行 @run
func (i *Interpreter) executeRun(block *CodeBlock, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("@run requires a command")
	}

	cmdName := args[0]
	cmdArgs := args[1:]

	// 评估参数中的变量
	for idx, arg := range cmdArgs {
		cmdArgs[idx] = i.evalExpression(arg)
	}

	cmd := exec.Command(cmdName, cmdArgs...)
	exitCh := make(chan int, 1)

	go func() {
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCh <- exitErr.ExitCode()
			} else {
				exitCh <- -1
			}
		} else {
			exitCh <- 0
		}
	}()

	block.State = StateWaiting
	block.WaitCond = WaitCondition{
		Type:          WaitProcess,
		ProcessExitCh: exitCh,
	}

	return nil
}

// executeCatch 执行 @catch
func (i *Interpreter) executeCatch(block *CodeBlock, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@catch requires variable and substring")
	}

	varName := strings.TrimPrefix(args[0], "#")
	substr := i.evalExpression(args[1])

	// 查找下一行的 @run
	if block.PC+1 >= len(block.Lines) {
		return fmt.Errorf("@catch must be followed by @run")
	}

	nextLine := strings.TrimSpace(block.Lines[block.PC+1])
	if !strings.HasPrefix(nextLine, "@run") {
		return fmt.Errorf("@catch must be followed by @run")
	}

	// 跳过 @run 行
	block.PC++

	// 执行 @run（非阻塞）
	_, runArgs := ParseLine(nextLine)
	if len(runArgs) < 1 {
		return fmt.Errorf("@run requires a command")
	}

	cmdName := runArgs[0]
	cmdArgs := runArgs[1:]
	for idx, arg := range cmdArgs {
		cmdArgs[idx] = i.evalExpression(arg)
	}

	cmd := exec.Command(cmdName, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// 监听输出
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				if strings.Contains(output, substr) {
					i.variables.Add(varName, output)
					// 唤醒代码块
					i.mu.Lock()
					block.State = StateReady
					block.WaitCond = WaitCondition{Type: WaitNone}
					i.mu.Unlock()
				}
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				if strings.Contains(output, substr) {
					i.variables.Add(varName, output)
					// 唤醒代码块
					i.mu.Lock()
					block.State = StateReady
					block.WaitCond = WaitCondition{Type: WaitNone}
					i.mu.Unlock()
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// 等待进程结束作为后备
	exitCh := make(chan int, 1)
	go func() {
		cmd.Wait()
		exitCh <- 0
		// 进程结束后，如果还在等待，也唤醒
		i.mu.Lock()
		if block.State == StateWaiting {
			block.State = StateReady
			block.WaitCond = WaitCondition{Type: WaitNone}
		}
		i.mu.Unlock()
	}()

	block.State = StateWaiting
	block.WaitCond = WaitCondition{
		Type:          WaitProcess,
		ProcessExitCh: exitCh,
	}

	return nil
}

// executeIf 执行 @if
func (i *Interpreter) executeIf(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("@if requires condition ? truePointer : falsePointer")
	}

	// 解析条件表达式
	var condition string
	var truePtr string
	var falsePtr string
	mode := 0 // 0: condition, 1: truePtr, 2: falsePtr

	for _, arg := range args {
		if arg == "?" {
			mode = 1
		} else if arg == ":" {
			mode = 2
		} else {
			switch mode {
			case 0:
				condition += arg + " "
			case 1:
				truePtr = strings.TrimPrefix(arg, "*")
			case 2:
				falsePtr = strings.TrimPrefix(arg, "*")
			}
		}
	}

	condition = strings.TrimSpace(condition)
	result := i.evalCondition(condition)

	var targetPtr string
	if result {
		targetPtr = truePtr
	} else {
		targetPtr = falsePtr
	}

	if targetPtr != "" {
		if ptr, ok := i.pointers.Get(targetPtr); ok && ptr.Type == PointerClass {
			block := &CodeBlock{
				ID:    i.generateBlockID(),
				Lines: ptr.Lines,
				PC:    0,
				State: StateReady,
			}
			i.pendingBlocks = append(i.pendingBlocks, block)
		}
	}

	return nil
}

// executeCycle 执行 @cycle
func (i *Interpreter) executeCycle(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@cycle requires condition and pointer")
	}

	var condition string
	var pointerName string
	foundArrow := false

	for _, arg := range args {
		if !foundArrow && arg != "" {
			if strings.HasPrefix(arg, "*") {
				pointerName = strings.TrimPrefix(arg, "*")
				foundArrow = true
			} else {
				condition += arg + " "
			}
		}
	}

	condition = strings.TrimSpace(condition)

	if i.evalCondition(condition) && pointerName != "" {
		if ptr, ok := i.pointers.Get(pointerName); ok && ptr.Type == PointerClass {
			block := &CodeBlock{
				ID:    i.generateBlockID(),
				Lines: ptr.Lines,
				PC:    0,
				State: StateReady,
			}
			i.pendingBlocks = append(i.pendingBlocks, block)
		}
	}

	return nil
}

// executeBuild 执行 @build
func (i *Interpreter) executeBuild(block *CodeBlock, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@build requires pointer name and type")
	}

	pointerName := strings.TrimPrefix(args[0], "*")
	buildType := args[1]

	if buildType == "class" {
		// 收集除 @build 和 @def 外的所有行
		var lines []string
		for _, line := range block.Lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "@build") && !strings.HasPrefix(trimmed, "@def") {
				lines = append(lines, line)
			}
		}
		i.pointers.SetClass(pointerName, lines)
	} else if buildType == "snapshot" {
		snapshot := SnapshotState{
			Variables: i.variables.GetAll(),
			Tick:      i.currentTick,
		}
		i.pointers.SetSnapshot(pointerName, snapshot)
		i.log(fmt.Sprintf("Created snapshot %s at tick %d", pointerName, i.currentTick))
	}

	return nil
}

// executeDef 执行 @def
func (i *Interpreter) executeDef(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("@def requires a pointer name")
	}

	pointerName := strings.TrimPrefix(args[0], "*")
	// 定义空指针
	i.pointers.SetClass(pointerName, nil)
	return nil
}

// executeLimit 执行 @limit
func (i *Interpreter) executeLimit(block *CodeBlock, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("@limit requires milliseconds")
	}

	ms, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	limitTime := time.Now().Add(time.Duration(ms) * time.Millisecond)
	block.LimitTime = &limitTime
	return nil
}

// executeWrite 执行 @write
func (i *Interpreter) executeWrite(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@write requires path and content variables")
	}

	pathVar := strings.TrimPrefix(args[0], "#")
	contentVar := strings.TrimPrefix(args[1], "#")

	path := i.variables.Get(pathVar)
	content := i.variables.Get(contentVar)

	return os.WriteFile(path, []byte(content), 0644)
}

// executeRead 执行 @read
func (i *Interpreter) executeRead(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@read requires path and target variables")
	}

	pathVar := strings.TrimPrefix(args[0], "#")
	targetVar := strings.TrimPrefix(args[1], "#")

	path := i.variables.Get(pathVar)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	i.variables.Set(targetVar, string(data))
	return nil
}

// executeWeb 执行 @web
func (i *Interpreter) executeWeb(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("@web requires a URL")
	}

	url := i.evalExpression(args[0])
	return openBrowser(url)
}

// openBrowser 使用系统默认浏览器打开 URL
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

// executeRetry 执行 @retry
func (i *Interpreter) executeRetry(block *CodeBlock, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("@retry requires cooldown, max retries, and error variable")
	}

	cooldown, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	maxRetries, err := strconv.Atoi(args[1])
	if err != nil {
		return err
	}

	errorVar := strings.TrimPrefix(args[2], "#")

	block.RetryConfig = &RetryConfig{
		CooldownMs:    cooldown,
		MaxRetries:    maxRetries,
		ErrorVar:      errorVar,
		CurrentRetry:  0,
		OriginalLines: append([]string{}, block.Lines...),
	}

	return nil
}

// executeFilter 执行 @filter
func (i *Interpreter) executeFilter(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@filter requires variable and regex")
	}

	varName := strings.TrimPrefix(args[0], "#")
	pattern := i.evalExpression(args[1])

	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	input := i.variables.Get(varName)
	matches := re.FindStringSubmatch(input)

	if len(matches) > 1 {
		i.variables.Set(varName, matches[1])
	} else if len(matches) > 0 {
		i.variables.Set(varName, matches[0])
	} else {
		i.variables.Set(varName, "")
	}

	return nil
}

// executeMath 执行 @math
func (i *Interpreter) executeMath(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("@math requires variable and expression")
	}

	varName := strings.TrimPrefix(args[0], "#")
	expr := strings.Join(args[1:], " ")
	expr = i.evalExpression(expr)

	result, err := i.calculateMath(expr)
	if err != nil {
		return err
	}

	i.variables.Set(varName, fmt.Sprintf("%g", result))
	return nil
}

// calculateMath 计算数学表达式
func (i *Interpreter) calculateMath(expr string) (float64, error) {
	// 简单的数学计算（支持 +-*/()）
	// 实际项目中可以使用更完善的数学表达式库
	expr = strings.ReplaceAll(expr, " ", "")

	// 处理 int()
	intRegex := regexp.MustCompile(`int\(([^)]+)\)`)
	for {
		matches := intRegex.FindStringSubmatch(expr)
		if len(matches) < 2 {
			break
		}
		inner, err := i.calculateMath(matches[1])
		if err != nil {
			return 0, err
		}
		expr = strings.Replace(expr, matches[0], fmt.Sprintf("%g", math.Floor(inner)), 1)
	}

	// 简单的计算器实现
	return i.simpleCalc(expr)
}

func (i *Interpreter) simpleCalc(expr string) (float64, error) {
	// 处理括号
	for strings.Contains(expr, "(") {
		start := strings.LastIndex(expr, "(")
		end := strings.Index(expr[start:], ")") + start
		if start == -1 || end == -1 {
			break
		}
		inner := expr[start+1 : end]
		val, err := i.simpleCalc(inner)
		if err != nil {
			return 0, err
		}
		expr = expr[:start] + fmt.Sprintf("%g", val) + expr[end+1:]
	}

	// 处理乘除
	for {
		mulIdx := strings.Index(expr, "*")
		divIdx := strings.Index(expr, "/")

		if mulIdx == -1 && divIdx == -1 {
			break
		}

		var idx int
		var op string
		if mulIdx != -1 && (divIdx == -1 || mulIdx < divIdx) {
			idx = mulIdx
			op = "*"
		} else {
			idx = divIdx
			op = "/"
		}

		left, leftEnd := i.parseNumber(expr, idx, -1)
		right, rightStart := i.parseNumber(expr, idx, 1)

		var result float64
		if op == "*" {
			result = left * right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			result = left / right
		}

		expr = expr[:leftEnd+1] + fmt.Sprintf("%g", result) + expr[rightStart:]
	}

	// 处理加减
	result := 0.0
	sign := 1.0
	num := ""

	for _, r := range expr {
		if r == '+' || r == '-' {
			if num != "" {
				val, _ := strconv.ParseFloat(num, 64)
				result += sign * val
				num = ""
			}
			if r == '-' {
				sign = -1
			} else {
				sign = 1
			}
		} else {
			num += string(r)
		}
	}

	if num != "" {
		val, _ := strconv.ParseFloat(num, 64)
		result += sign * val
	}

	return result, nil
}

func (i *Interpreter) parseNumber(expr string, opIdx, direction int) (float64, int) {
	start := opIdx + direction
	end := start

	for start >= 0 && start < len(expr) {
		if direction < 0 {
			// 向左找
			if (expr[start] == '+' || expr[start] == '-') && start != 0 {
				break
			}
			start--
			if start < 0 || expr[start] == '+' || expr[start] == '-' {
				start++
				break
			}
		} else {
			// 向右找
			if expr[start] == '+' || expr[start] == '-' {
				break
			}
			start++
			if start >= len(expr) {
				break
			}
		}
	}

	if direction < 0 {
		start = max(0, start)
		numStr := expr[start:opIdx]
		val, _ := strconv.ParseFloat(numStr, 64)
		return val, start - 1
	} else {
		end = opIdx + 1
		for end < len(expr) && expr[end] != '+' && expr[end] != '-' {
			end++
		}
		numStr := expr[opIdx+1 : end]
		val, _ := strconv.ParseFloat(numStr, 64)
		return val, end
	}
}

// executePointerCall 执行指针调用
func (i *Interpreter) executePointerCall(pointerName string) error {
	if ptr, ok := i.pointers.Get(pointerName); ok {
		if ptr.Type == PointerClass {
			block := &CodeBlock{
				ID:    i.generateBlockID(),
				Lines: ptr.Lines,
				PC:    0,
				State: StateReady,
			}
			i.pendingBlocks = append(i.pendingBlocks, block)
		} else if ptr.Type == PointerSnapshot {
			i.variables.RestoreAll(ptr.Snapshot.Variables)
			i.log(fmt.Sprintf("Restored snapshot %s from tick %d", pointerName, ptr.Snapshot.Tick))
		}
	}
	return nil
}

// evalExpression 评估表达式（变量替换和字符串拼接）
func (i *Interpreter) evalExpression(expr string) string {
	// 替换变量
	var result bytes.Buffer
	var currentVar bytes.Buffer
	inVar := false

	for _, r := range expr {
		if r == '#' && !inVar {
			inVar = true
			currentVar.Reset()
			currentVar.WriteRune(r)
		} else if inVar {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				currentVar.WriteRune(r)
			} else {
				// 替换变量
				varName := strings.TrimPrefix(currentVar.String(), "#")
				result.WriteString(i.variables.Get(varName))
				inVar = false
				result.WriteRune(r)
			}
		} else {
			result.WriteRune(r)
		}
	}

	if inVar {
		varName := strings.TrimPrefix(currentVar.String(), "#")
		result.WriteString(i.variables.Get(varName))
	}

	return result.String()
}

// evalCondition 评估条件表达式
func (i *Interpreter) evalCondition(expr string) bool {
	expr = i.evalExpression(expr)
	expr = strings.TrimSpace(expr)

	if expr == "" || expr == "false" {
		return false
	}
	if expr == "true" {
		return true
	}

	// 简单比较
	if strings.Contains(expr, "==") {
		parts := strings.Split(expr, "==")
		return strings.TrimSpace(parts[0]) == strings.TrimSpace(parts[1])
	}
	if strings.Contains(expr, "!=") {
		parts := strings.Split(expr, "!=")
		return strings.TrimSpace(parts[0]) != strings.TrimSpace(parts[1])
	}
	if strings.Contains(expr, ">=") {
		parts := strings.Split(expr, ">=")
		a, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		return a >= b
	}
	if strings.Contains(expr, "<=") {
		parts := strings.Split(expr, "<=")
		a, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		return a <= b
	}
	if strings.Contains(expr, ">") {
		parts := strings.Split(expr, ">")
		a, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		return a > b
	}
	if strings.Contains(expr, "<") {
		parts := strings.Split(expr, "<")
		a, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		return a < b
	}

	// 逻辑与
	if strings.Contains(expr, "&&") {
		parts := strings.Split(expr, "&&")
		for _, part := range parts {
			if !i.evalCondition(strings.TrimSpace(part)) {
				return false
			}
		}
		return true
	}

	// 逻辑或
	if strings.Contains(expr, "||") {
		parts := strings.Split(expr, "||")
		for _, part := range parts {
			if i.evalCondition(strings.TrimSpace(part)) {
				return true
			}
		}
		return false
	}

	// 非
	if strings.HasPrefix(expr, "!") {
		return !i.evalCondition(strings.TrimPrefix(expr, "!"))
	}

	// 默认非空为 true
	return true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
