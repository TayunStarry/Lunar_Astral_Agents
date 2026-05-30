package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type BlockManager struct {
	mu        sync.RWMutex
	blocks    map[string]*CodeBlock
	readyList []string
	waitList  []string
	nextID    int
	pending   []*CodeBlock
}

func NewBlockManager() *BlockManager {
	return &BlockManager{
		blocks:    make(map[string]*CodeBlock),
		readyList: make([]string, 0),
		waitList:  make([]string, 0),
		pending:   make([]*CodeBlock, 0),
		nextID:    1,
	}
}

func (bm *BlockManager) CreateBlock(lines []string, origin string) *CodeBlock {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	id := fmt.Sprintf("block_%d", bm.nextID)
	bm.nextID++

	block := &CodeBlock{
		ID:        id,
		PC:        0,
		Status:    StatusReady,
		Origin:    origin,
		StartTime: time.Now(),
	}

	block.Instructions = ParseLines(lines)
	bm.blocks[id] = block

	return block
}

func (bm *BlockManager) AddReady(blockID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if block, exists := bm.blocks[blockID]; exists {
		block.Status = StatusReady
		bm.readyList = append(bm.readyList, blockID)
	}
}

func (bm *BlockManager) AddPending(block *CodeBlock) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.blocks[block.ID] = block
	bm.pending = append(bm.pending, block)
}

func (bm *BlockManager) FlushPending() []*CodeBlock {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	result := bm.pending
	bm.pending = make([]*CodeBlock, 0)

	for _, b := range result {
		bm.readyList = append(bm.readyList, b.ID)
	}

	return result
}

func (bm *BlockManager) MoveToWaiting(blockID string, cond WaitCondition) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if block, exists := bm.blocks[blockID]; exists {
		block.Status = StatusWaiting
		block.WaitCond = cond
		bm.waitList = append(bm.waitList, blockID)

		bm.readyList = removeFromSlice(bm.readyList, blockID)
	}
}

func (bm *BlockManager) MoveToReady(blockID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if block, exists := bm.blocks[blockID]; exists {
		block.Status = StatusReady
		block.WaitCond = WaitCondition{}
		bm.readyList = append(bm.readyList, blockID)

		bm.waitList = removeFromSlice(bm.waitList, blockID)
	}
}

func (bm *BlockManager) Terminate(blockID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if block, exists := bm.blocks[blockID]; exists {
		block.Status = StatusTerminated
		bm.readyList = removeFromSlice(bm.readyList, blockID)
		bm.waitList = removeFromSlice(bm.waitList, blockID)
	}
}

func (bm *BlockManager) TerminateAll() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, block := range bm.blocks {
		block.Status = StatusTerminated
	}
	bm.readyList = nil
	bm.waitList = nil
}

func (bm *BlockManager) GetBlock(blockID string) *CodeBlock {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.blocks[blockID]
}

func (bm *BlockManager) GetReadyList() []string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make([]string, len(bm.readyList))
	copy(result, bm.readyList)
	return result
}

func (bm *BlockManager) GetWaitList() []string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make([]string, len(bm.waitList))
	copy(result, bm.waitList)
	return result
}

func (bm *BlockManager) ReadyCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.readyList)
}

func (bm *BlockManager) WaitCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.waitList)
}

func (bm *BlockManager) ActiveCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.readyList) + len(bm.waitList) + len(bm.pending)
}

func (bm *BlockManager) CleanupTerminated() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	count := 0
	for id, block := range bm.blocks {
		if block.Status == StatusTerminated {
			delete(bm.blocks, id)
			count++
		}
	}
	return count
}

func (bm *BlockManager) AdvancePC(blockID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if block, exists := bm.blocks[blockID]; exists {
		block.PC++
	}
}

func (bm *BlockManager) HasMoreInstructions(blockID string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	block, exists := bm.blocks[blockID]
	return exists && block.PC < len(block.Instructions)
}

func (bm *BlockManager) Stats() TickStats {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return TickStats{
		ReadyBlocks:   len(bm.readyList),
		WaitingBlocks: len(bm.waitList),
	}
}

func ParseLines(lines []string) []Instruction {
	var instructions []Instruction

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		instr := ParseInstruction(line)
		instructions = append(instructions, instr)
	}

	return instructions
}

func removeFromSlice(slice []string, item string) []string {
	for i, v := range slice {
		if v == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
