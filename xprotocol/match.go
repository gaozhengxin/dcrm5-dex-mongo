// Copyright 2018 The fusion-dcrm 
//Author: caihaijun@fusion.org

package xprotocol 

import (
	"github.com/fusion/go-fusion/crypto/dcrm"
	"math/big"
	"errors"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/core"
	"github.com/fusion/go-fusion/accounts"
	"github.com/fusion/go-fusion/miner"
	"fmt"
	"sync"
	"bytes"
	"io"
	"strings"
	"compress/zlib"
	"github.com/fusion/go-fusion/xprotocol/orderbook"
	"github.com/shopspring/decimal"
	"encoding/gob"
	"container/list"
)

var (
    MAXPRICE,_ = decimal.NewFromString("100000000000000")
    prev_price = decimal.Zero
)

type CurrentVolume struct {
    Id string
    Vol string 
}

type MatchRes struct {
    Price string 
    Done []*orderbook.XvcOrder
    Volumes string
    Orders int

    CurVolume []*CurrentVolume

    //cancel
    Cancel []string
}

///vol
type sortableMatchResVol []*MatchRes

func (s sortableMatchResVol) Len() int {
	return len(s)
}

func (s sortableMatchResVol) Less(i, j int) bool { // 从大到小: [i] >= [j], 从小到大:[i] <= [j]
        ii,_ := decimal.NewFromString(s[i].Volumes)
        jj,_ := decimal.NewFromString(s[j].Volumes)
	return ii.Cmp(jj) >= 0
}

func (s sortableMatchResVol) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

///orders
type sortableMatchResOrders []*MatchRes

func (s sortableMatchResOrders) Len() int {
	return len(s)
}

func (s sortableMatchResOrders) Less(i, j int) bool { // 从大到小: [i] >= [j], 从小到大:[i] <= [j]
	return s[i].Orders >= s[j].Orders
}

func (s sortableMatchResOrders) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func EncodeMatchRes(res *MatchRes) (string,error) {
    if res == nil {
	return "",errors.New("match res is nil.")
    }

    gob.Register(decimal.Decimal{})
    gob.Register(orderbook.XvcOrderQueue{})
    gob.Register(orderbook.XvcOrderBook{})
    var buff bytes.Buffer
    enc := gob.NewEncoder(&buff)

    err1 := enc.Encode(res)
    if err1 != nil {
	log.Debug("=======EncodeMatchRes,encode match result fail.========","err",err1)
	return "",err1
    }

    return buff.String(),nil
}

func DecodeMatchRes(s string) (*MatchRes,error) {
    var data bytes.Buffer
    data.Write([]byte(s))
    
    dec := gob.NewDecoder(&data)

    var res MatchRes
    err := dec.Decode(&res)
    if err != nil {
	    panic(err)
    }

    return &res,nil
} 

////compress
func Compress(c []byte) (string,error) {

    if c == nil {
	return "",dcrm.GetRetErr(ErrCompressDataNil)
    }

    var in bytes.Buffer
    w,err := zlib.NewWriterLevel(&in,zlib.BestCompression-1)
    if err != nil {
	return "",dcrm.GetRetErr(ErrCompressNewWriterFail)
    }

    w.Write(c)
    w.Close()

    s := in.String()
    return s,nil
}

////uncompress
func UnCompress(s string) (string,error) {

    if s == "" {
	return "",dcrm.GetRetErr(ErrUnCompressDataNil)
    }

    var data bytes.Buffer
    data.Write([]byte(s))

    r,err := zlib.NewReader(&data)
    if err != nil {
	return "",err
    }

    var out bytes.Buffer
    io.Copy(&out, r)
    return out.String(),nil
}
////

type PriceLevel struct {
    Price string 
    Volume string
    Side string
}

func Max(a decimal.Decimal,b decimal.Decimal) decimal.Decimal {
    if a.GreaterThanOrEqual(b) {
	return a
    } 

    return b
}

func Min(a decimal.Decimal,b decimal.Decimal) decimal.Decimal {
    if a.LessThanOrEqual(b) {
	return a
    }

    return b
}

func CalcPriceFromHistory(p1 decimal.Decimal,p2 decimal.Decimal,prev decimal.Decimal,valids []*PriceLevel) decimal.Decimal {
    m1 := p1.Sub(prev)
    if m1.Sign() < 0 {
	m1 = (decimal.Zero).Sub(m1)
    }

    m2 := p2.Sub(prev)
    if m2.Sign() < 0 {
	m2 = (decimal.Zero).Sub(m2)
    }

    if len(valids) == 0 {
	if m1.Equal(m2) {
	    return Min(p1,p2)
	}

	min := Min(m1,m2)
	if min.Equal(m1) {
	    return p1
	}

	return p2
    }
    
    var prices = make(map[decimal.Decimal]decimal.Decimal)
    prices[p1] = m1
    prices[p2] = m2
    for _,v := range valids {
	vtmp,_ := decimal.NewFromString(v.Price)
	m := vtmp.Sub(prev)
	if m.Sign() < 0 {
	    m = (decimal.Zero).Sub(m)
	}
	prices[vtmp] = m
    }

    min := m1
    for _,v := range prices {
	if v.LessThan(min) {
	    min = v
	}
    }

    kk := 0
    var pp decimal.Decimal
    ps := make([]decimal.Decimal,len(prices)) //TODO
    for k,v := range prices {
	if min.Equal(v) {
	    //ps = append(ps,k)
	    ps[kk] = k
	    kk++
	    pp = k
	}
    }

    log.Debug("","CalcPriceFromHistory.ps",ps)
    for _,v := range ps {
	if v.Equal(decimal.Zero) == false && v.LessThan(pp) {  //TODO
	    pp = v
	}
    }

    return pp
}

/*func CopyOf(d *orderbook.XvcOrderBook) *orderbook.XvcOrderBook {
    log.Info("===========d.CopyOf=================")
    ///
    if d == nil || d.Buys.Len() == 0 || d.Sells.Len() == 0 {
	return nil
    }
    ///
    B := d.Buys.MaxPriceQueue()
    S := d.Sells.MinPriceQueue()
    if B == nil || S == nil {
	return nil 
    }
    
    var Bi decimal.Decimal
    var Sj decimal.Decimal
    var Bii decimal.Decimal
    var Sjj decimal.Decimal

    Bii = B.PRICE()
    Sjj = S.PRICE()
    Bi = Bii
    Sj = Sjj

    Bv := B.VOLUME()
    Sv := S.VOLUME()

    for Sjj.LessThanOrEqual(Bii) {
	Bi = Bii
	Sj = Sjj

	//1
	if Sv.LessThan(Bv) {
	    Bv = Bv.Sub(Sv)
	    S = d.Sells.GreaterThan(S.PRICE())
	    if S == nil {
		Sjj = MAXPRICE
		Sv = decimal.Zero
		break
	    }

	    if S.PRICE().GreaterThan(B.PRICE()) {
		Sjj = S.PRICE()
		Sv = decimal.Zero
		break
	    }

	    Sjj = S.PRICE()
	    Sv = S.VOLUME()

	    continue
	}

	//2
	if Sv.Equal(Bv) {

	    B = d.Buys.LessThan(B.PRICE())
	    if B == nil {
		Bii = decimal.Zero
		Bv = decimal.Zero
	    
		S = d.Sells.GreaterThan(S.PRICE())
		if S == nil {
		    Sjj = MAXPRICE
		    Sv = decimal.Zero
		    break
		}
		
		Sjj = S.PRICE()
		Sv = decimal.Zero
		break
	    }
	    
	    Bii = B.PRICE()
	    Bv = B.VOLUME()
	    S = d.Sells.GreaterThan(S.PRICE())
	    if S == nil {
		Sjj = MAXPRICE
		Sv = decimal.Zero
		break
	    }
	    
	    if S.PRICE().GreaterThan(B.PRICE()) {
		Sjj = S.PRICE()
		Sv = decimal.Zero
		break
	    }

	    Sjj = S.PRICE()
	    Sv = S.VOLUME()

	    continue
	}

	//3
	if Sv.GreaterThan(Bv) {
	    Sv = Sv.Sub(Bv)

	    B = d.Buys.LessThan(B.PRICE())
	    if B == nil {
		Bii = decimal.Zero
		Bv = decimal.Zero
		break
	    }

	    if S.PRICE().GreaterThan(B.PRICE()) {
		Bii = B.PRICE()
		Bv = decimal.Zero
		break
	    }

	    Bii = B.PRICE()
	    Bv = B.VOLUME()

	    continue
	}
    }
    
    P1 := Max(Sj, Bii)
    P2 := Min(Bi,Sjj)
    if P1.GreaterThan(P2) {
	log.Debug("===========d.CopyOf 22222222=================")
	return nil 
    }

    ob := orderbook.NewXvcOrderBook()
    tmp := d.Buys.MaxPriceQueue()
    for tmp != nil && tmp.PRICE().GreaterThanOrEqual(Bi) {
	od := tmp.Head()
	for od != nil {
	    o := orderbook.NewXvcOrder(od.Value.(*orderbook.XvcOrder).ID(),od.Value.(*orderbook.XvcOrder).FROM(),od.Value.(*orderbook.XvcOrder).TRADE(),od.Value.(*orderbook.XvcOrder).ORDERTYPE(),od.Value.(*orderbook.XvcOrder).SIDE(),od.Value.(*orderbook.XvcOrder).TIME(),od.Value.(*orderbook.XvcOrder).QUANTITY(),od.Value.(*orderbook.XvcOrder).PRICE(),od.Value.(*orderbook.XvcOrder).RULE())
	    log.Info("===========d.CopyOf,insert to orderbook=================","order id",od.Value.(*orderbook.XvcOrder).ID(),"order",o)
	    ob.InsertToOrderBook(o)
	    od = od.Next()
	}
	tmp = d.Buys.LessThan(tmp.PRICE())
    }

    tmp = d.Sells.MinPriceQueue()
    for tmp != nil && tmp.PRICE().LessThanOrEqual(Sj) {
	od := tmp.Head()
	for od != nil {
	    o := orderbook.NewXvcOrder(od.Value.(*orderbook.XvcOrder).ID(),od.Value.(*orderbook.XvcOrder).FROM(),od.Value.(*orderbook.XvcOrder).TRADE(),od.Value.(*orderbook.XvcOrder).ORDERTYPE(),od.Value.(*orderbook.XvcOrder).SIDE(),od.Value.(*orderbook.XvcOrder).TIME(),od.Value.(*orderbook.XvcOrder).QUANTITY(),od.Value.(*orderbook.XvcOrder).PRICE(),od.Value.(*orderbook.XvcOrder).RULE())
	    log.Info("===========d.CopyOf,insert to orderbook=================","order id",od.Value.(*orderbook.XvcOrder).ID(),"order",o)
	    ob.InsertToOrderBook(o)
	    od = od.Next()
	}
	tmp = d.Sells.GreaterThan(tmp.PRICE())
    }

    if ob.Orders.MapLength() == 0 {
	ob = nil
    }

    return ob
}*/

func DeepCopy(d *orderbook.XvcOrderBook,trade string,nonce *big.Int) *orderbook.XvcOrderBook {
    ///
    if d == nil || d.Buys.Len() == 0 || d.Sells.Len() == 0 {
	return nil
    }
    ///

    if trade == "" || nonce == nil {
	return nil
    }

    ob := orderbook.NewXvcOrderBook()
    _,value := d.Orders.ListMap()
    log.Info("==============DeepCopy=================","deep copy total order count",len(value),"match nonce",nonce,"trade",trade)

    buy := 0
    sell := 0

    for _,od := range value {
	o := orderbook.NewXvcOrder(od.Value.(*orderbook.XvcOrder).ID(),od.Value.(*orderbook.XvcOrder).FROM(),od.Value.(*orderbook.XvcOrder).TRADE(),od.Value.(*orderbook.XvcOrder).ORDERTYPE(),od.Value.(*orderbook.XvcOrder).SIDE(),od.Value.(*orderbook.XvcOrder).TIME(),od.Value.(*orderbook.XvcOrder).QUANTITY(),od.Value.(*orderbook.XvcOrder).PRICE(),od.Value.(*orderbook.XvcOrder).RULE())
	ob.InsertToOrderBook(o)

	if strings.EqualFold(o.SIDE(),"Buy") {
	    buy++
	}
	if strings.EqualFold(o.SIDE(),"Sell") {
	    sell++
	}
    }
    
    log.Info("==============DeepCopy=================","deep copy buy order count",buy,"deep copy sell order count",sell,"match nonce",nonce,"trade",trade)
    
    if ob.Orders.MapLength() == 0 {
	ob = nil
    }

    return ob
}

func DeepCopy_Tmp(d *orderbook.XvcOrderBook,trade string,nonce *big.Int) *orderbook.XvcOrderBook {
    ///
    if d == nil || d.Buys.Len() == 0 || d.Sells.Len() == 0 {
	return nil
    }
    ///

    if trade == "" || nonce == nil {
	return nil
    }

    ob := orderbook.NewXvcOrderBook()
    _,value := d.Orders.ListMap_Tmp()
    //log.Info("==============DeepCopy_Tmp=================","deep copy total order count",len(value),"match nonce",nonce,"trade",trade)

    buy := 0
    sell := 0
    var wg sync.WaitGroup

    for _,od := range value {
	wg.Add(1)
	
	go func(v *orderbook.XvcOrder) {
	    defer wg.Done()
	    
	    o := orderbook.NewXvcOrder(v.ID(),v.FROM(),v.TRADE(),v.ORDERTYPE(),v.SIDE(),v.TIME(),v.QUANTITY(),v.PRICE(),v.RULE())
	    ob.InsertToOrderBook(o)
	    
	    if strings.EqualFold(v.SIDE(),"Buy") {
		buy++
	    }
	    if strings.EqualFold(v.SIDE(),"Sell") {
		sell++
	    }
	}(od.Value.(*orderbook.XvcOrder))
    }
    wg.Wait()
    
    //log.Info("==============DeepCopy_Tmp=================","deep copy buy order count",buy,"deep copy sell order count",sell,"match nonce",nonce,"trade",trade)
    
    if ob.Orders.MapLength() == 0 {
	ob = nil
    }

    return ob
}

func RemoveOb(d *orderbook.XvcOrderBook) {
     go RemoveOb2(d)
}

func RemoveOb_Tmp(d *orderbook.XvcOrderBook) {
     RemoveOb2(d)
}

func RemoveOb2(d *orderbook.XvcOrderBook) {
    if d == nil {
	return
    }

    _,value := d.Orders.ListMap()

    for _,od := range value {
	id := od.Value.(*orderbook.XvcOrder).ID()
	d.CancelOrder(id)
    }
}

func FindOrder(e *orderbook.XvcOrder) *orderbook.XvcOrder {
    if e == nil {
	return nil
    }
    
    ob := GetOB(e.TRADE())
    if ob.Orders.MapLength() == 0 {
	return nil
    }

    vv,ok := ob.Orders.ReadMap(e.ID())
    if ok == false {
	return nil
    }

    return vv.Value.(*orderbook.XvcOrder)
}

/////
func MatchMakingBidding(ob *orderbook.XvcOrderBook) (decimal.Decimal,[]*orderbook.XvcOrder,decimal.Decimal,int,error) {
    if ob == nil {
	//log.Info("=============MatchMakingBidding,ob is nil.================")
	return decimal.Zero,nil,decimal.Zero,0,nil
    }

    //var done []*PriceLevel
    var partial *PriceLevel

    var Bi decimal.Decimal
    var Sj decimal.Decimal
    var Bii decimal.Decimal
    var Sjj decimal.Decimal
   
    B := ob.Buys.MaxPriceQueue()
    S := ob.Sells.MinPriceQueue()
    if B == nil || S == nil {
	//log.Info("=============MatchMakingBidding,B is nil or S is nil.================")
	return decimal.Zero,nil,decimal.Zero,0,nil
    }

    Bii,_ = decimal.NewFromString(B.PRICE())
    Sjj,_ = decimal.NewFromString(S.PRICE())
    Bi = Bii
    Sj = Sjj

    Bv,_ := decimal.NewFromString(B.VOLUME())
    Sv,_ := decimal.NewFromString(S.VOLUME())

    for Sjj.LessThanOrEqual(Bii) {
	Bi = Bii
	Sj = Sjj

	//1
	if Sv.LessThan(Bv) {
	    Bv = Bv.Sub(Sv)
	    tmp,_ := decimal.NewFromString(S.PRICE())
	    S = ob.Sells.GreaterThan(tmp)
	    if S == nil {
		Sjj = MAXPRICE
		Sv = decimal.Zero
		partial = &PriceLevel{Price:B.PRICE(),Volume:Bv.String(),Side:"Buy"}
		break
	    }

	    tmp2,_ := decimal.NewFromString(S.PRICE())
	    tmp3,_ := decimal.NewFromString(B.PRICE())
	    if tmp2.GreaterThan(tmp3) {
		Sjj,_ = decimal.NewFromString(S.PRICE())
		Sv = decimal.Zero
		partial = &PriceLevel{Price:B.PRICE(),Volume:Bv.String(),Side:"Buy"}
		break
	    }

	    Sjj,_ = decimal.NewFromString(S.PRICE())
	    Sv,_ = decimal.NewFromString(S.VOLUME())
	    partial = &PriceLevel{Price:B.PRICE(),Volume:Bv.StringFixed(10),Side:"Buy"}

	    continue
	}

	//2
	if Sv.Equal(Bv) {
	    partial = nil

	    tmpb,_ := decimal.NewFromString(B.PRICE())
	    B = ob.Buys.LessThan(tmpb)
	    if B == nil {
		Bii = decimal.Zero
		Bv = decimal.Zero
	    
		tmps,_ := decimal.NewFromString(S.PRICE())
		S = ob.Sells.GreaterThan(tmps)
		if S == nil {
		    Sjj = MAXPRICE
		    Sv = decimal.Zero
		    break
		}
		
		Sjj,_ = decimal.NewFromString(S.PRICE())
		Sv = decimal.Zero
		break
	    }
	    
	    Bii,_ = decimal.NewFromString(B.PRICE())
	    Bv,_ = decimal.NewFromString(B.VOLUME())

	    ss,_ := decimal.NewFromString(S.PRICE())
	    S = ob.Sells.GreaterThan(ss)
	    if S == nil {
		Sjj = MAXPRICE
		Sv = decimal.Zero
		break
	    }
	    
	    bb,_ := decimal.NewFromString(B.PRICE())
	    sp,_ := decimal.NewFromString(S.PRICE())
	    if sp.GreaterThan(bb) {
		Sjj,_ = decimal.NewFromString(S.PRICE())
		Sv = decimal.Zero
		break
	    }

	    Sjj,_ = decimal.NewFromString(S.PRICE())
	    Sv,_ = decimal.NewFromString(S.VOLUME())

	    continue
	}

	//3
	if Sv.GreaterThan(Bv) {
	    Sv = Sv.Sub(Bv)

	    tmp,_ := decimal.NewFromString(B.PRICE())
	    B = ob.Buys.LessThan(tmp)
	    if B == nil {
		Bii = decimal.Zero
		Bv = decimal.Zero
		partial = &PriceLevel{Price:S.PRICE(),Volume:Sv.StringFixed(10),Side:"Sell"}
		break
	    }

	    tmpbb,_ := decimal.NewFromString(B.PRICE())
	    tmpss,_ := decimal.NewFromString(S.PRICE())
	    if tmpss.GreaterThan(tmpbb) {
		Bii,_ = decimal.NewFromString(B.PRICE())
		Bv = decimal.Zero
		partial = &PriceLevel{Price:S.PRICE(),Volume:Sv.StringFixed(10),Side:"Sell"}
		break
	    }

	    Bii,_ = decimal.NewFromString(B.PRICE())
	    Bv,_ = decimal.NewFromString(B.VOLUME())
	    partial = &PriceLevel{Price:S.PRICE(),Volume:Sv.StringFixed(10),Side:"Sell"}

	    continue
	}
    }

    P1 := Max(Sj, Bii)
    P2 := Min(Bi,Sjj)
    if P1.GreaterThan(P2) {
	//log.Info("=============MatchMakingBidding,P1 > P2.================")
	return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
    }

    //price
    var valids []*PriceLevel
    var price decimal.Decimal
    var p1left decimal.Decimal
    var p2left decimal.Decimal
    
    if P1.Equal(P2) {
	price = P1
	prev_price = price
    } else {
	BB := ob.Buys.MaxPriceQueue()
	if BB == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}
	
	bbp,_ := decimal.NewFromString(BB.PRICE())
	SS := ob.Sells.MinPriceQueue()
	if SS == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	ssp,_ := decimal.NewFromString(SS.PRICE())

	tmpb,_ := decimal.NewFromString(BB.PRICE())
	for BB != nil && tmpb.GreaterThanOrEqual(ssp) {
	    if tmpb.GreaterThan(P1) && tmpb.LessThan(P2) {
		p := &PriceLevel{Price:BB.PRICE(),Volume:"0",Side:"Buy"}
		valids = append(valids,p)
	    }
	    BB = ob.Buys.LessThan(tmpb)
	    if BB == nil {
		break
	    }

	    tmpb,_ = decimal.NewFromString(BB.PRICE())
	}

	tmps,_ := decimal.NewFromString(SS.PRICE())
	for SS != nil && tmps.LessThanOrEqual(bbp) {
	    if tmps.GreaterThan(P1) && tmps.LessThan(P2) {
		p := &PriceLevel{Price:SS.PRICE(),Volume:"0",Side:"Sell"}
		valids = append(valids,p)
	    }
	    SS = ob.Sells.GreaterThan(tmps)
	    if SS == nil {
		break
	    }

	    tmps,_ = decimal.NewFromString(SS.PRICE())
	}

	if len(valids) == 0 {
	    //p1
	    if P1.Equal(Sj) {
		if partial == nil {
		    p1left = decimal.Zero
		}
		if partial != nil && strings.EqualFold(partial.Side,"Sell") == true {
		    p1left,_ = decimal.NewFromString(partial.Volume)
		}
		if partial != nil && strings.EqualFold(partial.Side,"Buy") == true {
		    p1left = decimal.Zero
		}
	    } else {
		if partial == nil {
		    if Bii.Equal(Bi) {
			p1left = decimal.Zero
		    } else {
			tmp := ob.Buys.LessThan(Bi)
			p1left,_ = decimal.NewFromString(tmp.VOLUME())
		    }
		}
		if partial != nil && strings.EqualFold(partial.Side,"Sell") == true {
		    if Bii.Equal(Bi) {
			p1left = decimal.Zero
		    } else {
			tmp := ob.Buys.LessThan(Bi)
			p1left,_ = decimal.NewFromString(tmp.VOLUME())
		    }
		}
		if partial != nil && strings.EqualFold(partial.Side,"Buy") == true {
		    if Bii.Equal(Bi) {
			p1left,_ = decimal.NewFromString(partial.Volume)
		    } else {
			tmp := ob.Buys.LessThan(Bi)
			p1left,_ = decimal.NewFromString(tmp.VOLUME())
		    }
		}
	    }

	    //p2
	    ////////
	    if P2.Equal(Sjj) {
		if partial == nil {
		    if Sjj.Equal(Sj) {
			p2left = decimal.Zero
		    } else {
			tmp := ob.Sells.GreaterThan(Sj)
			p2left,_ = decimal.NewFromString(tmp.VOLUME())
		    }
		}
		if partial != nil && strings.EqualFold(partial.Side,"Sell") == true {
		    if Sjj.Equal(Sj) {
			p2left,_ = decimal.NewFromString(partial.Volume)
		    } else {
			tmp := ob.Sells.GreaterThan(Sj)
			p2left,_ = decimal.NewFromString(tmp.VOLUME())
		    }
		}
		if partial != nil && strings.EqualFold(partial.Side,"Buy") == true {
		    if Sjj.Equal(Sj) {
			p2left = decimal.Zero 
		    } else {
			tmp := ob.Sells.GreaterThan(Sj)
			p2left,_ = decimal.NewFromString(tmp.VOLUME())
		    }
		}
	    } else {
		if partial == nil {
		    p2left = decimal.Zero
		}
		if partial != nil && strings.EqualFold(partial.Side,"Sell") == true {
		    p2left = decimal.Zero
		}
		if partial != nil && strings.EqualFold(partial.Side,"Buy") == true {
		    p2left,_ = decimal.NewFromString(partial.Volume)
		}
	    }
	    ////////

	    if p1left.LessThan(p2left) {
		price = P1
		prev_price = price
	    }
	    if p1left.GreaterThan(p2left) {
		price = P2
		prev_price = price
	    }
	    if p1left.Equal(p2left) {
		price = CalcPriceFromHistory(P1,P2,prev_price,valids)
		log.Debug("=============MatchMakingBidding,calc from history","price",price,"","=============")
		prev_price = price
	    }
	} else {
		price = CalcPriceFromHistory(P1,P2,prev_price,valids)
		log.Debug("=============MatchMakingBidding,calc from history","price",price,"","=============")
		prev_price = price
	}
    }

    //done
    dones := list.New()
    volumes := decimal.Zero
    var orders int 

    if partial == nil {

	tmp := ob.Buys.MaxPriceQueue()
	if tmp == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	bc := 0

	pp,_ := decimal.NewFromString(tmp.PRICE())
	for tmp != nil && pp.GreaterThanOrEqual(Bi) {
	    od := tmp.Head()
	    for od != nil {
		org := FindOrder(od.Value.(*orderbook.XvcOrder))
		if org != nil {
		    dones.PushBack(org)
		}
		od = od.Next()
		bc++
	    }
	    log.Debug("","tmp.vol",tmp.VOLUME(),"volumes",volumes.StringFixed(10))
	    vol,_ := decimal.NewFromString(tmp.VOLUME())
	    volumes = volumes.Add(vol)
	    tmp = ob.Buys.LessThan(pp)
	    if tmp == nil {
		break
	    }

	    pp,_ = decimal.NewFromString(tmp.PRICE())
	}
	
	//log.Info("================MatchMakingBidding,no partial.==================","buy done count",bc)

	tmp = ob.Sells.MinPriceQueue()
	if tmp == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	sc := 0
	
	pp,_ = decimal.NewFromString(tmp.PRICE())
	for tmp != nil && pp.LessThanOrEqual(Sj) {
	    od := tmp.Head()
	    for od != nil {
		org := FindOrder(od.Value.(*orderbook.XvcOrder))
		if org != nil {
		    dones.PushBack(org)
		}
		od = od.Next()
		sc++
	    }
	    tmp = ob.Sells.GreaterThan(pp)
	    if tmp == nil {
		break
	    }

	    pp,_ = decimal.NewFromString(tmp.PRICE())
	}
	
	//log.Info("================MatchMakingBidding,no partial.==================","sell done count",sc)
    }
    
    if partial != nil && strings.EqualFold(partial.Side,"Sell") == true {
	tmp := ob.Buys.MaxPriceQueue()
	if tmp == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	bc := 0 

	pp,_ := decimal.NewFromString(tmp.PRICE())
	for tmp != nil && pp.GreaterThanOrEqual(Bi) {
	    od := tmp.Head()
	    for od != nil {
		org := FindOrder(od.Value.(*orderbook.XvcOrder))
		if org != nil {
		    dones.PushBack(org)
		}
		od = od.Next()
		bc++
	    }
	    
	    log.Debug("","tmp.vol",tmp.VOLUME(),"volumes",volumes.StringFixed(10))
	    vol,_ := decimal.NewFromString(tmp.VOLUME())
	    volumes = volumes.Add(vol)
	    tmp = ob.Buys.LessThan(pp)
	    if tmp == nil {
		break
	    }

	    pp,_ = decimal.NewFromString(tmp.PRICE())
	}
	
	//log.Info("================MatchMakingBidding,partial(no new)==================","buy done count",bc)

	tmp = ob.Sells.MinPriceQueue()
	if tmp == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	sc1 := 0
	sc2 := 0

	pp,_ = decimal.NewFromString(tmp.PRICE())
	for tmp != nil && pp.LessThanOrEqual(Sj) {
	    if pp.Equal(Sj) {
		vol,_ := decimal.NewFromString(tmp.VOLUME())
		if vol.Equal(decimal.Zero) {
		    break
		}

		pv,_ := decimal.NewFromString(partial.Volume)
		processed := vol.Sub(pv)
		ratio := processed.Div(vol)
		od := tmp.Head()
		for od != nil {
		    oq,_ := decimal.NewFromString(od.Value.(*orderbook.XvcOrder).QUANTITY())
		    o := orderbook.NewXvcOrder(od.Value.(*orderbook.XvcOrder).ID(),od.Value.(*orderbook.XvcOrder).FROM(),od.Value.(*orderbook.XvcOrder).TRADE(),od.Value.(*orderbook.XvcOrder).ORDERTYPE(),od.Value.(*orderbook.XvcOrder).SIDE(),od.Value.(*orderbook.XvcOrder).TIME(),oq.Mul(ratio).StringFixed(10),od.Value.(*orderbook.XvcOrder).PRICE(),od.Value.(*orderbook.XvcOrder).RULE())
		    dones.PushBack(o)
		    od = od.Next()
		    sc1++
		}
		
		//log.Info("================MatchMakingBidding,partial(new)==================","sell done count",sc1)

		break
	    }

	    od := tmp.Head()
	    for od != nil {
		org := FindOrder(od.Value.(*orderbook.XvcOrder))
		if org != nil {
		    dones.PushBack(org)
		}
		od = od.Next()
		sc2++
	    }
	    
	    tmp = ob.Sells.GreaterThan(pp)
	    if tmp == nil {
		break
	    }

	    pp,_ = decimal.NewFromString(tmp.PRICE())
	}
	
	//log.Info("================MatchMakingBidding,partial(no new)==================","sell done count",sc2)
    }

    if partial != nil && strings.EqualFold(partial.Side,"Buy") == true {
	tmp := ob.Buys.MaxPriceQueue()
	if tmp == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	bc1 := 0
	bc2 := 0

	pp,_ := decimal.NewFromString(tmp.PRICE())
	for tmp != nil && pp.GreaterThanOrEqual(Bi) {
	    if pp.Equal(Bi) {
		vol,_ := decimal.NewFromString(tmp.VOLUME())
		if vol.Equal(decimal.Zero) {
		    break
		}

		pv,_ := decimal.NewFromString(partial.Volume)
		processed := vol.Sub(pv)
		ratio := processed.Div(vol)
		od := tmp.Head()
		for od != nil {
		    oq,_ := decimal.NewFromString(od.Value.(*orderbook.XvcOrder).QUANTITY())
		    o := orderbook.NewXvcOrder(od.Value.(*orderbook.XvcOrder).ID(),od.Value.(*orderbook.XvcOrder).FROM(),od.Value.(*orderbook.XvcOrder).TRADE(),od.Value.(*orderbook.XvcOrder).ORDERTYPE(),od.Value.(*orderbook.XvcOrder).SIDE(),od.Value.(*orderbook.XvcOrder).TIME(),oq.Mul(ratio).StringFixed(10),od.Value.(*orderbook.XvcOrder).PRICE(),od.Value.(*orderbook.XvcOrder).RULE())
		    dones.PushBack(o)
		    od = od.Next()
		    bc1++
		}
		//log.Info("================MatchMakingBidding,partial(new)==================","buy done count",bc1)

		break
	    }

	    od := tmp.Head()
	    for od != nil {
		org := FindOrder(od.Value.(*orderbook.XvcOrder))
		if org != nil {
		    dones.PushBack(org)
		}
		od = od.Next()
		bc2++
	    }
	    tmp = ob.Buys.LessThan(pp)
	    if tmp == nil {
		break
	    }

	    pp,_ = decimal.NewFromString(tmp.PRICE())
	}
	
	//log.Info("================MatchMakingBidding,partial(no new)==================","buy done count",bc2)

	tmp = ob.Sells.MinPriceQueue()
	if tmp == nil {
	    return decimal.Zero,nil,decimal.Zero,0,dcrm.GetRetErr(ErrMatchOrderBookFail)
	}

	sc := 0

	pp,_ = decimal.NewFromString(tmp.PRICE())
	for tmp != nil && pp.LessThanOrEqual(Sj) {
	    od := tmp.Head()
	    for od != nil {
		org := FindOrder(od.Value.(*orderbook.XvcOrder))
		if org != nil {
		    dones.PushBack(org)
		}
		od = od.Next()
		sc++
	    }
	     
	    log.Debug("","tmp.vol",tmp.VOLUME(),"volumes",volumes.StringFixed(10))
	    vol,_ := decimal.NewFromString(tmp.VOLUME())
	    volumes = volumes.Add(vol)
	    tmp = ob.Sells.GreaterThan(pp)
	    if tmp == nil {
		break
	    }

	    pp,_ = decimal.NewFromString(tmp.PRICE())
	}
	//log.Info("================MatchMakingBidding,partial(no new)==================","sell done count",sc)
    }

    i := 0
    orders = dones.Len()
    done := make([]*orderbook.XvcOrder,orders)
    iter := dones.Front()
    for iter != nil {
	order := iter.Value.(*orderbook.XvcOrder)
	if order == nil {
	    continue
	}

	done[i] = order
	i++
	iter = iter.Next()
    }

    return price,done,volumes,orders,nil
}

func GetZipData(mr *MatchRes) (string,error) {
    if mr == nil {
	return "",errors.New("match res is nil.")
    }

    res,err := EncodeMatchRes(mr)
    if err != nil {
	return "",err
    }

    res_compress,err := Compress([]byte(res))
    if err != nil {
	return "",err
    }

    return res_compress,nil
}

func GetMatchRes(s string) (*MatchRes,error) {
    if s == "" {
	return nil,errors.New("zip data is nil.")
    }

    v_uncompress,err := UnCompress(s)
    if err != nil {
	return nil,err
    }
    
    mr,err := DecodeMatchRes(v_uncompress)
    if err != nil {
	return nil,err
    }

    return mr,nil
}

func SendOrderbookTx(res_compress string,matchnonce *big.Int,trade string) error {
    if res_compress == "" || matchnonce == nil || trade == "" {
	return dcrm.GetRetErr(ErrMatchResNil)
    }

    cb,e := dcrm.Coinbase()
    if e != nil {
	return dcrm.GetRetErr(dcrm.ErrInvalidCoinbase)
    }
    
    fusionaddr := cb.Hex()
    fusions := []rune(fusionaddr)
    if len(fusions) != 42 { //42 = 2 + 20*2 =====>0x + addr
	log.Debug("SendOrderbookTx,fusion addr format error.")
	return dcrm.GetRetErr(dcrm.ErrParamError)
    }

    fromaddr,_ := new(big.Int).SetString(fusionaddr,0)
    txfrom := common.BytesToAddress(fromaddr.Bytes())

    toaddr := new(common.Address)
    *toaddr = types.XvcPrecompileAddr

    bn := fmt.Sprintf("%v",matchnonce)
    str := "ODB" + common.SepOB + bn + common.SepOB + res_compress + common.SepOB + trade

    account := accounts.Account{Address: txfrom}
    wallet, err := dcrm.AccountManager().Find(account)
    if err != nil {
	    return err
    }

    nonce := uint64(miner.TxPool().State().GetNonce(txfrom))
    qn,err := miner.TxPool().GetDcrmTxRealNonce(txfrom.Hex())
    if err == nil {
	//log.Info("============SendOrderbookTx,","get nonce",nonce,"get queue",qn,"match nonce",matchnonce,"","================")
	nonce = nonce + uint64(qn)
    }
    //log.Info("============SendOrderbookTx,","from nonce",nonce,"match nonce",matchnonce,"","================")

    amount, _ := new(big.Int).SetString("0",10)
    gaslimit,err := core.IntrinsicGas([]byte(str),false,false)
    if err != nil {
	return err
    }
    ////////////////
    
    tx := types.NewTransaction(
	uint64(nonce),   // nonce 
	*toaddr,  // receive address
	amount,
	gaslimit,
	big.NewInt(41000000000), // gasPrice
	[]byte(str)) // data

    if tx == nil {
	return dcrm.GetRetErr(ErrNewMatchResTxFail)
    }
    ///////////

    var chainID *big.Int
    if config := miner.ChainConfig(); config.IsEIP155(big.NewInt(int64(miner.GetCurrentBlockHeight()))) {
	chainID = config.ChainID
    }
    
    signed, err := wallet.SignTx(account, tx, chainID)
    if err != nil {
	    return err
    }
    
    err = miner.TxPool().AddLocal(signed)
    log.Info("============SendOrderbookTx=================","err",err,"tx nonce",nonce,"match nonce",matchnonce,"trade",trade,"tx hash",signed.Hash(),"gas limit",gaslimit)
    return err
}

