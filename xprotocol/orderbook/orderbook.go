package orderbook

import (
	"strings"
	"container/list"
	"github.com/shopspring/decimal"
	"errors"
	"github.com/fusion/go-fusion/log" 
	"encoding/json"
	"sync"
	"github.com/fusion/go-fusion/crypto/dcrm"
)

type SafeMap struct {
    sync.RWMutex
    Map map[string]*list.Element
}

func NewSafeMap(size int) *SafeMap {
    sm := new(SafeMap)
    sm.Map = make(map[string]*list.Element)
    return sm
}

func (sm *SafeMap) ReadMap(key string) (*list.Element,bool) {
    sm.RLock()
    value,ok := sm.Map[key]
    sm.RUnlock()
    return value,ok
}

func (sm *SafeMap) WriteMap(key string, value *list.Element) {
    sm.Lock()
    sm.Map[key] = value
    sm.Unlock()
}

func (sm *SafeMap) DeleteMap(key string) {
    sm.Lock()
    delete(sm.Map,key)
    sm.Unlock()
}

func (sm *SafeMap) ListMap() ([]string,[]*list.Element) {
    sm.RLock()
    l := len(sm.Map)
    key := make([]string,l)
    value := make([]*list.Element,l)
    i := 0
    for k,v := range sm.Map {
	key[i] = k
	value[i] = v
	i++

	if i >= l {
	    break
	}
    }
    sm.RUnlock()

    return key,value
}

func (sm *SafeMap) MapLength() int {
    sm.RLock()
    l := len(sm.Map)
    sm.RUnlock()
    return l
}

////
func (sm *SafeMap) ListMap_Tmp() ([]string,[]*list.Element) {
    sm.RLock()
    l := len(sm.Map)
    //log.Info("========ListMap_Tmp=============","current total order count",l)

    if l > 800 {
	l = 800
    }

    key := make([]string,l)
    value := make([]*list.Element,l)
    i := 0
    for k,v := range sm.Map {
	key[i] = k
	value[i] = v
	i++

	if i >= l {
	    break
	}
    }
    sm.RUnlock()

    return key,value
}
////

type XvcOrderBook struct {
	Orders *SafeMap

	Buys *XvcOrderSide
	Sells *XvcOrderSide
}

func NewXvcOrderBook() *XvcOrderBook {
	return &XvcOrderBook{
		Orders: NewSafeMap(10),
		Buys:   NewOrderSide(),
		Sells:   NewOrderSide(),
	}
}

func (d *XvcOrderBook) Set(k string,v *list.Element) {
  d.Orders.WriteMap(k,v)
}

func (ob *XvcOrderBook) Order(orderID string) *XvcOrder {
	e, ok := ob.Orders.ReadMap(orderID)
	if !ok {
	    return nil
	}

	r := e.Value.(*XvcOrder)
	return r
}

/*func (d *XvcOrderBook) UpdateOrder(e *list.Element,newod *XvcOrder) *list.Element {
    if newod == nil || e == nil {
	return nil
    }

    if strings.EqualFold(e.Value.(*XvcOrder).SIDE(),"Buy") {
	return d.Buys.UpdateOrder(e,newod)
    }
	
    return d.Sells.UpdateOrder(e,newod)
}*/

func (d *XvcOrderBook) UpdateOrder(e *XvcOrder,diff decimal.Decimal) {
    if e == nil || diff.LessThanOrEqual(decimal.Zero) {
	return
    }

    if strings.EqualFold(e.SIDE(),"Buy") {
	d.Buys.UpdateOrder(e,diff)
	return
    }
	
    d.Sells.UpdateOrder(e,diff)
}

func (d *XvcOrderBook) CancelOrder(orderID string) *XvcOrder {

      e, ok := d.Orders.ReadMap(orderID)
	if !ok {
		return nil
	}

	d.Orders.DeleteMap(orderID)

	if strings.EqualFold(e.Value.(*XvcOrder).SIDE(),"Buy") {
	    r := d.Buys.Remove(e)
	    return r
	}

	r := d.Sells.Remove(e)
	return r
}

func (ob *XvcOrderBook) InsertToOrderBook(od *XvcOrder) error {
    if od == nil {
	tmp := `{Code:82,Error:"orderbook: order object is nil."}` //TODO
	    return dcrm.GetRetErr(tmp) 
    }

    orderID := od.ID()
    _, ok := ob.Orders.ReadMap(orderID)
    if ok {
	log.Debug("==========XvcOrderBook.InsertToOrderBook,order has already exsit in orderbook.==============","order id",orderID)
	return errors.New("order has already exsit in orderbook.")
    }

    log.Debug("=============ob.InsertToOrderBook==================","order id",orderID,"order trade",od.TRADE())
    side := od.SIDE()
    if strings.EqualFold(side,"Buy") {
	e := ob.Buys.Append(od)
	ob.Set(orderID,e)
    } else {
	e := ob.Sells.Append(od)
	ob.Set(orderID,e)
    }
    
    return nil
}


type OrderRes struct {
    Price decimal.Decimal
    Volume decimal.Decimal
}

type CurrentOrder struct {
    Sells []*OrderRes `json:"sells"`
    Buys []*OrderRes `json:"buys"`
}

func (ob *XvcOrderBook) String(trade string) (string,error) {

    sells := ob.Sells.Orders(trade)
    buys := ob.Buys.Orders(trade)

    if len(sells) == 0 && len(buys) == 0 {
	return "",nil
    }

    co := &CurrentOrder{Sells:sells,Buys:buys}

    res, err := json.Marshal(co)
    if err != nil {
	return "",err
    }

    return string(res),nil
}

