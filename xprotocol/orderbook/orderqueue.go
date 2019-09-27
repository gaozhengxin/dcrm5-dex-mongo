package orderbook

import (
	"github.com/shopspring/decimal"
	"container/list"
	"github.com/fusion/go-fusion/log"
	"strings"
	"fmt"
)

type XvcOrderQueue struct {
	//Volume decimal.Decimal
	//Price  decimal.Decimal
	Volume string
	Price  string
	//Orders []*XvcOrder
	Orders *list.List
}

func NewXvcOrderQueue(price string) *XvcOrderQueue {
	return &XvcOrderQueue{
		Price:  price,
		Volume: "0",
		Orders: list.New(),
	}
}

//func (oq *XvcOrderQueue) Len() int {
//	return len(oq.Orders)
//}

// Len returns amount of orders in queue
func (oq *XvcOrderQueue) Len() int {
	return oq.Orders.Len()
}

func (oq *XvcOrderQueue) PRICE() string {
	return oq.Price
}

func (oq *XvcOrderQueue) VOLUME() string {
	return oq.Volume
}

// Head returns top order in queue
func (oq *XvcOrderQueue) Head() *list.Element {
	return oq.Orders.Front()
}

// Tail returns bottom order in queue
func (oq *XvcOrderQueue) Tail() *list.Element {
	return oq.Orders.Back()
}

// Append adds order to tail of the queue
func (oq *XvcOrderQueue) Append(o *XvcOrder) *list.Element {
        vol,_ := decimal.NewFromString(oq.Volume)
	quan,_ := decimal.NewFromString(o.QUANTITY())
	//oq.Volume = oq.Volume.Add(o.QUANTITY())
	vol = vol.Add(quan)
	oq.Volume = vol.StringFixed(10)
	return oq.Orders.PushBack(o)
}

// Update sets up new order to list value
/*func (oq *XvcOrderQueue) Update(e *list.Element, o *XvcOrder) *list.Element {
        order := e.Value.(*XvcOrder)
	if order == nil {
	    return nil
	}

	vol,_ := decimal.NewFromString(oq.Volume)
	quan,_ := decimal.NewFromString(order.QUANTITY())
	oquan,_ := decimal.NewFromString(o.QUANTITY())

	vol = vol.Sub(quan)
	vol = vol.Add(oquan)

	oq.Volume = vol.String() 
	e.Value = o
	return e
}*/

func (oq *XvcOrderQueue) Update(e *XvcOrder,diff decimal.Decimal) {
    if e == nil || diff.LessThanOrEqual(decimal.Zero) {
	return
    }

    vol,_ := decimal.NewFromString(oq.Volume)
    quan,_ := decimal.NewFromString(e.QUANTITY())

    vol = vol.Sub(quan)
    vol = vol.Add(diff)

    oq.Volume = vol.StringFixed(10)
    e.Quantity = diff.StringFixed(10)
}

// Remove removes order from the queue and link order chain
func (oq *XvcOrderQueue) Remove(e *list.Element) *XvcOrder {

    log.Debug("","XvcOrderQueue.remove e",e)
        order := e.Value.(*XvcOrder)
	if order == nil {
	    return nil
	}
	log.Debug("","XvcOrderQueue.remove order",order)
	log.Debug("","XvcOrderQueue.remove oq.Volume",oq.Volume)

	vol,_ := decimal.NewFromString(oq.Volume)
	quan,_ := decimal.NewFromString(order.QUANTITY())
	vol = vol.Sub(quan)

	oq.Volume = vol.StringFixed(10)
	return oq.Orders.Remove(e).(*XvcOrder)
}

// String implements fmt.Stringer interface
func (oq *XvcOrderQueue) String() string {
	sb := strings.Builder{}
	iter := oq.Orders.Front()
	sb.WriteString(fmt.Sprintf("\nqueue length: %d, price: %s, volume: %s, orders:", oq.Len(), oq.PRICE(), oq.VOLUME()))
	for iter != nil {
		order := iter.Value.(*XvcOrder)
		str := fmt.Sprintf("\n\tid: %s, volume: %s, time: %s", order.ID(), order.QUANTITY(), order.PRICE())
		sb.WriteString(str)
		iter = iter.Next()
	}
	return sb.String()
}

func (oq *XvcOrderQueue) OrderVolume(trade string) decimal.Decimal {
    ret := decimal.Zero

    iter := oq.Orders.Front()
    for iter != nil {
	order := iter.Value.(*XvcOrder)
	if order != nil && order.TRADE() == trade {
	    vol,_ := decimal.NewFromString(order.QUANTITY())
	    ret = ret.Add(vol)
	}
	iter = iter.Next()
    }

    return ret
}

