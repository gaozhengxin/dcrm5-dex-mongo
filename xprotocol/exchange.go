// Copyright 2018
//Author: caihaijun@fusion.org

package xprotocol 

import (
	"fmt"
	"github.com/fusion/go-fusion/crypto/dcrm"
	"github.com/fusion/go-fusion/core/vm"
	"github.com/fusion/go-fusion/xprotocol/orderbook"
	"github.com/shopspring/decimal"
	"strings"
	"errors"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/rpc"
	"github.com/fusion/go-fusion/core"
	"github.com/fusion/go-fusion/core/rawdb"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/core/state"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/mongodb"
	"github.com/fusion/go-fusion/miner"
	"container/list"
	"time"
	"sync"
	"math/big"
	"encoding/json"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
)

var (
    CancelOrders *list.List
    Match bool
    MatchTrade []string

    //MatchNonce *big.Int //must big int
    //startmatch = make(chan bool, 1)
    //timeout = make(chan bool, 1)
    
    //match_res = make(chan MatchResData, 500000)
    all_order = make(chan string, 100000)
    //reorg_match_res = make(chan *types.Transaction, 10000)
    
    OB = common.NewSafeMap(10)
    UNIT = common.NewSafeMap(10)
    
    MatchNonce = common.NewSafeMap(10)
    startmatch = common.NewSafeMap(10)
    timeout = common.NewSafeMap(10)
    match_res = common.NewSafeMap(10)
    
    EXCHANGE_DEFAULT_FEE decimal.Decimal

    ReOrgList *common.Queue

    ch = make(chan int)
    //quit = make(chan bool)
    quit = common.NewSafeMap(10)
)

func init() {
    dcrm.RegisterCheckOrderBalanceCB(IsOrderBalanceOk)
    dcrm.RegisterObCallback(RecvOrder)
    dcrm.RegisterObCallback2(RecvOrder2)
    vm.RegisterSettleOdcb(SettleOrders)
    vm.RegisterGetUnitCB(GetUnit)
    vm.RegisterGetABByTradeCB(GetABByTrade)
    vm.RegisterGetExChangeDefaultFeeCB(GetDefaultFee)
    vm.RegisterAddNewTradeCB(AddNewTrade)
    
    core.RegisterOrderTestcb(CallOrderTest)
    core.RegisterCancelOrdercb(InsertCancelOrder)
    core.RegisterUpdateOBcb(UpdateOB)
    core.RegisterGetTradecb(GetTradeFromMatchRes)
    rawdb.RegisterGetTradecb(GetTradeFromMatchRes)
    core.RegisterReOrgOrderCb(ReOrgOrder)
    core.RegisterWaitStopCb(WaitStop)
    
    CancelOrders = list.New()

    Match = false
 
    InitTrade()
    InitOb()
    InitUnit()

    InitMatchNonce()
    Init_startmatch()
    Init_timeout()

    Init_match_res()
    Init_quit()
    
    EXCHANGE_DEFAULT_FEE,_ = decimal.NewFromString("0.003") //exchange fee: 3/1000

    ReOrgList = common.NewQueue()
}

func InitTrade() {
    types.AllSupportedTrades = append(types.AllSupportedTrades,"ETH/BTC")
    types.AllSupportedTrades = append(types.AllSupportedTrades,"FSN/BTC")
    types.AllSupportedTrades = append(types.AllSupportedTrades,"FSN/ETH")
}

func InitOb() {
    for _,trade := range types.AllSupportedTrades {
	OB.WriteMap(trade,orderbook.NewXvcOrderBook())
    }
}

func InitUnit() {
    var dat types.CoinSupport
    if err := json.Unmarshal([]byte(types.CoinStatus), &dat); err == nil {
	for _, coin := range dat.COINS {
	    unit,_ := decimal.NewFromString(coin.UNIT)
	    //log.Info("=========InitUnit===========","coin",coin.COIN,"unit",unit.String())
	    UNIT.WriteMap(coin.COIN,unit)
	}
    }

    ///add FSN
    unit,_ := decimal.NewFromString("1000000000000000000")
    UNIT.WriteMap("FSN",unit)
}

func InitMatchNonce() {
    for _,trade := range types.AllSupportedTrades {
	tmp,_ := new(big.Int) .SetString("0",10)
	MatchNonce.WriteMap(trade,tmp)
    }
}

func Init_startmatch() {
    for _,trade := range types.AllSupportedTrades {
	tmp := make(chan bool, 1)
	startmatch.WriteMap(trade,tmp)
    }
}

func Init_timeout() {
    for _,trade := range types.AllSupportedTrades {
	tmp := make(chan bool, 1)
	timeout.WriteMap(trade,tmp)
    }
}

func Init_match_res() {
    for _,trade := range types.AllSupportedTrades {
	tmp := make(chan MatchResData, 500000)
	match_res.WriteMap(trade,tmp)
    }
}

func Init_quit() {
    for _,trade := range types.AllSupportedTrades {
	tmp := make(chan bool, 1)
	quit.WriteMap(trade,tmp)
    }
}

func AddNewTrade(trade string,local bool) error {
    //log.Info("===========AddNewTrade=============","trade",trade,"local",local)
    if trade == "" {
	return errors.New("add new trade fail,param error.")
    }

    _,_,err := GetUnit(trade)
    if err != nil {
	log.Info("===============AddNewTrade,add new trade fail,get coin unit fail.===============",)
	return errors.New("add new trade fail,get coin unit fail.")
    }

    for _,tr := range types.AllSupportedTrades {
	if strings.EqualFold(tr,trade) {
	    //log.Info("===============AddNewTrade,already support trade.===============",)
	    return errors.New("already support trade.")
	}
    }
    types.AllSupportedTrades = append(types.AllSupportedTrades,trade)

    if !dcrm.IsInXvcGroup() || !local { //non-xvc-group dont need orderbook,but need AllSupportedTrades array.
	return nil
    }
    
    OB.WriteMap(trade,orderbook.NewXvcOrderBook())
    zero,_ := new(big.Int) .SetString("0",10)
    MatchNonce.WriteMap(trade,zero)
    tmp := make(chan bool, 1)
    startmatch.WriteMap(trade,tmp)
    tmp = make(chan bool, 1)
    timeout.WriteMap(trade,tmp)
    data := make(chan MatchResData, 500000)
    match_res.WriteMap(trade,data)
    tmp = make(chan bool, 1)
    quit.WriteMap(trade,tmp)

    MatchTrade = append(MatchTrade,trade)
    go StartXvcMatch(trade)
    go SendMatchResTx(trade)
    
    return nil
}

func GetDefaultFee() decimal.Decimal {
    return EXCHANGE_DEFAULT_FEE
}

func GetOB(trade string) *orderbook.XvcOrderBook {
    if trade == "" {
	return nil
    }

    tmp,exsit := OB.ReadMap(trade)
    if tmp == nil || !exsit {
	return nil
    }

    return tmp.(*orderbook.XvcOrderBook)
}

//trade = A/B
func GetABByTrade(trade string) (string,string) {
    if trade == "" {
	return "",""
    }

    s := strings.Split(trade,"/")

    A := s[0]
    B := s[1]

    //check
    exsit := false
    erc20_handler := cryptocoins.NewCryptocoinHandler(A)
    if erc20_handler != nil {
	exsit = true
    } else {
	for _,cointype := range types.AllSupportedCointypes {
	    if strings.EqualFold(cointype,A) {
		exsit = true
		break
	    }
	}
    }

    if !exsit {
	return "",""
    }

    exsit = false
    erc20_handler = cryptocoins.NewCryptocoinHandler(B)
    if erc20_handler != nil {
	exsit = true
    } else {
	for _,cointype := range types.AllSupportedCointypes {
	    if strings.EqualFold(cointype,B) {
		exsit = true
		break
	    }
	}
    }
    
    if !exsit {
	return "",""
    }

    return A,B
}

//trade: A/B
//return: U_A,U_B
//1A = U_A*e
//1B = U_B*e
// 1A = UA*unit for example: 1BTC = 10^8 cong
func GetUnit(trade string) (decimal.Decimal,decimal.Decimal,error) {

    A,B := GetABByTrade(trade)
    if A == "" || B == "" {
	return decimal.Zero,decimal.Zero,dcrm.GetRetErr(dcrm.ErrGetTradeUnitFail)
    }

    Ua,exsit := UNIT.ReadMap(A)
    if !exsit {
	log.Info("===========GetUnit,Ua is not exsit.==============")
	return decimal.Zero,decimal.Zero,dcrm.GetRetErr(dcrm.ErrGetTradeUnitFail)
    }
    
    Ub,exsit := UNIT.ReadMap(B)
    if !exsit {
	log.Info("===========GetUnit,Ub is not exsit.==============")
	return decimal.Zero,decimal.Zero,dcrm.GetRetErr(dcrm.ErrGetTradeUnitFail)
    }

    return Ua.(decimal.Decimal),Ub.(decimal.Decimal),nil
}

func UpdateOrder(mr *MatchRes) {
    if mr == nil {
	return
    }

    done := mr.Done
    if len(done) == 0 {
	return
    }

    fail := false
    var wg sync.WaitGroup
    
    smallest_left,_ := decimal.NewFromString("100") //0.00000001
    for _,v := range done {
	wg.Add(1)

	go func(od *orderbook.XvcOrder) {
	    defer wg.Done()
	    
	    id := od.ID()
	    ob := GetOB(od.TRADE())
	    if ob == nil {
		fail = true
		return
	    }

	    e := ob.Order(id)
	    if e == nil {
		//////////////
		curvol := mr.CurVolume
		if len(curvol) == 0 {
		    log.Info("============================!!!!!!!!!! UpdateOrder,ERROR:current volume len is 0. !!!!!!!!!!!=====================","order id",id)
		    fail = true
		    return
		}

		for _,vv := range curvol {
		    if strings.EqualFold(vv.Id,id) {
			//1
			lf,_ := decimal.NewFromString(od.QUANTITY())
			vvol,_ := decimal.NewFromString(vv.Vol)
			left := vvol.Sub(lf)
			if left.LessThan(decimal.Zero) {
			    log.Info("============================!!!!!!!!!! UpdateOrder,order not exsit in orderbootk. ERROR:current volume < done volume. !!!!!!!!!!!=====================","order id",id,"vv.Vol",vv.Vol,"od.QUANTITY()",od.QUANTITY(),"left",left.StringFixed(10))

			    fail = true
			    break
	    }
			//2
			if left.GreaterThanOrEqual(decimal.Zero) && left.LessThan(smallest_left) {
			    //log.Info("============================UpdateOrder,order not exsit in orderbootk. but not need to insert order to orderbook.=====================","order id",id,"vv.Vol",vv.Vol,"od.QUANTITY()",od.QUANTITY(),"left",left.StringFixed(10))
			    break
			}

			//3
			o := orderbook.NewXvcOrder(id,od.FROM(),od.TRADE(),od.ORDERTYPE(),od.SIDE(),od.TIME(),left.StringFixed(10),od.PRICE(),od.RULE())
			ob.InsertToOrderBook(o)
			//log.Info("============================UpdateOrder,order not exsit in orderbootk. and insert order to orderbook.=====================","order id",id,"vv.Vol",vv.Vol,"od.QUANTITY()",od.QUANTITY(),"left",left.StringFixed(10))
			break
		    }
		}
		//////////////
		return
	    }

	    eee,_ := decimal.NewFromString(e.QUANTITY())
	    vvv,_ := decimal.NewFromString(od.QUANTITY())
	    diff := eee.Sub(vvv)
	    if diff.LessThan(decimal.Zero) {
		log.Info("============================!!!!!!!!!! UpdateOrder,ERROR:current volume < done volume. !!!!!!!!!!!=====================","order id",id,"old vol",e.QUANTITY(),"od vol",od.QUANTITY(),"diff",diff.StringFixed(10))

		fail = true
	    }

	    //log.Info("=========UpdateOrder============","order id",id,"old vol",e.QUANTITY(),"od vol",od.QUANTITY(),"diff",diff.StringFixed(10))
	    if diff.GreaterThanOrEqual(decimal.Zero) && diff.LessThan(smallest_left) {
		//log.Info("============UpdateOrder,diff < smallest left and cancel the order.===================","order id",id)
		
		ob.CancelOrder(id)
		//mongodb
		mongodb.UpdateOrderStatus(id, mongodb.SUCCESS)
		mongodb.RemoveOrderCache(e.PRICE(), e.TRADE(), e.SIDE())
		
		return
	    }

	    if diff.GreaterThan(decimal.Zero) {
		//log.Info("=========UpdateOrder,update order before.============","order id",e.ID(),"order vol",e.QUANTITY(),"od vol",od.QUANTITY(),"diff",diff.StringFixed(10))
	       ob.UpdateOrder(e,diff)
		return
	    }
	}(v)
    }
    
    wg.Wait()
}

func CancelOrder(mr *MatchRes) {
    if mr == nil || len(mr.Cancel) == 0 {
	return
    }

    for _,id := range mr.Cancel {
	for _,tr := range types.AllSupportedTrades {
	    ob := GetOB(tr)
	    e := ob.Order(id)
	    if e != nil {
		ob.CancelOrder(id)

		iter := CancelOrders.Front()
		for iter != nil && iter.Value != nil {
		    value := iter.Value.(string)
		    if strings.EqualFold(value,id) {
			CancelOrders.Remove(iter)
			break
		    }

		    iter = iter.Next()
		}
		
		break
	    }
	}
    }

    //TODO delete from CancelOrders
}

func GetCancelOrder() (int,[]string) {
    ress := DeepCopyCurrentCancelOrders(CancelOrders)
    num := len(ress)
    if num == 0 {
	return 0,nil
    }

    i := 0
    cancel := make([]string,num)
    for _,v := range ress {
	if v == nil {
	    continue
	}

	id := v.Value.(string)

	for _,tr := range MatchTrade {
	    ob := GetOB(tr)
	    e := ob.Order(id)
	    if e != nil {
		cancel[i] = id
		i++
		break
	    }
	}
    }

    return i,cancel
}

func print_done(ob *orderbook.XvcOrderBook,done []*orderbook.XvcOrder,trade string,nonce *big.Int) {
    if ob == nil || len(done) == 0 || trade == "" || nonce == nil {
	return
    }

    for _,vvv := range done {
	log.Info("=========print_done=============","order id",vvv.ID(),"order trade",vvv.TRADE(),"order side",vvv.SIDE(),"order price",vvv.PRICE(),"order quantity",vvv.QUANTITY(),"match nonce",nonce,"trade",trade)
	teste := ob.Order(vvv.ID())
	if teste == nil {
	    log.Info("=========print_done,order not exsit in ob.=============","order id",vvv.ID(),"match nonce",nonce,"trade",trade)
	} else {
	    log.Info("=========print_done,order exsit in ob.=============","order id",vvv.ID(),"match nonce",nonce,"trade",trade)
	}
    }
}

func MatchXvcOrder(trade string,nonce *big.Int) error {
    //log.Info("=========begin match=============","match nonce",nonce,"trade",trade)
    if trade == "" || nonce == nil {
	go MatchTimeOut(trade)
	qc,_ := quit.ReadMap(trade)
	qc2 := qc.(chan bool)
	<- qc2
	
	out,_ := timeout.ReadMap(trade)
	tout := out.(chan bool)
	tout <-true
	return dcrm.GetRetErr(ErrOrderBookObjectNil)
    }

    ob := GetOB(trade)
    /////////////////////////////
    //d := DeepCopy(ob,trade,nonce) 
    d := DeepCopy_Tmp(ob,trade,nonce)
    /////////////////////////////
    
    //log.Info("=========begin match,deep copy orderbook finish.=============","match nonce",nonce,"trade",trade)

    price,done,volumes,orders,err := MatchMakingBidding(d)

    ///////tmp
    /*if len(done) > 800 {
	log.Info("==========begin match,done len > 800 and re-match.=================","match nonce",nonce,"trade",trade)
	RemoveOb_Tmp(d)
	d = DeepCopy_Tmp(d,ob,trade,nonce)
	price,done,volumes,orders,err = MatchMakingBidding(d)
    }*/
    //////

    ///////for test only
    //log.Info("=========begin match,match finish.=============","err",err,"done len",len(done),"match nonce",nonce,"trade",trade)

    //go print_done(ob,done,trade,nonce)
    ////////////////////

    if err != nil || len(done) == 0 {
	count,cancel := GetCancelOrder()
	if count == 0 || cancel == nil {
	    RemoveOb(d)
	    go MatchTimeOut(trade)
	    qc,_ := quit.ReadMap(trade)
	    qc2 := qc.(chan bool)
	    <- qc2
	    out,_ := timeout.ReadMap(trade)
	    tout := out.(chan bool)
	    tout <-true
	    return nil
	}
	
	RemoveOb(d)
	//log.Info("=========begin match,match fail,but cancel order success.=============","match nonce",nonce,"trade",trade)
	matchres := &MatchRes{Cancel:cancel}
	CancelOrder(matchres)
	//log.Info("=========begin match,update orderbook finish.=============","match nonce",nonce,"trade",trade)
	start,_ := startmatch.ReadMap(trade)
	startm := start.(chan bool)
	startm <-true

	data := MatchResData{Nonce:nonce,Trade:trade,Mr:matchres}
	res,_ := match_res.ReadMap(trade)
	mres := res.(chan MatchResData)
	mres <- data
	return nil
    }

    //log.Info("=========begin match,match success.=============","match nonce",nonce,"trade",trade)

    fail := false
    var wg sync.WaitGroup
    
    curvol := make([]*CurrentVolume,len(done))
    for k,v := range done {
	wg.Add(1)
	go func(index int,od *orderbook.XvcOrder) {
	    defer wg.Done()
	    
	    id := od.ID()
	    e := ob.Order(id)
	    if e == nil {
		count,cancel := GetCancelOrder()
		if count == 0 || cancel == nil {
		    RemoveOb(d)
		    //log.Info("==============begin match,get current vol fail and no cancel.===============","order id",id,"match nonce",nonce,"trade",trade)
		    go MatchTimeOut(trade)
		    qc,_ := quit.ReadMap(trade)
		    qc2 := qc.(chan bool)
		    <- qc2
		    out,_ := timeout.ReadMap(trade)
		    tout := out.(chan bool)
		    tout <-true

		    fail = true
		    return
		}
		
		RemoveOb(d)
		//log.Info("=========begin match,get current vol fail,but cancel order success.=============","order id",id,"match nonce",nonce,"trade",trade)
		matchres := &MatchRes{Cancel:cancel}
		CancelOrder(matchres)
		    log.Info("=========begin match,update orderbook finish.=============","match nonce",nonce,"trade",trade)
		    start,_ := startmatch.ReadMap(trade)
		    startm := start.(chan bool)
		    startm <-true

		data := MatchResData{Nonce:nonce,Trade:trade,Mr:matchres}
		res,_ := match_res.ReadMap(trade)
		mres := res.(chan MatchResData)
		mres <- data

		fail = true
		return
	    }

	    curvol[index] = &CurrentVolume{Id:id,Vol:e.QUANTITY()}
	}(k,v)
    }
    ////
    wg.Wait()

    if fail == true {
	//log.Info("=========begin match,get current vol fail.=============","match nonce",nonce,"trade",trade)
	return nil
    }

    //log.Info("=========begin match,get current vol success.=============","match nonce",nonce,"trade",trade)
    
    RemoveOb(d)
    matchres := &MatchRes{Price:price.StringFixed(10),Done:done,Volumes:volumes.StringFixed(10),Orders:orders,CurVolume:curvol}
   
    UpdateOrder(matchres)
    ress := DeepCopyCurrentCancelOrders(CancelOrders)
    if len(ress) != 0 {
	matchres.Cancel = make([]string,len(ress))
	i := 0
	for _,v := range ress {
	    if v == nil {
		continue
	    }

	    id := v.Value.(string)

	    for _,tr := range MatchTrade {
		ob := GetOB(tr)
		e := ob.Order(id)
		if e != nil {
		    matchres.Cancel[i] = id
		    i++
		    break
		}
	    }
	}
    }
    CancelOrder(matchres)
	//log.Info("=========begin match,update orderbook finish.=============","match nonce",nonce,"trade",trade)
    start,_ := startmatch.ReadMap(trade)
    startm := start.(chan bool)
    startm <-true

    data := MatchResData{Nonce:nonce,Trade:trade,Mr:matchres}
    RevertMatchResRealData(data.Mr)
    res,_ := match_res.ReadMap(trade)
    mres := res.(chan MatchResData)
    mres <- data
    
    curvolString := []string{}
    for i := 0; i < len(curvol); i++ {
	    curvolString = append(curvolString, fmt.Sprintf("%v", (data.Mr.CurVolume)[i].Vol))
    }
    //mongodb
    //mongodb.MgoUpdateOrderAndCaches(data.Mr.Done, curvolString)

    return nil
}

type MatchResData struct {
    Nonce *big.Int 
    Trade string
    Mr *MatchRes
}

func RevertMatchResRealData2(mr *MatchRes) {
    if mr == nil {
	return
    }

    ut,_ := decimal.NewFromString(dcrm.UT)
    for _,v := range mr.Done {
	p,_ := decimal.NewFromString(v.PRICE())
	q,_ := decimal.NewFromString(v.QUANTITY())
	p = p.Mul(ut)
	q = q.Mul(ut)
	v.Price = p.StringFixed(10)
	v.Quantity = q.StringFixed(10)
    }
	
    price,_ := decimal.NewFromString(mr.Price)
    price = price.Mul(ut)
    mr.Price = price.StringFixed(10)
	
    vol,_ := decimal.NewFromString(mr.Volumes)
    vol = vol.Mul(ut)
    mr.Volumes = vol.StringFixed(10)

    for _,v := range mr.CurVolume {
	q,_ := decimal.NewFromString(v.Vol)
	q = q.Mul(ut)
	//mr.CurVolume[k] = &CurrentVolume{Id:v.Id,Vol:q.String()}
	v.Vol = q.StringFixed(10)
    }
}

func RevertMatchResRealData(mr *MatchRes) {
    if mr == nil {
	return
    }

    ut,_ := decimal.NewFromString(dcrm.UT)
    for _,v := range mr.Done {
	p,_ := decimal.NewFromString(v.PRICE())
	q,_ := decimal.NewFromString(v.QUANTITY())
	p = p.Div(ut)
	q = q.Div(ut)
	v.Price = p.StringFixed(10)
	v.Quantity = q.StringFixed(10)
    }
	
    price,_ := decimal.NewFromString(mr.Price)
    price = price.Div(ut)
    mr.Price = price.StringFixed(10)
	
    vol,_ := decimal.NewFromString(mr.Volumes)
    vol = vol.Div(ut)
    mr.Volumes = vol.StringFixed(10)

    for _,v := range mr.CurVolume {
	q,_ := decimal.NewFromString(v.Vol)
	q = q.Div(ut)
	//mr.CurVolume[k] = &CurrentVolume{Id:v.Id,Vol:q.String()}
	v.Vol = q.StringFixed(10)
    }
}

func ExecMatchResTx(data MatchResData) error {
    mr := data.Mr

    nonce := data.Nonce
    trade := data.Trade

    if mr != nil && nonce != nil && trade != "" {
	res_compress,err := GetZipData(mr)
	if err != nil {
	    log.Info("=========begin match,compress match result fail and re-compress.=============","err",err,"match nonce",nonce,"trade",trade)
	    return err
	}
	
	//log.Info("=========begin match,compress match result success.=============","match nonce",nonce,"trade",trade)

	err = SendOrderbookTx(res_compress,nonce,trade)
	if err != nil {
	    //log.Info("=========begin match, send match result to txpool fail and re-send.=============","err",err,"match nonce",nonce,"trade",trade)
	    return err
	}

	//log.Info("=========begin match, send match result to txpool success.=============","match nonce",nonce,"trade",trade)

	return nil
    }

    return fmt.Errorf("===============begin match, send match result to txpool fail,param error.===================")
}

func SendMatchResTx(trade string) {
    if trade == "" {
	return
    }

    res,_ := match_res.ReadMap(trade)
    mres := res.(chan MatchResData)

    for {
	select {
	case data := <- mres:
	    err := ExecMatchResTx(data)
	    if err != nil {
		times := 50
		i := 0
		for i=0;i<times;i++ {
		    time.Sleep(time.Duration(100000))  //na, 1 s = 10e9 na
		    err = ExecMatchResTx(data)
		    if err == nil {
			break
		    }
		}

	    }
	}
    }
}

func ReceivAllOrder() {

    for {
	select {
	case msg := <- all_order:
	    go InsertToOrderBook2(msg)
	}
    }
}

func MatchTimeOut(trade string) {
    if trade == "" {
	return
    }

    qc,_ := quit.ReadMap(trade)
    qc2 := qc.(chan bool)
    
    select {
    case num := <-ch:
	fmt.Println("num = ", num)
    case <-time.After(2 * time.Second):
    //case <-time.After(time.Duration(1000000)): //mine-fast
	qc2 <- true
    }
}

func StartXvcMatch(trade string) {

    one,_ := new(big.Int).SetString("1",10)

    start,exsit := startmatch.ReadMap(trade)
    if exsit == false {
	return
    }
    startm := start.(chan bool)

    out,exsit := timeout.ReadMap(trade)
    if exsit == false {
	return
    }
    tout := out.(chan bool)

    mn2,exsit := MatchNonce.ReadMap(trade)
    if exsit == false {
	return
    }
    mn := mn2.(*big.Int)

    startm <-true

     for {
	     select {
	    case <-startm:
		go MatchTimeOut(trade)
		qc,_ := quit.ReadMap(trade)
		qc2 := qc.(chan bool)
		<-qc2
		mn = new(big.Int).Add(mn,one)
		MatchNonce.WriteMap(trade,mn)
		go MatchXvcOrder(trade,mn)
	    case <-tout:
		mn = new(big.Int).Add(mn,one)
		MatchNonce.WriteMap(trade,mn)
		go MatchXvcOrder(trade,mn)
	    }
	}
}

//exchange start in gdcrm init
func Start() {
    for _,matchtrade := range MatchTrade {
	if matchtrade != "" {
	    go StartXvcMatch(matchtrade)
	    go SendMatchResTx(matchtrade)
	}
    }

    //go ReceivAllOrder()
    //go ReOrgMatchResTx()
}

func IsOrderBalanceOk(from string,trade string,side string,price decimal.Decimal,quantity decimal.Decimal,fusion decimal.Decimal,B decimal.Decimal,A decimal.Decimal,ch chan interface{}) {

    ///////tmp code/////
    res := dcrm.RpcDcrmRes{Ret:"ok",Err:nil}
    ch <- res
    return
    //////////////////

    /*a := A
    b := B
    ua,ub,err := GetUnit(trade)
    if err != nil {
	log.Debug("============IsOrderBalanceOk,get unit error.===========")
	var ret2 dcrm.Err
	ret2.Info = "get unit error"
	res := dcrm.RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return
    }

    for _,trade := range types.AllSupportedTrades {
    }

    if strings.EqualFold(trade,"ETH/BTC") == true {
	//eth-btc
	if eth_btc_ob != nil {
	    keys,values := eth_btc_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

	//fsn-btc
	if fsn_btc_ob != nil {
	    keys,values := fsn_btc_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

	//fsn-eth
	if fsn_eth_ob != nil {
	    keys,values := fsn_eth_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

    }

    if strings.EqualFold(trade,"FSN/BTC") == true {
	//eth-btc
	if eth_btc_ob != nil {
	    keys,values := eth_btc_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

	//fsn-btc
	if fsn_btc_ob != nil {
	    keys,values := fsn_btc_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

	//fsn-eth
	if fsn_eth_ob != nil {
	    keys,values := fsn_eth_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

    }

    if strings.EqualFold(trade,"FSN/ETH") == true {
	//eth-btc
	if eth_btc_ob != nil {
	    keys,values := eth_btc_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

	//fsn-btc
	if fsn_btc_ob != nil {
	    keys,values := fsn_btc_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

	//fsn-eth
	if fsn_eth_ob != nil {
	    keys,values := fsn_eth_ob.Orders.ListMap()
	    l := len(keys)
	    i := 0
	    for i = 0;i<l;i++ {
		//k := keys[i]
		v := values[i]

		side_tmp := v.Value.(*orderbook.XvcOrder).SIDE()
		from_tmp := v.Value.(*orderbook.XvcOrder).FROM()
		price_tmp := v.Value.(*orderbook.XvcOrder).PRICE()
		quantity_tmp := v.Value.(*orderbook.XvcOrder).QUANTITY()

		if !strings.EqualFold(from_tmp.Hex(),from) {
		    continue
		}

		if strings.EqualFold(side_tmp,"Buy") == true {
		    tmp := price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    b = b.Sub(tmp)

		    tmp = quantity_tmp.Mul(ua)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    a = a.Add(tmp)
		}
		if strings.EqualFold(side_tmp,"Sell") == true {
		    tmp := quantity_tmp.Mul(ua)
		    a = a.Sub(tmp)

		    tmp = price_tmp.Mul(quantity_tmp)
		    tmp = tmp.Mul(ub)
		    fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
		    tmp = tmp.Sub(fee)
		    b = b.Add(tmp)
		}
	    }
	}

    }

    if strings.EqualFold(side,"Buy") == true {
	tmp := price.Mul(quantity)
	tmp = tmp.Mul(ub)
	b = b.Sub(tmp)

	tmp = quantity.Mul(ua)
	fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
	tmp = tmp.Sub(fee)
	a = a.Add(tmp)
    }
    if strings.EqualFold(side,"Sell") == true {
	tmp := quantity.Mul(ua)
	a = a.Sub(tmp)

	tmp = price.Mul(quantity)
	tmp = tmp.Mul(ub)
	fee := tmp.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
	tmp = tmp.Sub(fee)
	b = b.Add(tmp)
    }

    if b.Sign() < 0 {
	log.Debug("============BTC balance is not ok.===========")
	res := dcrm.RpcDcrmRes{Ret:"",Err:dcrm.GetRetErr(ErrOrderInsufficient)}
	ch <- res
	return
    }

    if a.Sign() < 0 {
	log.Debug("============ETH balance is not ok.===========")
	res := dcrm.RpcDcrmRes{Ret:"",Err:dcrm.GetRetErr(ErrOrderInsufficient)}
	ch <- res
	return
    }

    res := dcrm.RpcDcrmRes{Ret:"ok",Err:nil}
    ch <- res
    return*/
}

func InsertByNonce(tx *types.Transaction) {

    iter := ReOrgList.Front()
    for iter != nil {
	d := iter.Value.(*types.Transaction)
	if d == nil {
	    iter = iter.Next()
	    break
	}

	if d.Nonce() > tx.Nonce() {
	    ReOrgList.InsertBefore(tx,iter)
	    break
	}

	iter = iter.Next()
    }

    ReOrgList.PushBack(tx)
}

func DoReSendTx() {

    iter := ReOrgList.Front()
    for iter != nil {
	d := iter.Value.(*types.Transaction)
	if d == nil {
	    iter = iter.Next()
	    break
	}

	if types.IsXvcTx(d) {
	    str := string(d.Data())
	    realtxdata,_ := types.GetRealTxData(str)
	    m := strings.Split(realtxdata,common.SepOB)

	    nonce := m[1]
	    matchnonce,_ := new(big.Int).SetString(nonce,10)
	    //res_compress := m[2]
	    trade := m[3]

	    log.Info("========DoReSendTx===========","tx nonce",d.Nonce(),"tx hash",d.Hash(),"match nonce",matchnonce,"trade",trade)

	    err := miner.TxPool().AddLocal(d)
	    if err != nil {
		log.Info("========DoReSendTx,re-send fail.===========","err",err,"tx old nonce",d.Nonce(),"tx hash",d.Hash(),"match nonce",matchnonce,"trade",trade)
	    }
	} else {
	    //TODO
	}

	iter = iter.Next()
    }

    var next *list.Element
    for e := ReOrgList.Front(); e != nil; e = next {
        next = e.Next()
        ReOrgList.Remove(e)
    }
}

func IsClosed(ch <-chan interface{}) bool {
    select {
    case <-ch:
        return true
    default:
    }
    
    return false
}

func MyClose(ch chan bool) {
    defer func() {
        if recover() != nil {
            // close(ch) panic occur
        }
    }()
    
    close(ch) // panic if ch is closed
}

func WaitStop() {
}

func ReOrgOrder(txs types.Transactions) {

    return
    
    if len(txs) == 0 {
    }

    log.Info("================ReOrgOrder 1111======================")

    for _,tx := range txs {
	if tx == nil {
	    continue
	}

	err := miner.TxPool().AddLocal(tx)
	if err != nil {
	    log.Info("========ReOrgOrder,re-send fail.===========","err",err,"tx nonce",tx.Nonce(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	}
    }

    /*acc := make(map[common.Address]bool)
    for _,tx := range txs {
	if tx == nil {
	    continue
	}

	from,err := miner.TxPool().GetFrom(tx)
	if err != nil {
	    continue
	}

	acc[from] = true
    }

    log.Info("================ReOrgOrder 22222======================")

    for addr,_ := range acc {
	nonce := uint64(miner.TxPool().State().GetNonce(addr))
	qn,err := miner.TxPool().GetDcrmTxRealNonce(addr.Hex())
	if err == nil {
	    nonce = nonce + uint64(qn)
	}

	for _,tx := range txs {
	    if tx == nil {
		continue
	    }

	    from,err := miner.TxPool().GetFrom(tx)
	    if err != nil {
		continue
	    }
	    
	    if strings.EqualFold(from.Hex(),addr.Hex()) == false {
	        continue
	    }

	    InsertByNonce(tx)
	}
	
	DoReSendTx()
    }*/

    log.Info("================ReOrgOrder 33333======================")
}

func InsertToOrderBook2(msg string) {
    m := strings.Split(msg,common.Sep9)
    if len(m) < 10 {
	log.Debug("data for create order error")
	return
    }

    id := m[0]
    ob := GetOB(m[3])
    if ob == nil {
	return
    }

    ////TODO
    if ob.Orders.MapLength() >= 10000000 {
    	log.Info("===========InsertToOrderBook,insert order timeout.==============")
	return
    }
    /////

    tt,_ := new(big.Int).SetString(m[9],10)
    timestamp := tt.Int64()
    from := common.HexToAddress(m[2])
    _,err := decimal.NewFromString(m[7])
    if err != nil {
	return
    }
    _,err = decimal.NewFromString(m[6])
    if err != nil {
	return
    }

    //////TODO
    if Match == false {
	return
    }

    conti := false
    for _,mt := range MatchTrade {
	if strings.EqualFold(mt,m[3]) {
	    conti = true
	    break
	}
    }

    if !conti {
	return
    }
    //////////
   
    //////////////////////////////////TODO
    e := ob.Order(id)
    if e != nil {
	return
    }
    /////////////////////////////////////

    od := orderbook.NewXvcOrder(id,from,m[3],m[4],m[5],timestamp,m[7],m[6],m[8]) //height TODO

    retOB := ob.InsertToOrderBook(od)
	if retOB == nil {
		mongodb.InsertOrderAndCache(msg)
	}

    //log.Info("==========InsertToOrderBook,","id",id,"trade",m[3],"","===============")
    return
}

func InsertToOrderBook(msg string,ch chan interface{}) error {
    m := strings.Split(msg,common.Sep9)
    if len(m) < 10 {
	log.Debug("data for create order error")
	res := dcrm.RpcDcrmRes{Ret:"",Err:dcrm.GetRetErr(dcrm.ErrInternalMsgFormatError)}
	ch <- res
	return dcrm.GetRetErr(dcrm.ErrInternalMsgFormatError)
    }

    id := m[0]
    ob := GetOB(m[3])
    if ob == nil {
	log.Debug("============InsertToOrderBook,get orderbook error.===========","id",id)
	res := dcrm.RpcDcrmRes{Ret:"",Err:dcrm.GetRetErr(ErrGetODBError)}
	ch <- res
	return dcrm.GetRetErr(ErrGetODBError)
    }

    ////TODO
    if ob.Orders.MapLength() >= 1000000 {
    	log.Info("===========InsertToOrderBook,insert order timeout.==============")
	return errors.New("insert order timeout.")
    }
    /////

    tt,_ := new(big.Int).SetString(m[9],10)
    timestamp := tt.Int64()
    from := common.HexToAddress(m[2])
    _,err := decimal.NewFromString(m[7])
    if err != nil {
	log.Debug(err.Error())
	var ret2 dcrm.Err
	ret2.Info = err.Error()
	res := dcrm.RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return err
    }
    _,err = decimal.NewFromString(m[6])
    if err != nil {
	log.Debug(err.Error())
	var ret2 dcrm.Err
	ret2.Info = err.Error()
	res := dcrm.RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return err 
    }

    //////TODO
    if Match == false {
	log.Debug("============InsertToOrderBook,match flag is false.===========")
	res := dcrm.RpcDcrmRes{Ret:"",Err:errors.New("do not insert.")}
	ch <- res
	return errors.New("do not insert.")
    }
    //////////
   
    //////////////////////////////////TODO
    e := ob.Order(id)
    if e != nil {
	return errors.New("order have already handle.")
    }
    /////////////////////////////////////

    od := orderbook.NewXvcOrder(id,from,m[3],m[4],m[5],timestamp,m[7],m[6],m[8]) //height TODO

    ob.InsertToOrderBook(od)

    log.Debug("==========InsertToOrderBook,","id",id,"trade",m[3],"od",od,"","===============")

    res := dcrm.RpcDcrmRes{Ret:"success",Err:nil}
    ch <- res
    return nil 
}

func RecvOrder(msg string,ch chan interface{}) {
    //all_order <- msg
    go InsertToOrderBook(msg,ch)
}

func RecvOrder2(msg string) {
    go InsertToOrderBook2(msg)
}

func GetTradeFromMatchRes(res string) string {
    if res == "" {
	return ""
    }
    
    m := strings.Split(res,common.SepOB)
    if m[0] != "ODB" {
	return ""
    }

    return m[3]
}

func DeepCopyCurrentCancelOrders(l *list.List) []*list.Element {
    if l == nil {
	return nil
    }

    length := l.Len()
    if length == 0 {
	return nil
    }

    i := 0
    res := make([]*list.Element,length)
    iter := l.Front()
    for iter != nil {
	or := iter
	if or == nil {
	    continue
	}

	res[i] = or

	if i == (length-1) {
	    break
	}

	i++
	iter = iter.Next()
    }

    return res
}

func CallOrderTest(statedb *state.StateDB) {
    log.Debug("==========CallOrderTest=================")
    if dcrm.OrderTest == "yes" {
	log.Debug("==========CallOrderTest 2222222=================")
	core.DoOrderTest(statedb)
    }
}

func InsertCancelOrder(id string) {
    if id == "" {
	return
    }
   
    for _,tr := range MatchTrade {
	ob := GetOB(tr)
	e := ob.Order(id)
	if e != nil {
	    iter := CancelOrders.Front()
	    for iter != nil && iter.Value != nil {
		value := iter.Value.(string)
		if strings.EqualFold(value,id) {
		    return
		}

		iter = iter.Next()
	    }
	    
	    CancelOrders.PushBack(id)
	    break
	}
    }
}

func UpdateOB(data string) {
    return

    if data == "" {
	return
    }

    trade := GetTradeFromMatchRes(data)

    for _,mt := range MatchTrade {
	if strings.EqualFold(mt,trade) {
	    return
	}
    }
    
    m := strings.Split(data,common.SepOB)
    if m[0] != "ODB" {
	return
    }
    mr,err := GetMatchRes(m[2]) 
    if err != nil || mr == nil {
	return
    }

    //ut,_ := decimal.NewFromString(dcrm.UT)
    RevertMatchResRealData2(mr)

    log.Debug("========UpdateOB========","match nonce",m[1],"trade",trade)
    UpdateOrder(mr)
    CancelOrder(mr)
}

func SettleOrders(evm *vm.EVM,from common.Address,data string) {
    if data == "" || evm == nil {
	return
    }

    m := strings.Split(data,common.SepOB)
    if m[0] != "ODB" {
	return
    }

    matchnonce := m[1]
    if matchnonce == "" {
	return
    }

    mr,err := GetMatchRes(m[2])
    if err != nil || mr == nil {
	return
    }

    tr := m[3]
    if tr == "" {
	return
    }

    //tmp,_ := json.Marshal(mr)
    //log.Debug("===========SettleOrders===========","settle match result",string(tmp))

    hash := crypto.Keccak256Hash([]byte(matchnonce+":"+strings.ToLower(tr))).Hex()
    evm.StateDB.SetMatchNonce(from,hash)

    price := mr.Price
    done := mr.Done
    for _,v := range done {
	from := v.FROM()
	side := v.SIDE()
	trade := v.TRADE()
	quan := v.QUANTITY()

	p,_ := decimal.NewFromString(price)
	q,_ := decimal.NewFromString(quan)
	vm.SettleOrders(evm,p,from,side,trade,q)
    }
}

func Orders(trade string) (string,error) {
    ob := GetOB(trade)
    if ob == nil {
	return "",dcrm.GetRetErr(ErrGetODBError)
    }

    return ob.String(trade)
}

func OrderDealedInfo(blockNr rpc.BlockNumber,trade string) (string,error) {
    return "",nil
}

func GetPairList() (string,error) {
    if len(types.AllSupportedTrades) <= 0 {
	return "",nil
    }

    s := types.AllSupportedTrades[0]
    for k,trade := range types.AllSupportedTrades {
	if k != 0 && trade != "" {
	    s += ":"
	    s += trade
	}
    }

    return s,nil
}

