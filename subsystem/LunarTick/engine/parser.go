package engine

import (
	"strings"
)

func ParseInstruction(line string) Instruction {
	line = strings.TrimSpace(line)
	result := Instruction{
		Line: line,
		Raw:  line,
	}

	if strings.HasPrefix(line, "@") {
		return parseDirective(line, &result)
	}

	if strings.HasPrefix(line, "*") {
		ptrName := strings.TrimPrefix(line, "*")
		ptrName = strings.TrimSpace(ptrName)
		result.Type = InstrPtr
		result.Args = []InstructionArg{
			{Type: ArgPointer, Value: ptrName},
		}
		return result
	}

	result.Type = InstrLog
	result.Args = []InstructionArg{
		{Type: ArgLiteral, Value: line},
	}
	return result
}

func parseDirective(line string, result *Instruction) Instruction {
	rest := strings.TrimPrefix(line, "@")
	var cmdName string

	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		cmdName = rest[:idx]
		rest = strings.TrimSpace(rest[idx+1:])
	} else {
		cmdName = rest
		rest = ""
	}

	cmdName = strings.ToUpper(cmdName)

	switch cmdName {
	case "SET":
		result.Type = InstrSET
		result.Args = parseVarExpr(rest)
	case "ADD":
		result.Type = InstrADD
		result.Args = parseVarExpr(rest)
	case "WRT":
		result.Type = InstrWRT
		result.Args = parseVarExpr(rest)
	case "RON":
		result.Type = InstrRON
		result.Args = parseVarExpr(rest)
	case "UNL":
		result.Type = InstrUNL
		result.Args = parseVarExpr(rest)
	case "RUN":
		result.Type = InstrRun
		result.Args = parseRunArgs(rest)
	case "CATCH":
		result.Type = InstrCatch
		result.Args = parseCatchArgs(rest)
	case "CALL":
		result.Type = InstrCall
		result.Args = parseCallArgs(rest)
	case "WAIT":
		result.Type = InstrWait
		result.Args = parseArgs(rest)
	case "SLEEP":
		result.Type = InstrSleep
		result.Args = parseArgs(rest)
	case "FILTER":
		result.Type = InstrFilter
		result.Args = parseArgs(rest)
	case "MATH":
		result.Type = InstrMath
		result.Args = parseArgs(rest)
	case "IF":
		result.Type = InstrIf
		result.Args = parseIfArgs(rest)
	case "CYCLE":
		result.Type = InstrCycle
		result.Args = parseCycleArgs(rest)
	case "RETRY":
		result.Type = InstrRetry
		result.Args = parseArgs(rest)
	case "WRITE":
		result.Type = InstrWrite
		result.Args = parseArgs(rest)
	case "READ":
		result.Type = InstrRead
		result.Args = parseArgs(rest)
	case "LOG":
		result.Type = InstrLog
		result.Args = parseArgs(rest)
	case "WEB":
		result.Type = InstrWeb
		result.Args = parseArgs(rest)
	case "STOP":
		result.Type = InstrStop
	case "LIMIT":
		result.Type = InstrLimit
		result.Args = parseArgs(rest)
	case "DEF":
		result.Type = InstrDef
		result.Args = parseArgs(rest)
	case "LAZY":
		result.Type = InstrLazy
		result.Args = parseArgs(rest)
	case "BUILD":
		result.Type = InstrBuild
		result.Args = parseArgs(rest)
	case "END":
		result.Type = InstrEnd
		result.Args = parseArgs(rest)
	default:
		result.Type = InstrLog
		result.Args = []InstructionArg{
			{Type: ArgLiteral, Value: line},
		}
	}

	return *result
}

func parseArgs(input string) []InstructionArg {
	var args []InstructionArg
	remaining := strings.TrimSpace(input)

	for len(remaining) > 0 {
		arg, rest := parseOneArg(remaining)
		if arg != nil {
			args = append(args, *arg)
		}
		remaining = rest
	}

	return args
}

func parseVarExpr(input string) []InstructionArg {
	return parseArgs(input)
}

func parseRunArgs(input string) []InstructionArg {
	var args []InstructionArg
	remaining := strings.TrimSpace(input)

	for len(remaining) > 0 {
		arg, rest := parseOneArg(remaining)
		if arg != nil {
			args = append(args, *arg)
		}
		remaining = rest
	}

	return args
}

func parseCatchArgs(input string) []InstructionArg {
	return parseArgs(input)
}

func parseCallArgs(input string) []InstructionArg {
	return parseArgs(input)
}

func parseIfArgs(input string) []InstructionArg {
	var args []InstructionArg
	remaining := strings.TrimSpace(input)

	for len(remaining) > 0 {
		if remaining[0] == '\'' {
			arg, rest := parseOneArg(remaining)
			if arg != nil {
				args = append(args, *arg)
			}
			remaining = rest
			continue
		}
		if remaining[0] == '*' {
			arg, rest := parseOneArg(remaining)
			if arg != nil {
				args = append(args, *arg)
			}
			remaining = rest
			continue
		}
		if remaining[0] == '?' || remaining[0] == ':' {
			remaining = strings.TrimSpace(remaining[1:])
			continue
		}
		i := 0
		for i < len(remaining) && remaining[i] != ' ' && remaining[i] != '\t' && remaining[i] != '?' && remaining[i] != ':' {
			i++
		}
		remaining = strings.TrimSpace(remaining[i:])
	}

	return args
}

func parseCycleArgs(input string) []InstructionArg {
	return parseArgs(input)
}

func parseOneArg(input string) (*InstructionArg, string) {
	input = strings.TrimSpace(input)
	if len(input) == 0 {
		return nil, ""
	}

	if input[0] == '\'' {
		end := 1
		for end < len(input) {
			if input[end] == '\\' && end+1 < len(input) {
				end += 2
				continue
			}
			if input[end] == '\'' {
				end++
				break
			}
			end++
		}
		value := input[1 : end-1]
		rest := strings.TrimSpace(input[end:])
		return &InstructionArg{Type: ArgLiteral, Value: value}, rest
	}

	if input[0] == '"' {
		end := 1
		for end < len(input) {
			if input[end] == '\\' && end+1 < len(input) {
				end += 2
				continue
			}
			if input[end] == '"' {
				end++
				break
			}
			end++
		}
		value := input[1 : end-1]
		rest := strings.TrimSpace(input[end:])
		return &InstructionArg{Type: ArgLiteral, Value: value}, rest
	}

	if input[0] == '#' {
		end := 1
		for end < len(input) && !isSpace(input[end]) {
			end++
		}
		name := input[1:end]
		rest := strings.TrimSpace(input[end:])
		return &InstructionArg{Type: ArgVariable, Value: name}, rest
	}

	if input[0] == '*' {
		end := 1
		for end < len(input) && !isSpace(input[end]) {
			end++
		}
		name := input[1:end]
		rest := strings.TrimSpace(input[end:])
		return &InstructionArg{Type: ArgPointer, Value: name}, rest
	}

	end := 0
	for end < len(input) && !isSpace(input[end]) {
		end++
	}
	value := input[:end]
	rest := strings.TrimSpace(input[end:])
	return &InstructionArg{Type: ArgLiteral, Value: value}, rest
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t'
}