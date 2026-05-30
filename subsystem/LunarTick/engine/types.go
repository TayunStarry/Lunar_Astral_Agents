package engine

import (
	"sync"
	"time"
)

type InstructionType string

const (
	InstrSET    InstructionType = "SET"
	InstrADD    InstructionType = "ADD"
	InstrWRT    InstructionType = "WRT"
	InstrRON    InstructionType = "RON"
	InstrUNL    InstructionType = "UNL"
	InstrRun    InstructionType = "RUN"
	InstrCatch  InstructionType = "CATCH"
	InstrCall   InstructionType = "CALL"
	InstrWait   InstructionType = "WAIT"
	InstrSleep  InstructionType = "SLEEP"
	InstrFilter InstructionType = "FILTER"
	InstrMath   InstructionType = "MATH"
	InstrIf     InstructionType = "IF"
	InstrCycle  InstructionType = "CYCLE"
	InstrRetry  InstructionType = "RETRY"
	InstrWrite  InstructionType = "WRITE"
	InstrRead   InstructionType = "READ"
	InstrLog    InstructionType = "LOG"
	InstrWeb    InstructionType = "WEB"
	InstrStop   InstructionType = "STOP"
	InstrLimit  InstructionType = "LIMIT"
	InstrDef    InstructionType = "DEF"
	InstrLazy   InstructionType = "LAZY"
	InstrBuild  InstructionType = "BUILD"
	InstrEnd    InstructionType = "END"
	InstrPtr    InstructionType = "PTR"
)

type BlockStatus int

const (
	StatusReady BlockStatus = iota
	StatusWaiting
	StatusTerminated
)

func (s BlockStatus) String() string {
	switch s {
	case StatusReady:
		return "Ready"
	case StatusWaiting:
		return "Waiting"
	case StatusTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

type VarMode int

const (
	ModeNormal VarMode = iota
	ModeReadOnly
	ModeWriteOnly
)

type PointerType int

const (
	PointerClass PointerType = iota
	PointerSnapshot
)

type ArgType int

const (
	ArgLiteral ArgType = iota
	ArgVariable
	ArgPointer
)

type InstructionArg struct {
	Type  ArgType
	Value string
}

type Instruction struct {
	Type InstructionType
	Args []InstructionArg
	Line string
	Raw  string
}

type BlockID = string

type CodeBlock struct {
	ID            string
	Instructions  []Instruction
	PC            int
	Status        BlockStatus
	WaitCond      WaitCondition
	Origin        string
	RetryCool     time.Duration
	RetryMax      int
	RetryCount    int
	RetryErrVar   string
	LimitMS       time.Duration
	StartTime     time.Time
	CatchVar      string
	CatchSubstr   string
	CallVar       string
	CallPtr       string
	CallRunning   bool
	ProcessHandle interface{}
	IsLazy        bool
	mu            sync.Mutex
}

type WaitCondition struct {
	Type  WaitType
	Var   string
	Until time.Time
	Done  <-chan struct{}
}

type WaitType int

const (
	WaitNone WaitType = iota
	WaitVariable
	WaitSleep
	WaitProcess
	WaitCatch
)

type ErrorInfo struct {
	BlockID    string
	Message    string
	TickNumber int
}

type TickStats struct {
	TickNumber      int
	ReadyBlocks     int
	WaitingBlocks   int
	TerminatedCount int
	ProcessLatency  time.Duration
	IsSuspended     bool
}
