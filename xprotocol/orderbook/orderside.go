package orderbook

import (
	rbtx "github.com/emirpasic/gods/examples/redblacktreeextended"
	rbt "github.com/emirpasic/gods/trees/redblacktree"
	"github.com/shopspring/decimal"
	"github.com/fusion/go-fusion/log"
	"container/list"
	"strings"
	"sync"
	"fmt"
	"github.com/fusion/go-fusion/mongodb"
)

type XvcOrderSide struct {
	PriceTree *rbtx.RedBlackTreeExtended
	
	Prices    map[string]*XvcOrderQueue
	Volume    string 
	NumOrders int
	Depth     int
	Lock sync.Mutex
}

func rbtComparator(a, b interface{}) int {
	return a.(decimal.Decimal).Cmp(b.(decimal.Decimal))
}

func NewOrderSide() *XvcOrderSide {
	//return &XvcOrderSide{
	//	Prices: map[string]*XvcOrderQueue{},
	//	Volume: decimal.Zero,
	//}

	return &XvcOrderSide{
		PriceTree: &rbtx.RedBlackTreeExtended{
			Tree: rbt.NewWith(rbtComparator),
		},
		Prices: map[string]*XvcOrderQueue{},
		Volume: "0",
	}
}

func (os *XvcOrderSide) Len() int {
	return os.NumOrders
}

func (os *XvcOrderSide) VOLUME() string {
	return os.Volume
}

// Depth returns depth of market
func (os *XvcOrderSide) DEPTH() int {
	return os.Depth
}

// Append appends order to definite price level
func (os *XvcOrderSide) Append(o *XvcOrder) *list.Element {
	price := o.PRICE()
	strPrice := price

	os.Lock.Lock()
	priceQueue, ok := os.Prices[strPrice]
	if !ok {
		priceQueue = NewXvcOrderQueue(o.PRICE())
		//log.Debug("","XvcOrderSide.Append, priceQueue",priceQueue)
		if priceQueue == nil {
		    os.Lock.Unlock()
		    return nil
		}

		os.Prices[strPrice] = priceQueue
		tmp,_ := decimal.NewFromString(price)
		os.PriceTree.Put(tmp, priceQueue)
		os.Depth++
	}
	os.NumOrders++
	vol,_ := decimal.NewFromString(os.Volume)
	quan,_ := decimal.NewFromString(o.QUANTITY())
	vol = vol.Add(quan)
	os.Volume = vol.StringFixed(10)
	r := priceQueue.Append(o)
	os.Lock.Unlock()
	//mongo insert Cache
	mongodb.InsertOrderCache(o.ID(), o.PRICE(), priceQueue.Volume, o.TRADE(), o.SIDE())
	return r
}

// Remove removes order from definite price level
func (os *XvcOrderSide) Remove(e *list.Element) *XvcOrder {
	price := e.Value.(*XvcOrder).PRICE()
	strPrice := price

	os.Lock.Lock()
	priceQueue := os.Prices[strPrice]
	log.Debug("","XvcOrderSide,Remove, priceQueue",priceQueue)
	if priceQueue == nil {
	    os.Lock.Unlock()
	    return nil
	}

	o := priceQueue.Remove(e)

	if priceQueue.Len() == 0 {
		delete(os.Prices, strPrice)
		tmp,_ := decimal.NewFromString(price)
		os.PriceTree.Remove(tmp)
		os.Depth--
	}

	os.NumOrders--
	vol,_ := decimal.NewFromString(os.Volume)
	quan,_ := decimal.NewFromString(o.QUANTITY())
	vol = vol.Sub(quan)
	os.Volume = vol.StringFixed(10)
	os.Lock.Unlock()
	return o
}

// MaxPriceQueue returns maximal level of price
func (os *XvcOrderSide) MaxPriceQueue() *XvcOrderQueue {
	if os.Depth > 0 {
		if value, found := os.PriceTree.GetMax(); found {
			return value.(*XvcOrderQueue)
		}
	}
	return nil
}

// MinPriceQueue returns maximal level of price
func (os *XvcOrderSide) MinPriceQueue() *XvcOrderQueue {
	if os.Depth > 0 {
		if value, found := os.PriceTree.GetMin(); found {
			return value.(*XvcOrderQueue)
		}
	}
	return nil
}

// LessThan returns nearest OrderQueue with price less than given
func (os *XvcOrderSide) LessThan(price decimal.Decimal) *XvcOrderQueue {
	tree := os.PriceTree.Tree
	node := tree.Root

	var floor *rbt.Node
	for node != nil {
		if tree.Comparator(price, node.Key) > 0 {
			floor = node
			node = node.Right
		} else {
			node = node.Left
		}
	}

	if floor != nil {
		return floor.Value.(*XvcOrderQueue)
	}

	return nil
}

// GreaterThan returns nearest OrderQueue with price greater than given
func (os *XvcOrderSide) GreaterThan(price decimal.Decimal) *XvcOrderQueue {
	tree := os.PriceTree.Tree
	node := tree.Root

	var ceiling *rbt.Node
	for node != nil {
		if tree.Comparator(price, node.Key) < 0 {
			ceiling = node
			node = node.Left
		} else {
			node = node.Right
		}
	}

	if ceiling != nil {
		return ceiling.Value.(*XvcOrderQueue)
	}

	return nil
}

func (os *XvcOrderSide) Orders(trade string) []*OrderRes {
    os.Lock.Lock()
    ods := list.New()
    for k,v := range os.Prices {
	price,_ := decimal.NewFromString(k)
	volume := v.OrderVolume(trade)

	if volume.Equal(decimal.Zero) {
	    continue
	}

	or := &OrderRes{Price:price,Volume:volume}
	ods.PushBack(or)
	//res = append(res,or)
    } 
    
    i := 0
    res := make([]*OrderRes,ods.Len())
    iter := ods.Front()
    for iter != nil {
	or := iter.Value.(*OrderRes)
	if or == nil {
	    continue
	}

	res[i] = or
	i++
	iter = iter.Next()
    }

    os.Lock.Unlock()
    return res
}

// String implements fmt.Stringer interface
func (os *XvcOrderSide) String() string {
	sb := strings.Builder{}

	level := os.MaxPriceQueue()
	for level != nil {
		sb.WriteString(fmt.Sprintf("\n%s -> %s", level.PRICE(), level.VOLUME()))
		p,_ := decimal.NewFromString(level.PRICE())
		level = os.LessThan(p)
	}

	return sb.String()
}

/*func (os *XvcOrderSide) UpdateOrder(e *list.Element,o *XvcOrder) *list.Element {
    if e == nil || o == nil {
	return nil
    }

	os.Lock.Lock()
	price := e.Value.(*XvcOrder).PRICE()
	strPrice := price

	priceQueue := os.Prices[strPrice]
	if priceQueue == nil {
	    os.Lock.Unlock()
	    return nil
	}

	r := priceQueue.Update(e,o)
	os.Lock.Unlock()
	return r
}*/

func (os *XvcOrderSide) UpdateOrder(e *XvcOrder,diff decimal.Decimal) {
    if e == nil || diff.LessThanOrEqual(decimal.Zero) {
	return
    }

    os.Lock.Lock()
    priceQueue := os.Prices[e.PRICE()]
    if priceQueue == nil {
	os.Lock.Unlock()
	return 
    }

    priceQueue.Update(e,diff)
	//mongodb
	mongodb.UpdateOrder(e.ID(), diff.String())
	mongodb.InsertOrderCache(e.ID(), e.PRICE(), priceQueue.Volume, e.TRADE(), e.SIDE())
    os.Lock.Unlock()
    return
}

