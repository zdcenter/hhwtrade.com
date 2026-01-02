package ctp

import (
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// Handler processes incoming CTP responses using the database and notifier.
type Handler struct {
	db       *gorm.DB
	notifier domain.Notifier
}

// NewHandler creates a new CTP Response Handler.
func NewHandler(db *gorm.DB, notifier domain.Notifier) *Handler {
	return &Handler{
		db:       db,
		notifier: notifier,
	}
}

// ProcessResponse dispatches the response based on its type.
func (h *Handler) ProcessResponse(resp TradeResponse) {
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
	case "QRY_POS_RSP":
		h.handleQryPosRsp(payload)
	case "QRY_INSTRUMENT_RSP":
		h.handleQryInstrumentRsp(payload)
	case "QRY_ACCOUNT_RSP":
		// TODO: Implement Account Update Logic
		log.Printf("Received Account Update: %v", payload)
	}
}

func (h *Handler) handleRtnOrder(resp TradeResponse, payload map[string]interface{}) {
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
			h.notifyUser(order.UserID, resp)
		}
	}
}

func (h *Handler) handleRtnTrade(resp TradeResponse, payload map[string]interface{}) {
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

		// 3. Update Position
		h.updatePosition(order, payload)

		// 4. Notify user
		h.notifyUser(order.UserID, resp)
	}
}

func (h *Handler) handleErrOrder(resp TradeResponse, payload map[string]interface{}) {
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
		h.notifyUser(order.UserID, resp)
	}
}

func (h *Handler) handleQryPosRsp(payload map[string]interface{}) {
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

func (h *Handler) handleQryInstrumentRsp(payload map[string]interface{}) {
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

func (h *Handler) updatePosition(order model.Order, tradePayload map[string]interface{}) {
	// Determine PosiDirection: '2' Long, '3' Short
	posiDir := "2" // Default to Long
	if order.Direction == model.DirectionBuy {
		if order.CombOffsetFlag != model.OffsetOpen {
			posiDir = "3" // Buy Close -> belongs to Short side
		}
	} else {
		if order.CombOffsetFlag == model.OffsetOpen {
			posiDir = "3" // Sell Open -> belongs to Short side
		}
	}

	var pos model.Position
	err := h.db.Where("user_id = ? AND instrument_id = ? AND posi_direction = ?", order.UserID, order.InstrumentID, posiDir).First(&pos).Error

	tradeVol, _ := tradePayload["Volume"].(float64)
	tradePrice, _ := tradePayload["Price"].(float64)

	if err != nil {
		// New position
		if order.CombOffsetFlag == model.OffsetOpen {
			pos = model.Position{
				UserID:        order.UserID,
				InstrumentID:  order.InstrumentID,
				PosiDirection: posiDir,
				Position:      int(tradeVol),
				TodayPosition: int(tradeVol),
				AveragePrice:  tradePrice,
				PositionCost:  tradePrice * tradeVol,
				UpdatedAt:    time.Now(),
			}
			h.db.Create(&pos)
		}
	} else {
		// Existing position
		if order.CombOffsetFlag == model.OffsetOpen {
			newTotal := pos.Position + int(tradeVol)
			pos.PositionCost += tradePrice * tradeVol
			if newTotal > 0 {
				pos.AveragePrice = pos.PositionCost / float64(newTotal)	
			}
			pos.Position = newTotal
			pos.TodayPosition += int(tradeVol)
		} else {
			pos.Position -= int(tradeVol)
			if pos.Position < 0 {
				pos.Position = 0
			}
			if order.CombOffsetFlag == model.OffsetCloseToday {
				pos.TodayPosition -= int(tradeVol)
			} else {
				pos.YdPosition -= int(tradeVol)
			}
			if pos.TodayPosition < 0 { pos.TodayPosition = 0 }
			if pos.YdPosition < 0 { pos.YdPosition = 0 }
		}
		pos.UpdatedAt = time.Now()
		h.db.Save(&pos)
	}
}

func (h *Handler) notifyUser(userID string, data interface{}) {
	if h.notifier != nil {
		_ = userID
		h.notifier.BroadcastToAll(data)
	}
}
