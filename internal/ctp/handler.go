package ctp

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// CTPHandler processes incoming CTP responses using the database and notifier.
type CTPHandler struct {
	db       *gorm.DB
	notifier domain.Notifier
}

// NewCTPHandler creates a new CTP Response Handler.
func NewCTPHandler(db *gorm.DB, notifier domain.Notifier) *CTPHandler {
	return &CTPHandler{
		db:       db,
		notifier: notifier,
	}
}

// ProcessResponse dispatches the response based on its type.
func (h *CTPHandler) ProcessResponse(resp TradeResponse) {
	log.Printf("CTP Handler: Processing %s, ReqID=%s", resp.Type, resp.RequestID)

	payload, ok := resp.Payload.(map[string]interface{})
	if !ok {
		// Some responses like QRY_POS_RSP might have nested structures that decode differently
		// if we aren't careful, but based on current engine logic, Payload is usually a map.
		// However, for QRY_POS_RSP/QRY_INSTRUMENT_RSP, if they come as raw json in Payload,
		// we might need to be careful. The original code assumed Payload is map[string]interface{}.
		// Let's stick to the original logic which checks type assertions.
		log.Printf("CTP Handler: Invalid payload format for %s", resp.Type)
		return
	}

	switch resp.Type {
	case "RTN_ORDER":
		h.handleRtnOrder(resp, payload)
	case "RTN_TRADE":
		h.handleRtnTrade(resp, payload)
	case "ERR_ORDER":
		h.handleErrOrder(resp, payload)
	case "POSITION_SNAPSHOT":
		// 完整持仓快照 (启动时或手动查询)
		h.handlePositionSnapshot(resp, payload)
	case "QRY_POS_RSP":
		// 查询持仓 (legacy)
		h.handleQryPosRsp(payload)
	case "QRY_INSTRUMENT_RSP":
		// 查询合约信息
		h.handleQryInstrumentRsp(payload)
	case "QRY_MARGIN_RATE_RSP":
		// 查询合约保证金率
		h.handleQryMarginRateRsp(resp.RequestID, payload)
	case "QRY_COMMISSION_RATE_RSP":
		// 查询合约手续费率
		h.handleQryCommissionRateRsp(resp.RequestID, payload)
	case "QRY_ACCOUNT_RSP":
		// TODO: Implement Account Update Logic
		log.Printf("Received Account Update: %v", payload)
	}
}

func (h *CTPHandler) handleRtnOrder(resp TradeResponse, payload map[string]interface{}) {
	statusStr, _ := payload["OrderStatus"].(string)
	orderSysID, _ := payload["OrderSysID"].(string)
	errorMsg, _ := payload["StatusMsg"].(string)

	var order model.Order
	if err := h.db.Where("order_ref = ?", resp.RequestID).First(&order).Error; err == nil {
		// Record Log
		h.db.Create(&model.OrderLog{
			OrderID:   order.ID,
			OldStatus: string(order.OrderStatus),
			NewStatus: statusStr,
			Message:   errorMsg,
			CreatedAt: time.Now(),
		})

		updates := map[string]interface{}{}
		if statusStr != "" {
			updates["OrderStatus"] = statusStr
		}
		if orderSysID != "" {
			updates["OrderSysID"] = orderSysID
		}
		if errorMsg != "" {
			updates["StatusMsg"] = errorMsg
		}

		if len(updates) > 0 {
			h.db.Model(&order).Updates(updates)
			h.notifyUser(order.InvestorID, resp)
		}
	}
}

func (h *CTPHandler) handleRtnTrade(resp TradeResponse, payload map[string]interface{}) {
	var order model.Order
	if h.db.Where("order_ref = ?", resp.RequestID).First(&order).Error == nil {
		tradeVol, _ := payload["Volume"].(float64)
		price, _ := payload["Price"].(float64)
		tradeID, _ := payload["TradeID"].(string)

		// 1. Insert Trade Record
		h.db.Create(&model.Trade{
			OrderID:      order.ID,
			OrderRef:     order.OrderRef,
			OrderSysID:   order.OrderSysID,
			TradeID:      tradeID,
			InstrumentID: order.InstrumentID,
			Direction:    string(order.Direction),
			OffsetFlag:   string(order.CombOffsetFlag),
			Price:        price,
			Volume:       int(tradeVol),
			TradeTime:    time.Now().Format("15:04:05"),
			TradingDay:   time.Now().Format("20060102"), // Should ideally come from CTP
			StrategyID:   order.StrategyID,
		})

		// 2. Partial Fill Logic
		newFilledVol := order.VolumeTraded + int(tradeVol)
		updates := map[string]interface{}{
			"VolumeTraded": newFilledVol,
		}

		if newFilledVol >= order.VolumeTotalOriginal {
			updates["OrderStatus"] = model.OrderStatusAllTraded
		} else {
			updates["OrderStatus"] = model.OrderStatusPartTradedQueueing
		}

		h.db.Model(&order).Updates(updates)

		// 3. Update Position 数据库
		h.updatePosition(order, payload)

		// 4. Notify user about trade
		h.notifyUser(order.InvestorID, resp)

		// 5. 重要：成交后立即向前端推送最新的完整持仓快照
		go func() {
			var allPos []model.Position
			if err := h.db.Where("investor_id = ?", order.InvestorID).Find(&allPos).Error; err == nil {
				wsPayload := map[string]interface{}{
					"Type": "POSITION_SNAPSHOT",
					"Payload": map[string]interface{}{
						"InvestorID": order.InvestorID,
						"Positions":  allPos,
						"Total":      len(allPos),
						"Timestamp":  time.Now().Format(time.RFC3339),
					},
				}
				h.notifier.BroadcastToAll(wsPayload)
			}
		}()
	}
}

func (h *CTPHandler) handleErrOrder(resp TradeResponse, payload map[string]interface{}) {
	errorMsg, _ := payload["ErrorMsg"].(string)

	var order model.Order
	if h.db.Where("order_ref = ?", resp.RequestID).First(&order).Error == nil {
		h.db.Create(&model.OrderLog{
			OrderID:   order.ID,
			OldStatus: string(order.OrderStatus),
			NewStatus: string(model.OrderStatusNoTradeNotQueueing), // Rejected
			Message:   errorMsg,
			CreatedAt: time.Now(),
		})

		h.db.Model(&order).Updates(map[string]interface{}{
			"OrderStatus": model.OrderStatusNoTradeNotQueueing,
			"StatusMsg":   errorMsg,
		})
		h.notifyUser(order.InvestorID, resp)
	}
}

// handlePositionSnapshot 处理完整持仓快照（从 CTP 批量返回）
func (h *CTPHandler) handlePositionSnapshot(resp TradeResponse, payload map[string]interface{}) {
	investorID, _ := payload["UserID"].(string) // CTP 核心传过来的是 UserID 键名，但我们要存为 InvestorID
	if investorID == "" {
		investorID = payload["InvestorID"].(string)
	}

	positionsRaw, ok := payload["Positions"].([]interface{})
	if !ok {
		log.Printf("POSITION_SNAPSHOT: Invalid Positions format")
		return
	}

	log.Printf("POSITION_SNAPSHOT: Received %d positions for investor %s", len(positionsRaw), investorID)

	// 开启事务处理持仓更新
	tx := h.db.Begin()

	// 先清除该投资者的旧持仓
	if err := tx.Where("investor_id = ?", investorID).Delete(&model.Position{}).Error; err != nil {
		tx.Rollback()
		log.Printf("POSITION_SNAPSHOT: Failed to clear old positions: %v", err)
		return
	}

	var savedPositions []model.Position
	for _, p := range positionsRaw {
		pBytes, _ := json.Marshal(p)
		var pos model.Position
		if err := json.Unmarshal(pBytes, &pos); err != nil {
			log.Printf("POSITION_SNAPSHOT: Failed to unmarshal position: %v", err)
			continue
		}

		// 设置投资者ID和更新时间
		pos.InvestorID = investorID
		pos.UpdatedAt = time.Now()

		// 计算均价 (如果持仓成本和数量都有效)
		if pos.Position > 0 && pos.PositionCost > 0 {
			// 从 futures 表获取合约乘数来计算均价
			var future model.Future
			if h.db.Where("instrument_id = ?", pos.InstrumentID).First(&future).Error == nil {
				if future.VolumeMultiple > 0 {
					pos.AveragePrice = pos.PositionCost / float64(pos.Position) / float64(future.VolumeMultiple)
				}
			}
		}

		if err := tx.Create(&pos).Error; err != nil {
			log.Printf("POSITION_SNAPSHOT: Failed to save position %s: %v", pos.InstrumentID, err)
			continue
		}
		savedPositions = append(savedPositions, pos)
	}

	tx.Commit()
	log.Printf("POSITION_SNAPSHOT: Saved %d positions to database", len(savedPositions))

	// 通过 WebSocket 广播持仓快照给所有客户端
	if h.notifier != nil {
		wsPayload := map[string]interface{}{
			"Type": "POSITION_SNAPSHOT",
			"Payload": map[string]interface{}{
				"InvestorID": investorID,
				"Positions":  savedPositions,
				"Total":      len(savedPositions),
				"Timestamp":  time.Now().Format(time.RFC3339),
			},
		}
		h.notifier.BroadcastToAll(wsPayload)
		log.Printf("POSITION_SNAPSHOT: Broadcasted to WebSocket clients")
	}
}

func (h *CTPHandler) handleQryPosRsp(payload map[string]interface{}) {
	if positions, ok := payload["Positions"].([]interface{}); ok {
		for _, p := range positions {
			pBytes, _ := json.Marshal(p)
			var pos model.Position
			if err := json.Unmarshal(pBytes, &pos); err == nil {
				h.db.Save(&pos)
			}
		}
		log.Printf("Synchronized %d positions", len(positions))
	}
}

func (h *CTPHandler) handleQryInstrumentRsp(payload map[string]interface{}) {
	if instruments, ok := payload["Instruments"].([]interface{}); ok {
		for _, inst := range instruments {
			instBytes, _ := json.Marshal(inst)
			var instrument model.Future
			if err := json.Unmarshal(instBytes, &instrument); err == nil {
				h.db.Save(&instrument)
			}
		}
		log.Printf("Synchronized %d instruments", len(instruments))
	}
}

func (h *CTPHandler) handleQryMarginRateRsp(reqID string, payload map[string]interface{}) {
	instID, _ := payload["InstrumentID"].(string)

	// 从 RequestID 中解析出原始查询的合约 (格式: query-margin-rb2605-2026...)
	var originalInstID string
	if reqID != "" {
		// 分割: query, margin, instrument, timestamp
		parts := strings.Split(reqID, "-")
		if len(parts) >= 3 {
			originalInstID = parts[2]
		}
	}

	updates := map[string]interface{}{}
	if val, ok := payload["LongMarginRatioByMoney"].(float64); ok {
		updates["LongMarginRatioByMoney"] = val
	}
	if val, ok := payload["LongMarginRatioByVolume"].(float64); ok {
		updates["LongMarginRatioByVolume"] = val
	}
	if val, ok := payload["ShortMarginRatioByMoney"].(float64); ok {
		updates["ShortMarginRatioByMoney"] = val
	}
	if val, ok := payload["ShortMarginRatioByVolume"].(float64); ok {
		updates["ShortMarginRatioByVolume"] = val
	}

	if len(updates) > 0 {
		// 策略:
		// 1. 如果返回的 instID 是完整的 (与 originalInfo 一致)，直接更新
		// 2. 如果返回的是产品代码 (如 "rb")，则更新 originalInstID，或者更新该产品下所有合约
		if instID != "" && instID == originalInstID {
			h.db.Model(&model.Future{}).Where("instrument_id = ?", instID).Updates(updates)
		} else if originalInstID != "" {
			// 兜底更新我们请求的那个
			h.db.Model(&model.Future{}).Where("instrument_id = ?", originalInstID).Updates(updates)
		}

		// 如果 instID 是产品代码，额外尝试更新该品种下所有合约 (可选)
		if len(instID) <= 2 && instID != "" {
			h.db.Model(&model.Future{}).Where("product_id = ?", instID).Updates(updates)
		}

		log.Printf("Updated Margin Rate for %s (Payload: %s, Req: %s)", originalInstID, instID, reqID)
	}
}

func (h *CTPHandler) handleQryCommissionRateRsp(reqID string, payload map[string]interface{}) {
	instID, _ := payload["InstrumentID"].(string)

	var originalInstID string
	if reqID != "" {
		parts := strings.Split(reqID, "-")
		if len(parts) >= 3 {
			originalInstID = parts[2]
		}
	}

	updates := map[string]interface{}{}
	if val, ok := payload["OpenRatioByMoney"].(float64); ok {
		updates["OpenRatioByMoney"] = val
	}
	if val, ok := payload["OpenRatioByVolume"].(float64); ok {
		updates["OpenRatioByVolume"] = val
	}
	if val, ok := payload["CloseRatioByMoney"].(float64); ok {
		updates["CloseRatioByMoney"] = val
	}
	if val, ok := payload["CloseRatioByVolume"].(float64); ok {
		updates["CloseRatioByVolume"] = val
	}
	if val, ok := payload["CloseTodayRatioByMoney"].(float64); ok {
		updates["CloseTodayRatioByMoney"] = val
	}
	if val, ok := payload["CloseTodayRatioByVolume"].(float64); ok {
		updates["CloseTodayRatioByVolume"] = val
	}

	if len(updates) > 0 {
		if instID != "" && instID == originalInstID {
			h.db.Model(&model.Future{}).Where("instrument_id = ?", instID).Updates(updates)
		} else if originalInstID != "" {
			h.db.Model(&model.Future{}).Where("instrument_id = ?", originalInstID).Updates(updates)
		}

		if len(instID) <= 2 && instID != "" {
			h.db.Model(&model.Future{}).Where("product_id = ?", instID).Updates(updates)
		}

		log.Printf("Updated Commission Rate for %s (Payload: %s, Req: %s)", originalInstID, instID, reqID)
	}
}

func (h *CTPHandler) updatePosition(order model.Order, tradePayload map[string]interface{}) {
	offset, _ := tradePayload["OffsetFlag"].(string)
	tradeVol, _ := tradePayload["Volume"].(float64)

	var pos model.Position
	if offset == string(model.OffsetOpen) {
		// 开仓逻辑
		if err := h.db.Where("investor_id = ? AND instrument_id = ? AND posi_direction = ?",
			order.InvestorID, order.InstrumentID, string(order.Direction)).First(&pos).Error; err == nil {
			pos.Position += int(tradeVol)
			pos.TodayPosition += int(tradeVol)
			pos.UpdatedAt = time.Now()
			h.db.Save(&pos)
		} else {
			// 没找到持仓，新建
			pos = model.Position{
				InvestorID:    order.InvestorID,
				InstrumentID:  order.InstrumentID,
				ExchangeID:    order.ExchangeID,
				PosiDirection: string(order.Direction),
				HedgeFlag:     "1",
				TodayPosition: int(tradeVol),
				Position:      int(tradeVol),
				UpdatedAt:     time.Now(),
			}
			h.db.Create(&pos)
		}
	} else {
		// 平仓逻辑
		if err := h.db.Where("investor_id = ? AND instrument_id = ?",
			order.InvestorID, order.InstrumentID).First(&pos).Error; err == nil {
			pos.Position -= int(tradeVol)
			if pos.Position < 0 {
				pos.Position = 0
			}
			if order.CombOffsetFlag == model.OffsetCloseToday {
				pos.TodayPosition -= int(tradeVol)
			} else {
				pos.YdPosition -= int(tradeVol)
			}
			if pos.TodayPosition < 0 {
				pos.TodayPosition = 0
			}
			if pos.YdPosition < 0 {
				pos.YdPosition = 0
			}
		}
		pos.UpdatedAt = time.Now()
		h.db.Save(&pos)
	}
}

// notifyUser 发送通知给用户
func (h *CTPHandler) notifyUser(investorID string, data interface{}) {
	if h.notifier != nil {
		_ = investorID
		h.notifier.BroadcastToAll(data)
	}
}
