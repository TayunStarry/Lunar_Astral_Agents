package engine

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type InstructionResult int

const (
	ResultContinue InstructionResult = iota
	ResultBlocked
	ResultTerminated
	ResultError
)

type ExecutionContext struct {
	Block   *CodeBlock
	Instr   Instruction
	VS      *VarStore
	PR      *PointerRegistry
	BM      *BlockManager
	PE      *ProcessExecutor
	TickNum int
	LogFn   func(msg string)
}

func ExecuteInstruction(ctx *ExecutionContext) InstructionResult {
	switch ctx.Instr.Type {
	case InstrSET:
		return execSET(ctx)
	case InstrADD:
		return execADD(ctx)
	case InstrWRT:
		return execWRT(ctx)
	case InstrRON:
		return execRON(ctx)
	case InstrUNL:
		return execUNL(ctx)
	case InstrRun:
		return execRun(ctx)
	case InstrCatch:
		return execCatch(ctx)
	case InstrCall:
		return execCall(ctx)
	case InstrWait:
		return execWait(ctx)
	case InstrSleep:
		return execSleep(ctx)
	case InstrFilter:
		return execFilter(ctx)
	case InstrMath:
		return execMath(ctx)
	case InstrIf:
		return execIf(ctx)
	case InstrCycle:
		return execCycle(ctx)
	case InstrRetry:
		return execRetry(ctx)
	case InstrWrite:
		return execWrite(ctx)
	case InstrRead:
		return execRead(ctx)
	case InstrLog:
		return execLog(ctx)
	case InstrWeb:
		return execWeb(ctx)
	case InstrStop:
		return execStop(ctx)
	case InstrLimit:
		return execLimit(ctx)
	case InstrDef:
		return execDef(ctx)
	case InstrLazy:
		return execLazy(ctx)
	case InstrBuild:
		return execBuild(ctx)
	case InstrEnd:
		return execEnd(ctx)
	case InstrPtr:
		return execPtr(ctx)
	default:
		return ResultContinue
	}
}

func resolveArg(arg InstructionArg, vs *VarStore) string {
	switch arg.Type {
	case ArgLiteral:
		return arg.Value
	case ArgVariable:
		return vs.Get(arg.Value)
	default:
		return arg.Value
	}
}

func resolveAllArgs(args []InstructionArg, vs *VarStore) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = resolveArg(arg, vs)
	}
	return result
}

func execSET(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	value := ""
	if len(ctx.Instr.Args) >= 2 {
		value = EvalExpression(strings.Join(resolveAllArgs(ctx.Instr.Args[1:], ctx.VS), " "), ctx.VS, ctx.PR)
	}

	ctx.VS.Set(varName, value)
	return ResultContinue
}

func execADD(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	value := ""
	if len(ctx.Instr.Args) >= 2 {
		value = EvalExpression(strings.Join(resolveAllArgs(ctx.Instr.Args[1:], ctx.VS), " "), ctx.VS, ctx.PR)
	}

	ctx.VS.Add(varName, value)
	return ResultContinue
}

func execWRT(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	value := ""
	if len(ctx.Instr.Args) >= 2 {
		value = EvalExpression(strings.Join(resolveAllArgs(ctx.Instr.Args[1:], ctx.VS), " "), ctx.VS, ctx.PR)
	}

	ctx.VS.Set(varName, value)
	ctx.VS.SetMode(varName, ModeWriteOnly)
	return ResultContinue
}

func execRON(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	value := ""
	if len(ctx.Instr.Args) >= 2 {
		value = EvalExpression(strings.Join(resolveAllArgs(ctx.Instr.Args[1:], ctx.VS), " "), ctx.VS, ctx.PR)
	}

	ctx.VS.Set(varName, value)
	ctx.VS.SetMode(varName, ModeReadOnly)
	return ResultContinue
}

func execUNL(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	value := ""
	if len(ctx.Instr.Args) >= 2 {
		value = EvalExpression(strings.Join(resolveAllArgs(ctx.Instr.Args[1:], ctx.VS), " "), ctx.VS, ctx.PR)
	}

	ctx.VS.SetMode(varName, ModeNormal)
	ctx.VS.Add(varName, value)
	return ResultContinue
}

func execRun(ctx *ExecutionContext) InstructionResult {
	args := resolveAllArgs(ctx.Instr.Args, ctx.VS)
	if len(args) == 0 {
		return ResultError
	}

	path := args[0]
	params := args[1:]

	if ctx.Block.ProcessHandle != nil {
		return ResultContinue
	}

	_, err := ctx.PE.Start(ctx.Block.ID, path, params)
	if err != nil {
		if ctx.LogFn != nil {
			ctx.LogFn(fmt.Sprintf("RUN error: %v", err))
		}
		ctx.VS.Set("?", "error:"+err.Error())
		return ResultContinue
	}

	cond := WaitCondition{
		Type: WaitProcess,
		Done: ctx.PE.WaitDone(ctx.Block.ID),
	}
	ctx.BM.MoveToWaiting(ctx.Block.ID, cond)
	return ResultBlocked
}

func execCatch(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	substr := ""
	if len(ctx.Instr.Args) >= 2 {
		substr = resolveArg(ctx.Instr.Args[1], ctx.VS)
	}

	ctx.Block.CatchVar = varName
	ctx.Block.CatchSubstr = substr

	cond := WaitCondition{
		Type: WaitCatch,
		Var:  ctx.Block.ID,
	}
	ctx.BM.MoveToWaiting(ctx.Block.ID, cond)
	return ResultBlocked
}

func execCall(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 2 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	secondArg := ctx.Instr.Args[1]
	if secondArg.Type != ArgPointer {
		return ResultError
	}

	ctx.Block.CallVar = firstArg.Value
	ctx.Block.CallPtr = secondArg.Value
	ctx.Block.CallRunning = true
	return ResultContinue
}

func execWait(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	if varName == "?" {
		if ctx.Block.ProcessHandle != nil {
			cond := WaitCondition{
				Type: WaitProcess,
				Done: ctx.PE.WaitDone(ctx.Block.ID),
			}
			ctx.BM.MoveToWaiting(ctx.Block.ID, cond)
			return ResultBlocked
		}
		return ResultContinue
	}

	val := ctx.VS.Get(varName)
	if val != "" {
		return ResultContinue
	}

	cond := WaitCondition{
		Type: WaitVariable,
		Var:  varName,
	}
	ctx.BM.MoveToWaiting(ctx.Block.ID, cond)
	return ResultBlocked
}

func execSleep(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	msStr := resolveArg(ctx.Instr.Args[0], ctx.VS)
	ms, err := strconv.Atoi(msStr)
	if err != nil {
		ms = 0
	}

	until := time.Now().Add(time.Duration(ms) * time.Millisecond)
	cond := WaitCondition{
		Type:  WaitSleep,
		Until: until,
	}
	ctx.BM.MoveToWaiting(ctx.Block.ID, cond)
	return ResultBlocked
}

func execFilter(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 2 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	pattern := resolveArg(ctx.Instr.Args[1], ctx.VS)

	currentVal := ctx.VS.Get(varName)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ResultContinue
	}

	matches := re.FindStringSubmatch(currentVal)
	if len(matches) > 1 {
		ctx.VS.Set(varName, matches[1])
	} else if len(matches) == 1 {
		ctx.VS.Set(varName, matches[0])
	}

	return ResultContinue
}

func execMath(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 2 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	if firstArg.Type != ArgVariable {
		return ResultError
	}

	varName := firstArg.Value
	expr := resolveArg(ctx.Instr.Args[1], ctx.VS)

	varsRe := regexp.MustCompile(`#([a-zA-Z_][a-zA-Z0-9_]*)`)
	expr = varsRe.ReplaceAllStringFunc(expr, func(match string) string {
		val := ctx.VS.Get(match[1:])
		if val == "" {
			return "0"
		}
		return val
	})

	result := evaluateArithmetic(expr)
	ctx.VS.Set(varName, strconv.FormatFloat(result, 'f', -1, 64))
	return ResultContinue
}

func evaluateArithmetic(expr string) float64 {
	expr = strings.ReplaceAll(expr, " ", "")

	if strings.Contains(expr, "int(") {
		re := regexp.MustCompile(`int\(([^)]+)\)`)
		expr = re.ReplaceAllStringFunc(expr, func(match string) string {
			inner := re.FindStringSubmatch(match)[1]
			val := evaluateArithmetic(inner)
			return strconv.Itoa(int(val))
		})
	}

	return evalSimpleArithmetic(expr)
}

func evalSimpleArithmetic(expr string) float64 {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0
	}

	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == ')' {
			depth := 1
			j := i - 1
			for j >= 0 && depth > 0 {
				if expr[j] == ')' {
					depth++
				} else if expr[j] == '(' {
					depth--
				}
				j--
			}
			if depth == 0 {
				inner := expr[j+2 : i]
				left := expr[:j+1]
				right := expr[i+1:]

				innerVal := evalSimpleArithmetic(inner)
				if right == "" && left == "" {
					return innerVal
				}
				return evalSimpleArithmetic(fmt.Sprintf("%s%v%s", left, innerVal, right))
			}
		}
	}

	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '+' && i > 0 && expr[i-1] != '+' && expr[i-1] != '-' && expr[i-1] != '*' && expr[i-1] != '/' {
			left := evalSimpleArithmetic(expr[:i])
			right := evalSimpleArithmetic(expr[i+1:])
			return left + right
		}
		if expr[i] == '-' && i > 0 && expr[i-1] != '+' && expr[i-1] != '-' && expr[i-1] != '*' && expr[i-1] != '/' {
			left := evalSimpleArithmetic(expr[:i])
			right := evalSimpleArithmetic(expr[i+1:])
			return left - right
		}
	}

	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '*' {
			left := evalSimpleArithmetic(expr[:i])
			right := evalSimpleArithmetic(expr[i+1:])
			return left * right
		}
		if expr[i] == '/' {
			left := evalSimpleArithmetic(expr[:i])
			right := evalSimpleArithmetic(expr[i+1:])
			if right == 0 {
				return 0
			}
			return left / right
		}
	}

	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '%' {
			left := evalSimpleArithmetic(expr[:i])
			right := evalSimpleArithmetic(expr[i+1:])
			if right == 0 {
				return 0
			}
			return float64(int(left) % int(right))
		}
	}

	if expr[0] == '(' && expr[len(expr)-1] == ')' {
		return evalSimpleArithmetic(expr[1 : len(expr)-1])
	}

	if expr[0] == '-' {
		return -evalSimpleArithmetic(expr[1:])
	}

	val, err := strconv.ParseFloat(expr, 64)
	if err != nil {
		return 0
	}
	return val
}

func execIf(ctx *ExecutionContext) InstructionResult {
	args := ctx.Instr.Args
	if len(args) < 2 {
		return ResultContinue
	}

	cond := resolveArg(args[0], ctx.VS)

	var truePtr, falsePtr string
	for i := 1; i < len(args); i++ {
		if args[i].Type == ArgPointer {
			if truePtr == "" {
				truePtr = args[i].Value
			} else {
				falsePtr = args[i].Value
			}
		}
	}

	result := EvalCondition(cond, ctx.VS, ctx.PR)
	targetPtr := falsePtr
	if result {
		targetPtr = truePtr
	}

	if targetPtr != "" && ctx.PR.Exists(targetPtr) {
		ptr, _ := ctx.PR.Get(targetPtr)
		if ptr.Type == PointerClass && len(ptr.Lines) > 0 {
			newBlock := ctx.BM.CreateBlock(ptr.Lines, targetPtr)
			ctx.BM.AddPending(newBlock)
		}
	}

	return ResultContinue
}

func execCycle(ctx *ExecutionContext) InstructionResult {
	args := ctx.Instr.Args
	if len(args) < 2 {
		return ResultContinue
	}

	cond := resolveArg(args[0], ctx.VS)

	if EvalCondition(cond, ctx.VS, ctx.PR) {
		for _, arg := range args[1:] {
			if arg.Type == ArgPointer && ctx.PR.Exists(arg.Value) {
				ptr, _ := ctx.PR.Get(arg.Value)
				if ptr.Type == PointerClass && len(ptr.Lines) > 0 {
					newBlock := ctx.BM.CreateBlock(ptr.Lines, arg.Value)
					ctx.BM.AddPending(newBlock)
				}
			}
		}
		return ResultContinue
	}

	return ResultContinue
}

func execRetry(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 3 {
		return ResultError
	}

	coolStr := resolveArg(ctx.Instr.Args[0], ctx.VS)
	maxStr := resolveArg(ctx.Instr.Args[1], ctx.VS)

	cool, _ := strconv.Atoi(coolStr)
	maxRetry, _ := strconv.Atoi(maxStr)

	ctx.Block.RetryCool = time.Duration(cool) * time.Millisecond
	ctx.Block.RetryMax = maxRetry
	if ctx.Instr.Args[2].Type == ArgVariable {
		ctx.Block.RetryErrVar = ctx.Instr.Args[2].Value
	}

	if maxRetry < 1 {
		return ResultContinue
	}

	return ResultContinue
}

func execWrite(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 2 {
		return ResultError
	}

	path := resolveArg(ctx.Instr.Args[0], ctx.VS)
	content := resolveArg(ctx.Instr.Args[1], ctx.VS)

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil && ctx.LogFn != nil {
		ctx.LogFn(fmt.Sprintf("WRITE error: %v", err))
	}

	return ResultContinue
}

func execRead(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 2 {
		return ResultError
	}

	firstArg := ctx.Instr.Args[0]
	secondArg := ctx.Instr.Args[1]
	if secondArg.Type != ArgVariable {
		return ResultError
	}

	path := resolveArg(firstArg, ctx.VS)
	targetVar := secondArg.Value

	data, err := os.ReadFile(path)
	if err != nil {
		if ctx.LogFn != nil {
			ctx.LogFn(fmt.Sprintf("READ error: %v", err))
		}
		return ResultContinue
	}

	ctx.VS.Set(targetVar, string(data))
	return ResultContinue
}

func execLog(ctx *ExecutionContext) InstructionResult {
	args := resolveAllArgs(ctx.Instr.Args, ctx.VS)
	msg := strings.Join(args, " ")

	if len(args) == 0 && ctx.Instr.Line != "" {
		msg = ctx.Instr.Line
	}

	if ctx.LogFn != nil {
		ctx.LogFn(msg)
	} else {
		fmt.Println(msg)
	}

	return ResultContinue
}

func execWeb(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	url := resolveArg(ctx.Instr.Args[0], ctx.VS)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil && ctx.LogFn != nil {
		ctx.LogFn(fmt.Sprintf("WEB error: %v", err))
	}

	return ResultContinue
}

func execStop(ctx *ExecutionContext) InstructionResult {
	ctx.BM.Terminate(ctx.Block.ID)
	return ResultTerminated
}

func execLimit(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	msStr := resolveArg(ctx.Instr.Args[0], ctx.VS)
	ms, _ := strconv.Atoi(msStr)
	ctx.Block.LimitMS = time.Duration(ms) * time.Millisecond
	return ResultContinue
}

func execDef(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultError
	}

	for _, arg := range ctx.Instr.Args {
		if arg.Type == ArgPointer {
			ctx.PR.Define(arg.Value)
		}
	}

	return ResultContinue
}

func execLazy(ctx *ExecutionContext) InstructionResult {
	return ResultContinue
}

func execBuild(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 2 {
		return ResultError
	}

	ptrArg := ctx.Instr.Args[0]
	if ptrArg.Type != ArgPointer {
		return ResultError
	}

	ptrName := ptrArg.Value
	buildType := strings.ToLower(resolveArg(ctx.Instr.Args[1], ctx.VS))

	switch buildType {
	case "class":
		lines := make([]string, len(ctx.Block.Instructions))
		for i, instr := range ctx.Block.Instructions {
			lines[i] = instr.Line
		}
		ctx.PR.BuildClass(ptrName, lines)
	case "snapshot":
		snap := ctx.VS.Snapshot()
		ctx.PR.BuildSnapshot(ptrName, snap, ctx.TickNum)
	}

	return ResultContinue
}

func execEnd(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) >= 1 {
		ptrArg := ctx.Instr.Args[0]
		if ptrArg.Type == ArgPointer {
			snap := ctx.VS.Snapshot()
			ctx.PR.BuildSnapshot(ptrArg.Value, snap, ctx.TickNum)
		}
	}

	ctx.BM.TerminateAll()
	return ResultTerminated
}

func execPtr(ctx *ExecutionContext) InstructionResult {
	if len(ctx.Instr.Args) < 1 {
		return ResultContinue
	}

	ptrName := ctx.Instr.Args[0].Value

	entry, exists := ctx.PR.Get(ptrName)
	if !exists {
		return ResultContinue
	}

	switch entry.Type {
	case PointerClass:
		if len(entry.Lines) > 0 {
			newBlock := ctx.BM.CreateBlock(entry.Lines, ptrName)
			ctx.BM.AddPending(newBlock)
		}
	case PointerSnapshot:
		ctx.VS.Restore(entry.SnapData)
	}

	return ResultContinue
}
