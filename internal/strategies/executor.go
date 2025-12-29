package strategies

import (
	"log"
	"sync"

	"gorm.io/gorm"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/model"
)

// Executor 是策略引擎的核心调度器
// 它管理所有正在运行的策略实例，并负责将行情分发给它们
type Executor struct {
	db *gorm.DB

	// 运行中的策略集合
	// Map结构: Symbol -> []StrategyRunner
	// 这样设计是为了快速索引：当 rb2601 行情来时，只遍历关注 rb2601 的策略
	runners map[string][]StrategyRunner

	// 锁，用于保护 runners map (防止并发读写)
	mu sync.RWMutex
}

// NewExecutor 创建一个新的调度器
func NewExecutor(db *gorm.DB) *Executor {
	return &Executor{
		db:      db,
		runners: make(map[string][]StrategyRunner),
	}
}

// LoadActiveStrategies 从数据库加载所有状态为 "active" 的策略到内存
// 通常在服务启动时调用
func (e *Executor) LoadActiveStrategies() {
	var strategies []model.Strategy
	// 查询 db: SELECT * FROM strategies WHERE status = 'active'
	if err := e.db.Where("status = ?", model.StrategyStatusActive).Find(&strategies).Error; err != nil {
		log.Printf("Error loading strategies: %v", err)
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 清空旧的，重新加载
	e.runners = make(map[string][]StrategyRunner)
	count := 0

	for _, s := range strategies {
		var runner StrategyRunner
		var err error

		// 工厂模式：根据策略类型创建对应的 Runner
		switch s.Type {
		case model.StrategyTypeConditionOrder:
			runner, err = NewConditionOrderRunner(s)
		// case model.StrategyTypeGridTrading:
		// runner, err = NewGridTradingRunner(s)
		default:
			log.Printf("Unknown strategy type: %s", s.Type)
			continue
		}

		if err != nil {
			log.Printf("Failed to init strategy %d: %v", s.ID, err)
			continue
		}

		// 将 Runner 注册到对应的 Symbol 列表下
		if e.runners[s.InstrumentID] == nil {
			e.runners[s.InstrumentID] = make([]StrategyRunner, 0)
		}
		e.runners[s.InstrumentID] = append(e.runners[s.InstrumentID], runner)
		count++
	}

	log.Printf("Loaded %d active strategies into memory", count)
}

// OnMarketData 当收到行情数据时被 Engine 调用
func (e *Executor) OnMarketData(symbol string, price float64) []*infra.Command {
	e.mu.RLock()
	runners, ok := e.runners[symbol]
	e.mu.RUnlock()

	if !ok || len(runners) == 0 {
		return nil
	}

	var commands []*infra.Command

	// 遍历所有关注该 Symbol 的策略
	// 并发安全注意：如果 Runner 内部状态复杂，这里可能需要加锁或单独通过 channel 通信
	for _, runner := range runners {
		cmd := runner.OnTick(price)
		if cmd != nil {
			commands = append(commands, cmd)
		}
	}

	return commands
}

// Reload 当用户新增与停止策略时，可以调用此方法热更新内存
// 简单起见，这里重新从数据库加载一次。
// 优化方案：可以增加 AddStrategy / RemoveStrategy 方法做增量更新。
func (e *Executor) Reload() {
	log.Println("Reloading strategies...")
	e.LoadActiveStrategies()
}

// GetSymbols returns all symbols currently monitored by strategies.
func (e *Executor) GetSymbols() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	symbols := make([]string, 0, len(e.runners))
	for sym := range e.runners {
		symbols = append(symbols, sym)
	}
	return symbols
}
