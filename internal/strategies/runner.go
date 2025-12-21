package strategies

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/model"
)

// StrategyRunner 定义每个策略实例必须实现的接口
// 不管是条件单、网格交易还是 CTA 策略，都必须实现这些方法
type StrategyRunner interface {
	// OnTick 当收到新的行情数据时被调用
	// 返回值: 如果需要下单，返回 Command；否则返回 nil
	OnTick(price float64) *infra.Command
}

// =======================
// 条件单策略实现
// =======================

// ConditionOrderRunner 是条件单的具体执行逻辑
type ConditionOrderRunner struct {
	strategyID uint                       // 策略 ID (数据库主键)
	symbol     string                     // 合约代码
	cfg        model.ConditionOrderConfig // 解析后的配置参数
	triggered  bool                       // 运行时状态：是否已经触发过
}

// NewConditionOrderRunner 创建一个新的条件单运行实例
func NewConditionOrderRunner(strategy model.Strategy) (*ConditionOrderRunner, error) {
	var cfg model.ConditionOrderConfig
	// 将数据库里存的 JSON 配置解析成具体的结构体
	if err := json.Unmarshal(strategy.Config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse condition order config: %v", err)
	}

	return &ConditionOrderRunner{
		strategyID: strategy.ID,
		symbol:     strategy.Symbol,
		cfg:        cfg,
		triggered:  false, // 初始状态未触发
	}, nil
}

// OnTick 是策略的核心大脑
func (r *ConditionOrderRunner) OnTick(price float64) *infra.Command {
	// 1. 如果已经触发过了，就不要再触发了（防止重复下单）
	if r.triggered {
		return nil
	}

	// 2. 判断条件是否满足
	match := false
	switch r.cfg.Operator {
	case ">":
		if price > r.cfg.TriggerPrice {
			match = true
		}
	case ">=":
		if price >= r.cfg.TriggerPrice {
			match = true
		}
	case "<":
		if price < r.cfg.TriggerPrice {
			match = true
		}
	case "<=":
		if price <= r.cfg.TriggerPrice {
			match = true
		}
	}

	// 3. 如果条件满足，执行下单逻辑
	if match {
		log.Printf("[Strategy %d] API 触发! 当前价: %.2f %s 触发价: %.2f",
			r.strategyID, price, r.cfg.Operator, r.cfg.TriggerPrice)

		r.triggered = true // 标记为已触发

		// 映射策略 Action 到 CTP 指令字符
		// CTP 规范: Direction ('0':买, '1':卖), Offset ('0':开, '1':平, '3':平今)
		direction := "0"
		offset := "0"

		switch r.cfg.Action {
		case "open_long":
			direction = "0"
			offset = "0"
		case "close_long":
			direction = "1"
			offset = "1"
		case "open_short":
			direction = "1"
			offset = "0"
		case "close_short":
			direction = "0"
			offset = "1"
		}

		// 组装下单指令，严格遵循统一协议
		return &infra.Command{
			Type: "INSERT_ORDER",
			Payload: map[string]interface{}{
				"symbol":      r.symbol,
				"direction":   direction,
				"offset":      offset,
				"volume":      r.cfg.Volume,
				"price":       price,
				"order_ref":   fmt.Sprintf("strat-%d", r.strategyID),
				"strategy_id": r.strategyID,
			},
			RequestID: fmt.Sprintf("strat-%d-%v", r.strategyID, timeNowUnix()),
		}
	}

	return nil
}

func timeNowUnix() int64 {
	return time.Now().Unix()
}
