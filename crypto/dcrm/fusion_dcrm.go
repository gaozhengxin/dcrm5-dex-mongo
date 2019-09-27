// Copyright 2018 The fusion-dcrm 
//Author: caihaijun@fusion.org

package dcrm

import (
	"math/big"
	"github.com/fusion/go-fusion/crypto/secp256k1"
	"fmt"
	"strings"
	"github.com/fusion/go-fusion/common/math"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/paillier"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/commit"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/vss"
	p2pdcrm "github.com/fusion/go-fusion/p2p/layer2"
	"github.com/fusion/go-fusion/p2p/discover"
	"os"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/ethdb"
	"github.com/fusion/go-fusion/core/types"
	"sync"
	"encoding/json"
	"strconv"
	"bytes"
	"time"
	"github.com/fusion/go-fusion/common/hexutil"
	"github.com/fusion/go-fusion/ethclient"
	"encoding/hex"
	"github.com/fusion/go-fusion/log"
	"github.com/syndtr/goleveldb/leveldb"
	"os/exec"
	"github.com/fusion/go-fusion/common/math/random"
	"github.com/fusion/go-fusion/crypto/sha3"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/schnorrZK"
	"github.com/fusion/go-fusion/crypto/dcrm/ec2/MtAZK"
	"github.com/fusion/go-fusion/accounts"
	"github.com/shopspring/decimal"
	"sort"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins/eos"
	"container/list"
	"compress/zlib"
	"io"
	"github.com/btcsuite/btcd/btcec"
	cryptocoinsconfig "github.com/fusion/go-fusion/crypto/dcrm/cryptocoins/config"
)
////////////

var (
    tmp2 string
    lock sync.Mutex
    
    FSN      Backend

    dir string//dir,_= ioutil.TempDir("", "dcrmkey")
    NodeCnt = 3
    ThresHold = 3
    PaillierKeyLength = 2048

    CHAIN_ID       = 4 //ethereum mainnet=1 rinkeby testnet=4  //TODO :get user define chainid.

    cur_enode string
    enode_cnts int 

    // 0:main net  
    //1:test net
    //2:namecoin
    bitcoin_net = 1

    //rpc-req //dcrm node
    RpcMaxWorker = 100 
    RpcMaxQueue  = 100
    RpcReqQueue chan RpcReq 
    workers []*RpcReqWorker
    //rpc-req
    
    //non dcrm node
    non_dcrm_workers []*RpcReqNonDcrmWorker
    RpcMaxNonDcrmWorker = 100
    RpcMaxNonDcrmQueue  = 100
    RpcReqNonDcrmQueue chan RpcReq 

    datadir string
    init_times = 0

    ETH_SERVER = "http://54.183.185.30:8018"
    
    erc20_client *ethclient.Client
 
    BTC_BLOCK_CONFIRMS int64
    BTC_DEFAULT_FEE float64
    ETH_DEFAULT_FEE *big.Int
    
    mergenum = 0

    //
    BLOCK_FORK_0 = "0"//"18000" //fork for dcrmsendtransaction.not to self.
    BLOCK_FORK_1 = "0"//"280000" //fork for lockin,txhash store into block.
    BLOCK_FORK_2 = "0"//"100000" //fork for lockout choose real dcrm from.

    rpcs *big.Int //add for rpc cmd prex

    splitnum = 800

    TryTimes = 100
    TryTimesForLockin = 10
    sendtogroup_lilo_timeout = 100
    sendtogroup_timeout = 80
    ch_t = 60
    txcall   func(string,string) *big.Int
    getdcrmaddrcall   func(string,string,string) string
    
    //for orderbook
    bmatchres_eth_btc chan bool
    msg_matchres_eth_btc chan string
    bmatchres_fsn_btc chan bool
    msg_matchres_fsn_btc chan string
    bmatchres_fsn_eth chan bool
    msg_matchres_fsn_eth chan string
    
    obcall   func(string,chan interface{})
    obcall2   func(string)
    checkodbalance func(string,string,string,decimal.Decimal,decimal.Decimal,decimal.Decimal,decimal.Decimal,decimal.Decimal,chan interface{})
    getmatchrescall   func(string,chan interface{})

    OrderTest string

    ReorgNum uint64//test net:10  main net:1000
    UT = "10000000000" ////add for exchange decimal--->big.Int
)

func RegisterTXCallback(recvObFunc func(string,string) *big.Int) {
	txcall = recvObFunc
}

func RegisterGetDcrmAddrCallback(recvObFunc func(string,string,string) string) {
	getdcrmaddrcall = recvObFunc
}

func GetDcrmAddrOutSideBalance(dcrmaddr string,cointype string) *big.Int {
    if dcrmaddr == "" || cointype == "" {
	return nil
    }

    return txcall(dcrmaddr,cointype)
}

func GetDcrmAddr(dcrmaddr string,cointype string,coin string) string {
    if dcrmaddr == "" || cointype == "" || coin == "" {
	return ""
    }

    return getdcrmaddrcall(dcrmaddr,cointype,coin)
}

func CanDoMatch() bool {
    ////////TODO
    ids := GetIds()
    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	if !IsCurNode(enodes,cur_enode) {
	    return false 
	} else {
	    return true
	}
    }

    return false 
    /////
}

func InsToOB(msg string) {
    obcall2(msg)
}

///////
func Encode(obj interface{}) (string,error) {
    switch obj.(type) {
    /*case *RpcDcrmRes:
	ch := obj.(*RpcDcrmRes)
	gob.Register(RpcDcrmRes{})
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)

	err1 := enc.Encode(ch)
	if err1 != nil {
	    panic(err1)
	}
	return buff.String(),nil

	ret,err := json.Marshal(ch)
	if err != nil {
	    return "",err
	}
	return string(ret),nil*/
    case *SendMsg:
	ch := obj.(*SendMsg)
	
	/*gob.Register(SendMsg{})
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)

	err1 := enc.Encode(ch)
	if err1 != nil {
	    panic(err1)
	}
	return buff.String(),nil*/

	ret,err := json.Marshal(ch)
	if err != nil {
	    return "",err
	}
	return string(ret),nil
    default:
	return "",GetRetErr(ErrEncodeSendMsgFail)
    }
}

func Decode(s string) (interface{},error) {
    /*var data bytes.Buffer
    data.Write([]byte(s))
    
    dec := gob.NewDecoder(&data)

    var res SendMsg
    err := dec.Decode(&res)
    if err != nil {
	    panic(err)
    }

    return &res,nil*/

    var m SendMsg
    err := json.Unmarshal([]byte(s), &m)
    if err != nil {
	return nil,err
    }

    return &m,nil
} 
///////

////compress
func Compress(c []byte) (string,error) {
    if c == nil {
	return "",GetRetErr(ErrParamError)
    }

    var in bytes.Buffer
    w,err := zlib.NewWriterLevel(&in,zlib.BestCompression-1)
    if err != nil {
	return "",err
    }

    w.Write(c)
    w.Close()

    s := in.String()
    return s,nil
}

////uncompress
func UnCompress(s string) (string,error) {

    if s == "" {
	return "",GetRetErr(ErrParamError)
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

func GetAllMatchRes(res string) ([]string,error) {
    return nil,nil //tmp code

    /*if res == "" {
	return nil,errors.New("error data in match res.")
    }

    m := strings.Split(res,sepob)
    if m[0] != "ODB" {
	return nil,errors.New("match res data error.")
    }
    
    trade := m[3]
    if trade == "" {
	return  nil,errors.New("error in get trade.")
    }

    timestamp := time.Now().Unix()
    tt := strconv.Itoa(int(timestamp))
    nonce := crypto.Keccak256Hash([]byte(tt)).Hex()
    mp := []string{nonce,cur_enode}
    enode := strings.Join(mp,"-")
    s0 := "GETRES"
    s1 := res
    ss := enode + common.Sep + s0 + common.Sep + s1
    fmt.Println("================send match res,code is GETRES,ss is %s==================",ss)
    SendMsgToDcrmGroup(ss)

    if  strings.EqualFold(trade,"ETH/BTC") {
	_,cherr := GetChannelValue(1000,bmatchres_eth_btc)
	if cherr != nil {
	    return nil,cherr
	}
	
	ret := make([]string,0)
	for i :=0;i<(NodeCnt-1);i++ {
	    v,cherr := GetChannelValue(1000,msg_matchres_eth_btc)
	    if cherr != nil {
		return nil,cherr
	    }

	    ret = append(ret,v)
	}
	
	return ret,nil
    } else if  strings.EqualFold(trade,"FSN/BTC") {
	_,cherr := GetChannelValue(1000,bmatchres_fsn_btc)
	if cherr != nil {
	    return nil,cherr
	}
	
	ret := make([]string,0)
	for i :=0;i<(NodeCnt-1);i++ {
	    v,cherr := GetChannelValue(1000,msg_matchres_fsn_btc)
	    if cherr != nil {
		return nil,cherr
	    }

	    ret = append(ret,v)
	}
	
	return ret,nil
    } else if  strings.EqualFold(trade,"FSN/ETH") {
	_,cherr := GetChannelValue(1000,bmatchres_fsn_eth)
	if cherr != nil {
	    return nil,cherr
	}
	
	ret := make([]string,0)
	for i :=0;i<(NodeCnt-1);i++ {
	    v,cherr := GetChannelValue(1000,msg_matchres_fsn_eth)
	    if cherr != nil {
		return nil,cherr
	    }

	    ret = append(ret,v)
	}
	
	return ret,nil
    }

    ret := make([]string,0)
    return ret,nil
    */
}

func RegisterGetMatchResCallback(recvObFunc func(string,chan interface{})) {
	getmatchrescall = recvObFunc
}

func RegisterObCallback(recvObFunc func(string,chan interface{})) {
	obcall = recvObFunc
}

func RegisterObCallback2(recvObFunc func(string)) {
	obcall2 = recvObFunc
}

func RegisterCheckOrderBalanceCB(recvObFunc func(string,string,string,decimal.Decimal,decimal.Decimal,decimal.Decimal,decimal.Decimal,decimal.Decimal,chan interface{})) {
	checkodbalance = recvObFunc
}

func IsOrderBalanceOk(from string,trade string,side string,price decimal.Decimal,quantity decimal.Decimal,fusion decimal.Decimal,A decimal.Decimal,B decimal.Decimal) (bool,error) {
    ch := make(chan interface{},1)
    checkodbalance(from,trade,side,price,quantity,fusion,B,A,ch)
    _,cherr := GetChannelValue(ch_t,ch)
    if cherr != nil {
	return false,cherr
    }

    return true,nil
}

func GetChannelValue(t int,obj interface{}) (string,error) {
    timeout := make(chan bool, 1)
    go func(timeout chan bool) {
	 time.Sleep(time.Duration(t)*time.Second) //1000 == 1s
	 timeout <- true
     }(timeout)

     switch obj.(type) {
	 case chan interface{} :
	     ch := obj.(chan interface{})
	     select {
		 case v := <- ch :
		     ret,ok := v.(RpcDcrmRes)
		     if ok == true {
			 return ret.Ret,ret.Err
			    //if ret.Ret != "" {
			//	return ret.Ret,nil
			  //  } else {
			//	return "",ret.Err
			  //  }
		     }
		 case <- timeout :
		     return "",GetRetErr(ErrGetOtherNodesDataFail)
	     }
	 case chan string:
	     ch := obj.(chan string)
	     select {
		 case v := <- ch :
			    return v,nil 
		 case <- timeout :
		     return "",GetRetErr(ErrGetOtherNodesDataFail)
	     }
	 case chan int64:
	     ch := obj.(chan int64)
	     select {
		 case v := <- ch :
		    return strconv.Itoa(int(v)),nil 
		 case <- timeout :
		     return "",GetRetErr(ErrGetOtherNodesDataFail)
	     }
	 case chan int:
	     ch := obj.(chan int)
	     select {
		 case v := <- ch :
		    return strconv.Itoa(v),nil 
		 case <- timeout :
		     return "",GetRetErr(ErrGetOtherNodesDataFail)
	     }
	 case chan bool:
	     ch := obj.(chan bool)
	     select {
		 case v := <- ch :
		    if !v {
			return "false",nil
		    } else {
			return "true",nil
		    }
		 case <- timeout :
		     return "",GetRetErr(ErrGetOtherNodesDataFail)
	     }
	 default:
	    return "",GetRetErr(ErrUnknownChType) 
     }

     return "",GetRetErr(ErrGetChValueFail)
 }

type DcrmAddrInfo struct {
    DcrmAddr string 
    FusionAccount  string 
    CoinType string
    Balance *big.Int
}

type DcrmAddrInfoWrapper struct {
    dcrmaddrinfo [] DcrmAddrInfo
    by func(p, q * DcrmAddrInfo) bool
}

func (dw DcrmAddrInfoWrapper) Len() int {
    return len(dw.dcrmaddrinfo)
}

func (dw DcrmAddrInfoWrapper) Swap(i, j int){
    dw.dcrmaddrinfo[i], dw.dcrmaddrinfo[j] = dw.dcrmaddrinfo[j], dw.dcrmaddrinfo[i]
}

func (dw DcrmAddrInfoWrapper) Less(i, j int) bool {
    return dw.by(&dw.dcrmaddrinfo[i], &dw.dcrmaddrinfo[j])
}

func MergeDcrmBalance2(account string,from string,to string,value *big.Int,cointype string,res chan bool) {
    res <-false 
    return 
    /*if strings.EqualFold(cointype,"ETH") || strings.EqualFold(cointype,"BTC") {
	    va := fmt.Sprintf("%v",value)
	    v := DcrmLockout{Txhash:"xxx",Tx:"xxx",FusionFrom:"xxx",DcrmFrom:"xxx",RealFusionFrom:account,RealDcrmFrom:from,Lockoutto:to,Value:va,Cointype:cointype}
	    retva,err := Validate_Lockout(&v)
	    if err != nil || retva == "" {
		    log.Debug("=============MergeDcrmBalance,send tx fail.==============")
		    res <-false 
		    return 
	    }
	    
	    retvas := strings.Split(retva,":")
	    if len(retvas) != 2 {
		res <-false 
		return
	    }
	    hashkey := retvas[0]
	    realdcrmfrom := retvas[1]
	   
	    //TODO
	    vv := DcrmLockin{Tx:"xxx"+"-"+va+"-"+cointype,LockinAddr:to,Hashkey:hashkey,RealDcrmFrom:realdcrmfrom}
	    if _,err = Validate_Txhash(&vv);err != nil {
		    log.Debug("=============MergeDcrmBalance,validate fail.==============")
		res <-false 
		return
	    }

	    res <-true
	    return
	}*/
}

func MergeDcrmBalance(account string,from string,to string,value *big.Int,cointype string) {
    if account == "" || from == "" || to == "" || value == nil || cointype == "" {
	return
    }

    count := 0
    for {
	count++
	if count == 400 {
	    return
	}
	
	res := make(chan bool, 1)
	go MergeDcrmBalance2(account,from,to,value,cointype,res)
	ret,cherr := GetChannelValue(ch_t,res)
	if cherr != nil {
	    return	
	}

	if ret != "" {
	    mergenum++
	    return
	}
	 
	time.Sleep(time.Duration(10)*time.Second) //1000 == 1s
    }
}

func ChooseRealFusionAccountForLockout(amount string,lockoutto string,cointype string) (string,string,error) {
	/*chandler := cryptocoins.NewCryptocoinHandler(cointype)
	if chandler == nil {
		return "", "", fmt.Errorf("coin type not support") // not supposed
	} else {
		var dai []DcrmAddrInfo
		lock.Lock()
		dbpath := GetDbDir()
		db, err := leveldb.OpenFile(dbpath, nil) 
		if err != nil { 
			log.Debug("===========ChooseRealFusionAccountForLockout,ERROR: Cannot Open LevelDB.","dbpath",dbpath,"error",err.Error(),"","================")
		    lock.Unlock()
		    return "","",err
		} 
	    
		var b bytes.Buffer 
		b.WriteString("") 
		b.WriteByte(0) 
		b.WriteString("") 
		iter := db.NewIterator(nil, nil) 
		for iter.Next() { 
		    key := string(iter.Key())
		    value := string(iter.Value())
		    s := strings.Split(value,common.Sep)
		    log.Debug("===========ChooseRealFusionAccountForLockout,","key",key,"","===============")
		    if len(s) != 0 {
			log.Debug("=============ChooseRealFusionAccountForLockout,1111111============================")

			var m AccountListInfo
			ok := json.Unmarshal([]byte(s[0]), &m)
			if ok == nil {
			    log.Debug("=============ChooseRealFusionAccountForLockout,2222222===========================")
			    ////
			} else {
			    //dcrmaddrs := []rune(key)
			    av := cryptocoins.NewDcrmAddressValidator(cointype)
			    if match := av.IsValidAddress(key); match == true {
				    log.Debug("=============ChooseRealFusionAccountForLockout,333333333===========================")
				    // bug: ETH BTC share same addresses
				    var m DcrmAddrInfo
				    m.DcrmAddr = key
				    m.FusionAccount = s[0]
				    m.CoinType = cointype
				    jsonstring := ""//`{"tokenType":"` + cointype + `"}` //erc20
				    bal, err := chandler.GetAddressBalance(key,jsonstring)
				    if err != nil {
					    log.Debug("===========ChooseRealFusionAccountForLockout,","err",err.Error(),"","===============")
					    continue
				    }

				    log.Debug("=============ChooseRealFusionAccountForLockout,55555===========================", "bal", bal)
				    // case1: coin eg. ETH, bal is {CoinBalance:{Cointype:"ETH",Val:100},TokenBalance:{Cointype:"",Val:0}}
				    // case2: token eg. ERC20BNB, bal is {CoinBalance:{Cointype:"ETH",Val:100},TokenBalance:{Cointype:"ERC20BNB":"",Val:0}}

				    if chandler.IsToken() == false {
					    // case1: coin
					    if strings.EqualFold(bal.CoinBalance.Cointype, cointype) == false {
						    log.Debug("=============ChooseRealFusionAccountForLockout,balance cointype error===========================")
						    continue
					    }
					    m.Balance = bal.CoinBalance.Val
				    } else {
					    // case2: token
					    if strings.EqualFold(bal.TokenBalance.Cointype, cointype) == false {
						    log.Debug("=============ChooseRealFusionAccountForLockout,balance cointype error===========================")
						    continue
					    }
					    m.Balance = bal.TokenBalance.Val
				    }

				    if m.Balance == nil {
					    log.Debug("=============ChooseRealFusionAccountForLockout, m.Balance is nil===========================")
					    continue
				    }

				    //m.Balance = bal
				    dai = append(dai,m)
				    log.Debug("===========ChooseRealFusionAccountForLockout","dai",dai,"","===============")
				    sort.Sort(DcrmAddrInfoWrapper{dai, func(p, q *DcrmAddrInfo) bool {
					return q.Balance.Cmp(p.Balance) <= 0 //q.Age < p.Age
				    }})

				    defaultFee := chandler.GetDefaultFee()
				    va,_ := new(big.Int).SetString(amount,10)
				    if chandler.IsToken() == false {
					    // case1: coin
					    if strings.EqualFold(defaultFee.Cointype, cointype) == false || defaultFee.Val == nil {
						    log.Debug("default fee error", "default fee", defaultFee)
						    continue
					    }
					    total := new(big.Int).Add(va,ETH_DEFAULT_FEE)
					    if bal.CoinBalance.Val.Cmp(total) >= 0 {
						    iter.Release()
						    db.Close()
						    //cancel()
						    lock.Unlock()
						    return s[0],key,nil
					    }
				    } else {
					    // case2: token
					    if strings.EqualFold(defaultFee.Cointype, bal.CoinBalance.Cointype) == false || defaultFee.Val == nil {
						    log.Debug("default fee error", "default fee", defaultFee)
						    continue
					    }
					    if bal.CoinBalance.Val.Cmp(defaultFee.Val) >= 0 && bal.TokenBalance.Val.Cmp(va) >= 0 {
						    iter.Release()
						    db.Close()
						    //cancel()
						    lock.Unlock()
						    return s[0],key,nil
					    }
				    }
			}
		}
		}
		}

		if len(dai) < 1 {
		    iter.Release() 
		    db.Close() 
		    lock.Unlock()
		    log.Debug("===========ChooseRealFusionAccountForLockout","","There's no proper account to do lockout.")
		    return "","",GetRetErr(ErrNoGetLOAccout)
		}
		
		mergenum = 0
		count := 0
		var fa string
		var fn string
		for i,v := range dai {
		    if i == 0 {
			fn = v.FusionAccount
			fa = v.DcrmAddr
			//bn = v.Balance
		    } else {
		    }
		}

		log.Debug("===========ChooseRealFusionAccountForLockout","fa",fa,"fn",fn)

		////
		times := 0
		for {
		    times++
		    if times == 400 {
			iter.Release() 
			db.Close() 
			lock.Unlock()
			log.Debug("AAAAAAAAAAA ChooseRealFusionAccountForLockout")
			return "","",GetRetErr(ErrNoGetLOAccout)
		    }

		    if mergenum == count {
			iter.Release() 
			db.Close() 
			lock.Unlock()
			log.Debug("BBBBBBBBBBB ChooseRealFusionAccountForLockout")
			return fn,fa,nil
		    }
		     
		    time.Sleep(time.Duration(10)*time.Second) //1000 == 1s
		}
		////
		
		iter.Release() 
		db.Close() 
		lock.Unlock()
	}

    return "","",GetRetErr(ErrNoGetLOAccout)*/

    chandler := cryptocoins.NewCryptocoinHandler(cointype)
    if chandler == nil {
	return "", "", fmt.Errorf("coin type not support") // not supposed
    }

    var dai []DcrmAddrInfo
    lock.Lock()
    dbpath := GetDbDir()
    db, err := leveldb.OpenFile(dbpath, nil) 
    if err != nil { 
	log.Debug("===========ChooseRealFusionAccountForLockout,ERROR: Cannot Open LevelDB.","dbpath",dbpath,"error",err.Error(),"","================")
	lock.Unlock()
	return "","",err
    } 

    var b bytes.Buffer 
    b.WriteString("") 
    b.WriteByte(0) 
    b.WriteString("") 
    iter := db.NewIterator(nil, nil) 
    for iter.Next() { 
	key := string(iter.Key())
	value := string(iter.Value())
	s := strings.Split(value,common.Sep)
	log.Debug("===========ChooseRealFusionAccountForLockout,","key",key,"","===============")
	if len(s) == 0 {
	    continue
	}
	
	av := cryptocoins.NewDcrmAddressValidator(cointype)
	if match := av.IsValidAddress(key); match != true {
	    continue
	}

	var m DcrmAddrInfo
	m.DcrmAddr = key
	m.FusionAccount = s[0]
	m.CoinType = cointype
	
	m.Balance = GetDcrmAddrOutSideBalance(key,cointype)
	if m.Balance == nil {
	    continue
	}
	
	dai = append(dai,m)
	log.Debug("===========ChooseRealFusionAccountForLockout","dai",dai,"","===============")
	sort.Sort(DcrmAddrInfoWrapper{dai, func(p, q *DcrmAddrInfo) bool {
	    return q.Balance.Cmp(p.Balance) <= 0 //q.Age < p.Age
	}})

	v := chandler.GetDefaultFee()
	va,_ := new(big.Int).SetString(amount,10)
	if chandler.IsToken() == false {
	    if strings.EqualFold(v.Cointype, cointype) == false || v.Val == nil {
		    log.Debug("default fee error", "default fee", v)
		    continue
	    }

	    total := new(big.Int).Add(va,v.Val)
	    if m.Balance.Cmp(total) >= 0 {
		    iter.Release()
		    db.Close()
		    lock.Unlock()
		    return s[0],key,nil
	    }
	} else {
	    chandlertmp := cryptocoins.NewCryptocoinHandler(v.Cointype)
	    if chandlertmp == nil || v.Val == nil {
		log.Debug("default fee error", "default fee", v)
		    continue
	    }

	    tmp := GetDcrmAddr(key,cointype,v.Cointype)
	    tmp2 := GetDcrmAddrOutSideBalance(tmp,v.Cointype)
	    if tmp2 != nil && tmp2.Cmp(v.Val) >= 0 && m.Balance.Cmp(va) >= 0 {
		    iter.Release()
		    db.Close()
		    lock.Unlock()
		    return s[0],key,nil
	    }
	}
    }

    if len(dai) < 1 {
	iter.Release() 
	db.Close() 
	lock.Unlock()
	log.Debug("===========ChooseRealFusionAccountForLockout","","There's no proper account to do lockout.")
	return "","",GetRetErr(ErrNoGetLOAccout)
    }

    mergenum = 0
    count := 0
    var fa string
    var fn string
    for i,v := range dai {
	if i == 0 {
	    fn = v.FusionAccount
	    fa = v.DcrmAddr
	    //bn = v.Balance
	} else {
	}
    }

    log.Debug("===========ChooseRealFusionAccountForLockout","fa",fa,"fn",fn)

    ////
    times := 0
    for {
	times++
	if times == 400 {
	    iter.Release() 
	    db.Close() 
	    lock.Unlock()
	    log.Debug("AAAAAAAAAAA ChooseRealFusionAccountForLockout")
	    return "","",GetRetErr(ErrNoGetLOAccout)
	}

	if mergenum == count {
	    iter.Release() 
	    db.Close() 
	    lock.Unlock()
	    log.Debug("BBBBBBBBBBB ChooseRealFusionAccountForLockout")
	    return fn,fa,nil
	}
	 
	time.Sleep(time.Duration(10)*time.Second) //1000 == 1s
    }
    ////
    
    iter.Release() 
    db.Close() 
    lock.Unlock()

    return "","",GetRetErr(ErrNoGetLOAccout)
}

func IsValidFusionAddr(s string) bool {
    if s == "" {
	return false
    }

    fusions := []rune(s)
    if string(fusions[0:2]) == "0x" && len(fusions) != 42 { //42 = 2 + 20*2 =====>0x + addr
	return false
    }
    if string(fusions[0:2]) != "0x" {
	return false
    }

    return true
}

func IsValidDcrmAddr(s string,cointype string) bool {
    av := cryptocoins.NewDcrmAddressValidator(cointype)
    return av.IsValidAddress(s)
}

type Backend interface {
	//BlockChain() *core.BlockChain
	//TxPool() *core.TxPool
	Etherbase() (eb common.Address, err error)
	ChainDb() ethdb.Database
	AccountManager() *accounts.Manager
}

func SetBackend(e Backend) {
    FSN = e
}

func AccountManager() *accounts.Manager  {
    return FSN.AccountManager()
}

func ChainDb() ethdb.Database {
    if FSN == nil {
	return nil
    }

    return FSN.ChainDb()
}

func Coinbase() (eb common.Address, err error) {
    return FSN.Etherbase()
}

func SendReqToGroup(msg string,rpctype string) (string,error) {
    var req RpcReq
    switch rpctype {
	case "rpc_req_dcrmaddr":
	    m := strings.Split(msg,common.Sep9)
	    v := ReqAddrSendMsgToDcrm{Fusionaddr:m[0],Pub:m[1],Cointype:m[2]}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "rpc_confirm_dcrmaddr":
	    m := strings.Split(msg,common.Sep9)
	    v := ConfirmAddrSendMsgToDcrm{Txhash:m[0],Tx:m[1],FusionAddr:m[2],DcrmAddr:m[3],Hashkey:m[4],Cointype:m[5]}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}	
	case "rpc_lockin":
	    m := strings.Split(msg,common.Sep9)
	    v := LockInSendMsgToDcrm{Txhash:m[0],Tx:m[1],Fusionaddr:m[2],Hashkey:m[3],Value:m[4],Cointype:m[5],LockinAddr:m[6],RealDcrmFrom:m[7]}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "rpc_lockout":
	    m := strings.Split(msg,common.Sep9)
	    v := LockoutSendMsgToDcrm{Txhash:m[0],Tx:m[1],FusionFrom:m[2],DcrmFrom:m[3],RealFusionFrom:m[4],RealDcrmFrom:m[5],Lockoutto:m[6],Value:m[7],Cointype:m[8]}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "rpc_get_real_account":
	    m := strings.Split(msg,common.Sep9)
	    v := LockoutGetRealAccount{Txhash:m[0],Tx:m[1],FusionFrom:m[2],DcrmFrom:m[3],RealFusionFrom:m[4],RealDcrmFrom:m[5],Lockoutto:m[6],Value:m[7],Cointype:m[8]}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "rpc_order_create":
	    v := OrderCreateSendMsgToXvc{Msg:msg}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "get_matchres":
	    v := GetMatchResSendMsgToXvc{Msg:msg}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "get_dcrmgroup_count":
	    v := GetDcrmGroupCountSendMsgToXvc{Msg:msg}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	case "get_all_matchres": //???
	    v := GetAllMatchResSendMsgToXvc{Msg:msg}
	    rch := make(chan interface{},1)
	    req = RpcReq{rpcdata:&v,ch:rch}
	default:
	    return "",nil
    }

    var t int
    if rpctype == "rpc_lockout" || rpctype == "rpc_lockin" {
	t = sendtogroup_lilo_timeout 
    } else {
	t = sendtogroup_timeout
    }

    if rpctype == "rpc_order_create" || rpctype == "get_matchres" || rpctype == "get_all_matchres" {
	if !IsInXvcGroup() {
	    RpcReqNonDcrmQueue <- req
	} else {
	    RpcReqQueue <- req
	}
    } else {
	if !IsInGroup() {
	    RpcReqNonDcrmQueue <- req
	} else {
	    RpcReqQueue <- req
	}
    }
    chret,cherr := GetChannelValue(t,req.ch)
    if cherr != nil {
	log.Debug("=============SendReqToGroup fail","rpc type",rpctype,"msg",msg,"return error",cherr.Error(),"chret",chret,"","==============")
	return chret,cherr
    }

    log.Debug("SendReqToGroup success","rpc type",rpctype,"msg",msg,"return",chret)
    return chret,nil
}

//msg = hash-enode:C1:X1:X2......
//send msg = hash-enode-C1|1|2|hash-enode:C1:
//send msg = hash-enode-C1|2|2|X1:X2......
func SendMsgToDcrmGroup(msg string) {
    log.Debug("===========SendMsgToDcrmGroup=============","msg len",len(msg),"msg",new(big.Int).SetBytes([]byte(msg)))
    //va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(msg)))
    /*timestamp := time.Now().Unix()
    tt := strconv.Itoa(int(timestamp))
    nonce := crypto.Keccak256Hash([]byte(tt)).Hex()

    sm := &SendMsg{MsgType:"ec2_data",Nonce:nonce,WorkId:0,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug("===========SendMsgToDcrmGroup=============","err",err)
	return
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug("===========SendMsgToDcrmGroup=============","err",err)
	return
    }*/

    p2pdcrm.SendMsg(msg)
    return

    /*log.Debug("SendMsgToDcrmGroup","msg",msg)
    mm := strings.Split(msg,common.Sep)
    if len(mm) < 3 {
	return //TODO
    }

    tmp := "dcrmchjsendmsg"
    rs := []byte(msg)
    
    prex := mm[0] + "-" + mm[1]
    log.Debug("SendMsgToDcrmGroup","prex",prex)

    if len(rs) > splitnum && splitnum != 0 {
	q1 := (len(rs)/splitnum)
	q2 := (len(rs)%splitnum)

	n := q1
	if q2 != 0 {
	    n++
	}

	log.Debug("SendMsgToDcrmGroup","len msg",len(msg),"splitnum",splitnum,"q1",q1,"q2",q2,"n",n,"len rs",len(rs))
	i := 0
	for i = 1;i<=q1;i++ {
	    st := (i-1)*splitnum
	    ed := i*splitnum
	    p := prex + tmp + strconv.Itoa(i) + tmp + strconv.Itoa(n) + tmp + string(rs[st:ed])
	    log.Debug("===========SendMsgToDcrmGroup=============","i",i,"msg len",len(p),"msg",new(big.Int).SetBytes([]byte(p)))
	    va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(p)))
	    p2pdcrm.SendMsg(va)
	}

	if q2 != 0 {
	    st := q1*splitnum
	    log.Debug("SendMsgToDcrmGroup","st",st,"len rs",len(rs))
	    p := prex + tmp + strconv.Itoa(n) + tmp + strconv.Itoa(n) + tmp + string(rs[st:])
	    log.Debug("===========SendMsgToDcrmGroup=============","i",i,"msg len",len(p),"msg",new(big.Int).SetBytes([]byte(p)))
	    va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(p)))
	    p2pdcrm.SendMsg(va)
	}

    } else {
	    msg = prex + tmp + "1" + tmp + "1" + tmp + msg
	    log.Debug("===========SendMsgToDcrmGroup=============","msg len",len(msg),"msg",new(big.Int).SetBytes([]byte(msg)))
	    va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(msg)))
	    p2pdcrm.SendMsg(va)
    }*/
}

//msg = hash-enode:C1:X1:X2......
//send msg = hash-enode-C1|1|2|hash-enode:C1:
//send msg = hash-enode-C1|2|2|X1:X2......
func SendMsgToPeer(enodes string,msg string) {
    log.Debug("===========SendMsgToPeer=============","msg len",len(msg),"msg",new(big.Int).SetBytes([]byte(msg)))
    //va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(msg)))
    /*timestamp := time.Now().Unix()
    tt := strconv.Itoa(int(timestamp))
    nonce := crypto.Keccak256Hash([]byte(tt)).Hex()

    sm := &SendMsg{MsgType:"ec2_data",Nonce:nonce,WorkId:0,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug("===========SendMsgToPeer=============","err",err)
	return
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug("===========SendMsgToPeer=============","err",err)
	return
    }*/

    p2pdcrm.SendMsgToPeer(enodes,msg)
    return
    
    /*log.Debug("SendMsgToPeer","msg",msg)
    mm := strings.Split(msg,common.Sep)
    if len(mm) < 3 {
	return //TODO
    }

    tmp := "dcrmchjsendmsg"
    rs := []byte(msg)
    
    prex := mm[0] + "-" + mm[1]
    log.Debug("SendMsgToPeer","prex",prex)

    if len(rs) > splitnum && splitnum != 0 {
	q1 := (len(rs)/splitnum)
	q2 := (len(rs)%splitnum)

	n := q1
	if q2 != 0 {
	    n++
	}

	log.Debug("SendMsgToPeer","len msg",len(msg),"splitnum",splitnum,"q1",q1,"q2",q2,"n",n,"len rs",len(rs))
	i := 0
	for i = 1;i<=q1;i++ {
	    st := (i-1)*splitnum
	    ed := i*splitnum
	    p := prex + tmp + strconv.Itoa(i) + tmp + strconv.Itoa(n) + tmp + string(rs[st:ed])
	    log.Debug("===========SendMsgToPeer=============","i",i,"msg len",len(p),"msg",new(big.Int).SetBytes([]byte(p)))
	    va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(p)))
	    p2pdcrm.SendMsgToPeer(enodes,va)
	}

	if q2 != 0 {
	    st := q1*splitnum
	    log.Debug("SendMsgToPeer","st",st,"len rs",len(rs))
	    p := prex + tmp + strconv.Itoa(n) + tmp + strconv.Itoa(n) + tmp + string(rs[st:])
	    log.Debug("===========SendMsgToPeer=============","i",i,"msg len",len(p),"msg",new(big.Int).SetBytes([]byte(p)))
	    va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(p)))
	    p2pdcrm.SendMsgToPeer(enodes,va)
	}

    } else {
	    log.Debug("===========SendMsgToPeer=============","msg len",len(msg),"msg",new(big.Int).SetBytes([]byte(msg)))
	    msg = prex + tmp + "1" + tmp + "1" + tmp + msg
	    va := fmt.Sprintf("%v",new(big.Int).SetBytes([]byte(msg)))
	    p2pdcrm.SendMsgToPeer(enodes,va)
    }*/
}

func finddc(hash string) bool {
    if hash == "" || DC == nil {
	return false
    }

    iter := DC.Front()
    for iter != nil {
	dc := iter.Value.(*DcrmChain)
	if dc == nil {
	    continue
	}

	if strings.EqualFold(dc.Txhash,hash) {
	    return true
	}

	iter = iter.Next()
    }

    return false
}

///////////////////////////////////////
func joinMsg(l *list.List) string {
    if l == nil {
	return ""
    }

    var dcrmparts = make(map[int]string)
    iter := l.Front()
    for iter != nil {
	s := iter.Value.(string)
	mm := strings.Split(s,"dcrmchjsendmsg")
	p,_ := strconv.Atoi(mm[1])

	dcrmparts[p] = mm[3]
	iter = iter.Next()
    }

    var c string = ""
    for i := 1; i <= l.Len(); i++ {
	c += dcrmparts[i]
    }

    return c
}

type DcrmAddrRes struct {
    FusionAccount string
    DcrmAddr string
    //Txhash string
    Type string
}

type DcrmPubkeyRes struct {
    FusionAccount string
    DcrmPubkey string
    //Type string
    //Status string //0:default   1:req  2:confirmed
    Addresses map[string]string
}

type WorkReq interface {
    Run(workid int,ch chan interface{}) bool
}

//RecvMsg
type RecvMsg struct {
    msg string
}

func (self *RecvMsg) Run(workid int,ch chan interface{}) bool {
    //log.Debug("==========RecvMsg.Run,","receiv orig msg",self.msg,"","=============")
    if workid < 0 { //TODO
	log.Debug("==========RecvMsg.Run,workid < 0","receiv orig msg",self.msg,"","=============")
	res2 := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res2
	return false
    }

    ///////////////////////
    /*mm := strings.Split(ss,"dcrmchjsendmsg")
    if len(mm) >= 4 {
	msgprex := mm[0]
	p,_ := strconv.Atoi(mm[1])
	n,_ := strconv.Atoi(mm[2])
	ms := strings.Split(msgprex,"-")
	if len(ms) < 3 {
	    res2 := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetPrexDataError)}
	    ch <- res2
	    return false
	}

	msgCode := ms[len(ms)-1]
	log.Debug("RecvMsg.Run","p",p,"n",n,"msgCode",msgCode)
	log.Debug("RecvMsg.Run","msgprex",msgprex)
	w,err := FindWorker(ms[0])
	if err != nil || w == nil { //TODO
	    res2 := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrNoFindWorker)}
	    ch <- res2
	    return false
	}
	log.Debug("==========RecvMsg.Run==========","find w.id",w.id)

	switch msgCode {
	case "C1":
	    val,ok := w.splitmsg_c1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_c1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_c1[msgprex] = val
	    }

	    if (w.splitmsg_c1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_c1[msgprex])
		log.Debug("RecvMsg.Run","c1 totalstr",totalstr)
		DisMsg(totalstr)
	    }
	case "D1":
	    val,ok := w.splitmsg_d1_1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_d1_1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_d1_1[msgprex] = val
	    }

	    if (w.splitmsg_d1_1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_d1_1[msgprex])
		log.Debug("RecvMsg.Run","d1 totalstr",totalstr)
		DisMsg(totalstr)
	    }
	case "SHARE1":
	    val,ok := w.splitmsg_share1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_share1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_share1[msgprex] = val
	    }

	    if (w.splitmsg_share1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_share1[msgprex])
		log.Debug("RecvMsg.Run","SHARE1 totalstr",totalstr)
		DisMsg(totalstr)
	    }
	case "ZKFACTPROOF":
	    val,ok := w.splitmsg_zkfact[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_zkfact[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_zkfact[msgprex] = val
	    }

	    if (w.splitmsg_zkfact[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_zkfact[msgprex])
		log.Debug("RecvMsg.Run","ZKFACTPROOF totalstr",totalstr)
		DisMsg(totalstr)
	    }
	case "ZKUPROOF":
	    val,ok := w.splitmsg_zku[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_zku[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_zku[msgprex] = val
	    }

	    if (w.splitmsg_zku[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_zku[msgprex])
		log.Debug("RecvMsg.Run","ZKUPROOF totalstr",totalstr)
		DisMsg(totalstr)
	    }
	case "MTAZK1PROOF":
	    log.Debug("RecvMsg.Run,MTAZK1PROOF","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	    val,ok := w.splitmsg_mtazk1proof[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_mtazk1proof[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_mtazk1proof[msgprex] = val
	    }

	    if (w.splitmsg_mtazk1proof[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_mtazk1proof[msgprex])
		//log.Debug("RecvMsg.Run","MTAZK1PROOF totalstr",totalstr)
		log.Debug("RecvMsg.Run,MTAZK1PROOF","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
       case "C11":
	    log.Debug("RecvMsg.Run,C11","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	   val,ok := w.splitmsg_c11[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_c11[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_c11[msgprex] = val
	    }

	    if (w.splitmsg_c11[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_c11[msgprex])
		//log.Debug("RecvMsg.Run","C11 totalstr",totalstr)
		log.Debug("RecvMsg.Run,C11","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
       case "KC":
	    log.Debug("RecvMsg.Run,KC","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	   val,ok := w.splitmsg_kc[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_kc[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_kc[msgprex] = val
	    }

	    if (w.splitmsg_kc[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_kc[msgprex])
		//log.Debug("RecvMsg.Run","KC totalstr",totalstr)
		log.Debug("RecvMsg.Run,KC","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
       case "MKG":
	    log.Debug("RecvMsg.Run,MKG","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	   val,ok := w.splitmsg_mkg[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_mkg[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_mkg[msgprex] = val
	    }

	    if (w.splitmsg_mkg[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_mkg[msgprex])
		//log.Debug("RecvMsg.Run","MKG totalstr",totalstr)
		log.Debug("RecvMsg.Run,MKG","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
       case "MKW":
	    log.Debug("RecvMsg.Run,MKW","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	   val,ok := w.splitmsg_mkw[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_mkw[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_mkw[msgprex] = val
	    }

	    if (w.splitmsg_mkw[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_mkw[msgprex])
		//log.Debug("RecvMsg.Run","MKW totalstr",totalstr)
		log.Debug("RecvMsg.Run,MKW","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
       case "DELTA1":
	    log.Debug("RecvMsg.Run,DELTA1","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	   val,ok := w.splitmsg_delta1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_delta1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_delta1[msgprex] = val
	    }

	    if (w.splitmsg_delta1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_delta1[msgprex])
		//log.Debug("RecvMsg.Run","DELTA1 totalstr",totalstr)
		log.Debug("RecvMsg.Run,DELTA1","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
	case "D11":
	    log.Debug("RecvMsg.Run,D11","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	    val,ok := w.splitmsg_d11_1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_d11_1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_d11_1[msgprex] = val
	    }

	    if (w.splitmsg_d11_1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_d11_1[msgprex])
		//log.Debug("RecvMsg.Run","D11 totalstr",totalstr)
		log.Debug("RecvMsg.Run,D11","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
	case "S1":
	    log.Debug("RecvMsg.Run,S1","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	    val,ok := w.splitmsg_s1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_s1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_s1[msgprex] = val
	    }

	    if (w.splitmsg_s1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_s1[msgprex])
		//log.Debug("RecvMsg.Run","S1 totalstr",totalstr)
		log.Debug("RecvMsg.Run,S1","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
	case "SS1":
	    log.Debug("RecvMsg.Run,SS1","msg len",len(ss),"msg",new(big.Int).SetBytes([]byte(ss)))
	    val,ok := w.splitmsg_ss1[msgprex]
	    if !ok || val == nil {
		l := list.New()
		l.PushBack(ss)
		w.splitmsg_ss1[msgprex] = l
	    } else {
		val.PushBack(ss)
		w.splitmsg_ss1[msgprex] = val
	    }

	    if (w.splitmsg_ss1[msgprex]).Len() == n {
		totalstr := joinMsg(w.splitmsg_ss1[msgprex])
		//log.Debug("RecvMsg.Run","SS1 totalstr",totalstr)
		log.Debug("RecvMsg.Run,SS1","totalstr len",len(totalstr),"totalstr",new(big.Int).SetBytes([]byte(totalstr)))
		DisMsg(totalstr)
	    }
	default:
	    log.Debug("unkown msg code")
	}

	return true 
    }*/
    ////////////////////////

    /////////
    res := self.msg
    if res == "" { //TODO
	return false 
    }

    mm := strings.Split(res,common.Sep)
    if len(mm) >= 2 {
	//msg:  hash-enode:C1:X1:X2
	DisMsg(res)
	return true 
    }
    
    res,err := UnCompress(res)
    if err != nil {
	return false
    }
    r,err := Decode(res)
    if err != nil {
	return false
    }

    switch r.(type) {
    case *SendMsg:
	rr := r.(*SendMsg)

	log.Debug("==========RecvMsg.Run,","decode msg",rr.Msg,"","=============")
	if rr.MsgType == "ec2_data" {
	    mm := strings.Split(rr.Msg,common.Sep)
	    if len(mm) >= 2 {
		//msg:  hash-enode:C1:X1:X2
		DisMsg(rr.Msg)
		return true 
	    }
	    return true
	}

	//get_matchres
	if rr.MsgType == "get_matchres" {
	    w := workers[workid]
	    w.sid = rr.Nonce
	    rch := make(chan interface{},1)
	    getmatchrescall(rr.Msg,rch)
	    chret,cherr := GetChannelValue(ch_t,rch)
	    if cherr != nil {
		var ret2 Err
		ret2.Info = cherr.Error()
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		return false
	    }
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
	    ch <- res2
	    return true
	}

	//get_dcrmgroup_count
	if rr.MsgType == "get_dcrmgroup_count" {
	    w := workers[workid]
	    w.sid = rr.Nonce
	    rch := make(chan interface{},1)
	    get_dcrmgroup_count(rr.Msg,rch)
	    chret,cherr := GetChannelValue(ch_t,rch)
	    if cherr != nil {
		var ret2 Err
		ret2.Info = cherr.Error()
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		return false
	    }
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
	    ch <- res2
	    return true
	}

	//rpc_order_create
	if rr.MsgType == "rpc_order_create" {
	    w := workers[workid]
	    w.sid = rr.Nonce
	    rch := make(chan interface{},1)
	    //SendMsgToDcrmGroup(self.msg)
	    obcall(rr.Msg,rch)
	    chret,cherr := GetChannelValue(ch_t,rch)
	    if cherr != nil {
		var ret2 Err
		ret2.Info = cherr.Error()
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		return false
	    }
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
	    ch <- res2
	    return true
	}

	//rpc_lockout
	if rr.MsgType == "rpc_lockout" {
	    w := workers[workid]
	    w.sid = rr.Nonce
	    //msg = lockout_txhash:lockout_tx:fusionaddr:dcrmfrom:realfusionfrom:realdcrmfrom:lockoutto:value:cointype
	    msg := rr.Msg
	    msgs := strings.Split(msg,common.Sep)
	    //log.Debug("==========RecvMsg.Run=========","selected workid",workid,"msg type",rr.MsgType,"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])

	    /////bug
	    val,ok := GetLockoutInfoFromLocalDB(msgs[0])
	    if ok == nil && val != "" {
		log.Debug("=================RecvMsg.Run,this lockout tx has handle before.=================","decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+val,Err:nil}
		ch <- res2
		return true 
	    }

	    var realfusionfrom string
	    var realdcrmfrom string

	    hash := crypto.Keccak256Hash([]byte(msgs[0] + ":" + strings.ToLower(msgs[8]))).Hex()
	    val,err := GetLockoutInfoFromLocalDB(hash)
	    if err == nil && val != "" {
		retvas := strings.Split(val,common.Sep10)
		if len(retvas) >= 2 {
		    log.Debug("============RecvMsg.Run,has already get real account===========","decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
		    realfusionfrom = retvas[0]
		    realdcrmfrom = retvas[1]
		}
	    }

	    if realfusionfrom == "" || realdcrmfrom == "" {
		realfusionfrom,realdcrmfrom,err = ChooseRealFusionAccountForLockout(msgs[7],msgs[6],msgs[8])
		if err != nil {
		    log.Debug("============RecvMsg.Run===========","err",err.Error(),"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
		    var ret2 Err
		    ret2.Info = err.Error() 
		    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		    ch <- res2
		    return false
		}

		if IsValidFusionAddr(realfusionfrom) == false {
		    log.Debug("============RecvMsg.Run,validate real fusion from fail===========","decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
		    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:GetRetErr(ErrValidateRealFusionAddrFail)}
		    ch <- res2
		    return false
		}
		if IsValidDcrmAddr(realdcrmfrom,msgs[8]) == false {
		    log.Debug("============validate_lockout,validate real dcrm from fail===========","decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
		    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:GetRetErr(ErrValidateRealDcrmFromFail)}
		    ch <- res2
		    return false
		}
	    }
	    /////////

	    /*if finddc(msgs[0]) {
		log.Debug("============RecvMsg.Run,tx hash has handle before.===========")
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:fmt.Errorf("already known lockout transaction:%s",msgs[0])}
		ch <- res2
		return false
	    }

	    dc := &DcrmChain{Lo_info:nil,Da:nil,Dc_info:nil,Txhash:msgs[0]}
	    DC.PushBack(dc)*/

	//bug//
	lo_signtx := new(types.Transaction)
	errjson := lo_signtx.UnmarshalJSON([]byte(msgs[1]))
	if errjson != nil {
	    log.Debug("===============validate_lockout==================","errjson",errjson,"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])

	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:errjson}
	    ch <- res2
	    return false
	}
	//

	    rch := make(chan interface{},1)
	    //SendMsgToDcrmGroup(self.msg)
	    validate_lockout(w.sid,msgs[0],msgs[1],msgs[2],msgs[3],realfusionfrom,realdcrmfrom,msgs[6],msgs[7],msgs[8],rch)
	    //txcall(lo_signtx) //bug:if someone kill gdcrm after send outside tx 

	    chret,cherr := GetChannelValue(ch_t,rch)
	    if chret != "" {
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
		ch <- res2
		//log.Debug("========for test only 111==========")
		return true
	    }

	    if cherr != nil {
		var ret2 Err
		ret2.Info = cherr.Error() 
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		//log.Debug("========for test only 222 ==========")
		return false
	    }
	    
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:GetRetErr(ErrSendTxToNetFail)}
	    ch <- res2
	    //log.Debug("========for test only 333==========")
	    return true
	}

	//rpc_get_real_account
	if rr.MsgType == "rpc_get_real_account" {
	    //msg = lockout_txhash:lockout_tx:fusionaddr:dcrmfrom:realfusionfrom:realdcrmfrom:lockoutto:value:cointype
	    msg := rr.Msg
	    msgs := strings.Split(msg,common.Sep)
	    //log.Debug("==========RecvMsg.Run,get real account=========","selected workid",workid,"msg type",rr.MsgType,"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])

	    //bug
	    hash := crypto.Keccak256Hash([]byte(msgs[0] + ":" + strings.ToLower(msgs[8]))).Hex()
	    val,err := GetLockoutInfoFromLocalDB(hash)
	    if err == nil && val != "" {
		retvas := strings.Split(val,common.Sep10)
		if len(retvas) >= 2 {
		    log.Debug("============RecvMsg.Run,has already get real account.===========","lockout info hash",hash,"lockout info",val,"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
		    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+val,Err:nil}
		    ch <- res2
		    return true
		}
	    }
	    //

	    realfusionfrom,realdcrmfrom,err := ChooseRealFusionAccountForLockout(msgs[7],msgs[6],msgs[8])
	if err != nil {
	    log.Debug("============RecvMsg.Run,get real account===========","err",err.Error(),"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
	    var ret2 Err
	    ret2.Info = err.Error() 
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
	    ch <- res2
	    return false
	}

	if IsValidFusionAddr(realfusionfrom) == false {
	    log.Debug("============RecvMsg.Run,validate real fusion from fail===========","decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:GetRetErr(ErrValidateRealFusionAddrFail)}
	    ch <- res2
	    return false
	}
	if IsValidDcrmAddr(realdcrmfrom,msgs[8]) == false {
	    log.Debug("============RecvMsg.Run,validate real dcrm from fail===========","decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:GetRetErr(ErrValidateRealDcrmFromFail)}
	    ch <- res2
	    return false
	}
	    
	    retva := realfusionfrom + common.Sep10 + realdcrmfrom
	    hash = crypto.Keccak256Hash([]byte(msgs[0] + ":" + strings.ToLower(msgs[8]))).Hex()
	    WriteLockoutInfoToLocalDB(hash,retva)
	    //log.Debug("============RecvMsg.Run,write lockout info to local db.===========","lockout info hash",hash,"lockout info",retva,"decode msg",rr.Msg,"lockout tx hash",msgs[0],"cointype",msgs[8])
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+retva,Err:nil}
	    ch <- res2
	    return true
	}

	//rpc_lockin
	if rr.MsgType == "rpc_lockin" {
	    w := workers[workid]
	    w.sid = rr.Nonce
	    //msg = lockin_txhash:lockin_tx:fusionaddr:hashkey:value:cointype:lockinaddr:realdcrmfrom
	    msg := rr.Msg
	    msgs := strings.Split(msg,common.Sep)
	    rch := make(chan interface{},1)
	    //SendMsgToDcrmGroup(self.msg)
	    validate_txhash(msgs[0],msgs[1],msgs[6],msgs[3],msgs[7],rch)
	    chret,cherr := GetChannelValue(ch_t,rch)
	    log.Debug("RecvMsg.Run","chret",chret,"cherr",cherr)
	    if cherr != nil {
		log.Debug("RecvMsg.Run,return fail.")
		var ret2 Err
		ret2.Info = cherr.Error() 
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		return false
	    }
	    log.Debug("RecvMsg.Run,return success.")
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
	    ch <- res2
	    return true
	}

	//rpc_confirm_dcrmaddr
	if rr.MsgType == "rpc_confirm_dcrmaddr" {
	    w := workers[workid]
	    w.sid = rr.Nonce
	    //msg = confirm_txhash:confirm_tx:fusionaddr:dcrmaddr:hashkey:cointype
	    msg := rr.Msg
	    msgs := strings.Split(msg,common.Sep)
	    rch := make(chan interface{},1)
	    //SendMsgToDcrmGroup(self.msg)
	    dcrm_confirmaddr("",msgs[0],msgs[1],msgs[2],msgs[3],msgs[4],msgs[5],rch)
	    chret,cherr := GetChannelValue(ch_t,rch)
	    if cherr != nil {
		var ret2 Err
		ret2.Info = cherr.Error()
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		return false
	    }
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
	    ch <- res2
	    return true
	}

	//rpc_req_dcrmaddr
	if rr.MsgType == "rpc_req_dcrmaddr" {
	    //msg = fusionaddr:pubkey:cointype
	    msg := rr.Msg
	    msgs := strings.Split(msg,common.Sep)
	    rch := make(chan interface{},1)
	    //SendMsgToDcrmGroup(self.msg)
	    w := workers[workid]
	    w.sid = rr.Nonce
	    log.Debug("RecvMsg.Run","w.sid",w.sid)

	    has,da,err := IsFusionAccountExsitDcrmAddr(msgs[0],msgs[2],"")
	    if err == nil && has == true { 
		log.Debug("RecvMsg.Run","da",da)
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+da,Err:nil}
		ch <- res2
		return true
	    }

	    dcrm_liloreqAddress(w.sid,msgs[0],msgs[1],msgs[2],rch)
	    //log.Debug("==========RecvMsg.Run,33333333","rr.MsgType",rr.MsgType,"","===================")
	    chret,cherr := GetChannelValue(ch_t,rch)
	    if cherr != nil {
		log.Debug("==========RecvMsg.Run,444444","rr.MsgType",rr.MsgType,"err",cherr.Error(),"chret",chret,"","===================")
		var ret2 Err
		ret2.Info = cherr.Error()
		res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType,Err:ret2}
		ch <- res2
		return false
	    }
	    //log.Debug("==========RecvMsg.Run,5555555","rr.MsgType",rr.MsgType,"","===================")
	    res2 := RpcDcrmRes{Ret:strconv.Itoa(rr.WorkId)+common.Sep+rr.MsgType+common.Sep+chret,Err:nil}
	    ch <- res2
	    return true
	}
    //case *RpcDcrmRes:
    default:
	return false
    }
    /////////

    return true 
}

////////////////////////////////////////////
type ReqAddrSendMsgToDcrm struct {
    Fusionaddr string
    Pub string
    Cointype string
}

func (self *ReqAddrSendMsgToDcrm) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("ReqAddrSendMsgToDcrm.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }

    GetEnodesInfo()
    timestamp := time.Now().Unix()
    tt := strconv.Itoa(int(timestamp))
    nonce := crypto.Keccak256Hash([]byte(strings.ToLower(self.Fusionaddr) + ":" + strings.ToLower(self.Cointype) + ":" + tt + ":" + strconv.Itoa(workid))).Hex()
    
    msg := self.Fusionaddr + common.Sep + self.Pub + common.Sep + self.Cointype

    sm := &SendMsg{MsgType:"rpc_req_dcrmaddr",Nonce:nonce,WorkId:workid,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }
	
    //log.Debug("ReqAddrSendMsgToDcrm.Run,begin send msg")
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("ReqAddrSendMsgToDcrm.Run,send fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true
}

type ConfirmAddrSendMsgToDcrm struct {
    Txhash string
    Tx string
    FusionAddr string
    DcrmAddr string
    Hashkey string
    Cointype string
}

func (self *ConfirmAddrSendMsgToDcrm) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("ConfirmAddrSendMsgToDcrm.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }

    GetEnodesInfo()
    
    msg := self.Txhash + common.Sep + self.Tx + common.Sep + self.FusionAddr + common.Sep + self.DcrmAddr + common.Sep + self.Hashkey + common.Sep + self.Cointype
	
    sm := &SendMsg{MsgType:"rpc_confirm_dcrmaddr",Nonce:self.Txhash,WorkId:workid,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("ConfirmAddrSendMsgToDcrm.Run,begin send msg")
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("ConfirmAddrSendMsgToDcrm.Run,send fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true 
}

//lockin
type LockInSendMsgToDcrm struct {
    Txhash string
    Tx string
    Fusionaddr string
    Hashkey string
    Value string
    Cointype string
    LockinAddr string
    RealDcrmFrom string
}

func (self *LockInSendMsgToDcrm) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("LockInSendMsgToDcrm.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }

    GetEnodesInfo()
    
    //
    msg := self.Txhash + common.Sep + self.Tx + common.Sep + self.Fusionaddr + common.Sep + self.Hashkey + common.Sep + self.Value + common.Sep + self.Cointype + common.Sep + self.LockinAddr + common.Sep + self.RealDcrmFrom

    sm := &SendMsg{MsgType:"rpc_lockin",Nonce:self.Txhash,WorkId:workid,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("LockInSendMsgToDcrm.Run,begin send msg")
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("LockInSendMsgToDcrm.Run,send fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_lilo_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_lilo_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true   
}

//lockout
type LockoutSendMsgToDcrm struct {
    Txhash string
    Tx string
    FusionFrom string
    DcrmFrom string
    RealFusionFrom string
    RealDcrmFrom string
    Lockoutto string
    Value string
    Cointype string
}

func (self *LockoutSendMsgToDcrm) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("LockoutSendMsgToDcrm.Run,workid < 0.","tx hash",self.Txhash,"cointype",self.Cointype)
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }

    GetEnodesInfo()
    
    msg := self.Txhash + common.Sep + self.Tx + common.Sep + self.FusionFrom + common.Sep + self.DcrmFrom + common.Sep + self.RealFusionFrom + common.Sep + self.RealDcrmFrom + common.Sep + self.Lockoutto + common.Sep + self.Value + common.Sep + self.Cointype

    sm := &SendMsg{MsgType:"rpc_lockout",Nonce:self.Txhash,WorkId:workid,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("LockoutSendMsgToDcrm.Run,begin send msg","current workid",workid,"tx hash",self.Txhash,"cointype",self.Cointype)
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("LockoutSendMsgToDcrm.Run,send fail.","current workid",workid,"tx hash",self.Txhash,"cointype",self.Cointype)
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_lilo_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_lilo_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true
}

//get real account
type LockoutGetRealAccount struct {
    Txhash string
    Tx string
    FusionFrom string
    DcrmFrom string
    RealFusionFrom string
    RealDcrmFrom string
    Lockoutto string
    Value string
    Cointype string
}

func (self *LockoutGetRealAccount) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("LockoutGetRealAccount.Run,workid < 0.","tx hash",self.Txhash,"cointype",self.Cointype)
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }

    GetEnodesInfo()

    msg := self.Txhash + common.Sep + self.Tx + common.Sep + self.FusionFrom + common.Sep + self.DcrmFrom + common.Sep + self.RealFusionFrom + common.Sep + self.RealDcrmFrom + common.Sep + self.Lockoutto + common.Sep + self.Value + common.Sep + self.Cointype

    sm := &SendMsg{MsgType:"rpc_get_real_account",Nonce:self.Txhash,WorkId:workid,Msg:msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("LockoutGetRealAccount.Run,begin send msg","current workid",workid,"tx hash",self.Txhash,"cointype",self.Cointype)
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("LockoutGetRealAccount.Run,send fail.","current workid",workid,"tx hash",self.Txhash,"cointype",self.Cointype)
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_lilo_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_lilo_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true
}

//order create
type OrderCreateSendMsgToXvc struct {
    //txhash:tx:fusionaddr:trade:ordertype:side:price:quanity:rule:time
    Msg string
}

func (self *OrderCreateSendMsgToXvc) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("OrderCreateSendMsgToXvc.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }
    
    GetEnodesInfo()

    segs := strings.Split(self.Msg,common.Sep9)
    if len(segs) < 10 {
	log.Debug("OrderCreateSendMsgToXvc.Run,Msg data format error.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrInternalMsgFormatError)}
	ch <- res
	return false
    }
    sm := &SendMsg{MsgType:"rpc_order_create",Nonce:segs[0],WorkId:workid,Msg:self.Msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("OrderCreateSendMsgToXvc.Run,begin send msg")
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("OrderCreateSendMsgToXvc.Run,send fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true
}

//get matchres
type SendMsg struct {
    MsgType string
    Nonce string 
    WorkId int
    Msg string
}

type GetMatchResSendMsgToXvc struct {
    Msg string
}

func (self *GetMatchResSendMsgToXvc) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("GetMatchResSendMsgToXvc.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }
    
    GetEnodesInfo()
    timestamp := time.Now().Unix()
    tt := strconv.Itoa(int(timestamp))
    nonce := crypto.Keccak256Hash([]byte(tt + ":" + strconv.Itoa(workid))).Hex()

    sm := &SendMsg{MsgType:"get_matchres",Nonce:nonce,WorkId:workid,Msg:self.Msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("GetMatchResSendMsgToXvc.Run,begin send msg")
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("GetMatchResSendMsgToXvc.Run,send fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true
}

//get dcrm group count
func get_dcrmgroup_count(msg string,ch chan interface{}) {
    c := strconv.Itoa(NodeCnt)
    res := RpcDcrmRes{Ret:c,Err:nil}
    ch <- res
}

type GetDcrmGroupCountSendMsgToXvc struct {
    Msg string
}

func (self *GetDcrmGroupCountSendMsgToXvc) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("GetDcrmGroupCountSendMsgToXvc.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }
    
    GetEnodesInfo()
    timestamp := time.Now().Unix()
    tt := strconv.Itoa(int(timestamp))
    nonce := crypto.Keccak256Hash([]byte(tt + ":" + strconv.Itoa(workid))).Hex()

    sm := &SendMsg{MsgType:"get_dcrmgroup_count",Nonce:nonce,WorkId:workid,Msg:self.Msg}
    res,err := Encode(sm)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    res,err = Compress([]byte(res))
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false
    }

    //log.Debug("GetDcrmGroupCountSendMsgToXvc.Run,begin send msg")
    s := p2pdcrm.SendToDcrmGroupAllNodes(res)
    if strings.EqualFold(s,"send fail.") {
	log.Debug("GetDcrmGroupCountSendMsgToXvc.Run,send fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendDataToGroupFail)}
	ch <- res
	return false
    }

    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    } else {
	w := workers[workid]
	chret,cherr := GetChannelValue(sendtogroup_timeout,w.ch)
	if cherr != nil {
	    res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	    ch <- res2
	    return false
	}
	res2 := RpcDcrmRes{Ret:chret,Err:cherr}
	ch <- res2
    }

    return true
}

//get all matchres
type GetAllMatchResSendMsgToXvc struct {
    Msg string
}

func (self *GetAllMatchResSendMsgToXvc) Run(workid int,ch chan interface{}) bool {
    if workid < 0 {
	log.Debug("GetAllMatchResSendMsgToXvc.Run,workid < 0.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }
    
    GetEnodesInfo()
    return true
}

//===========================================

type RpcDcrmRes struct {
    Ret string
    Err error
}

type RpcReq struct {
    rpcdata WorkReq
    ch chan interface{}
}

/////non dcrm///

func InitNonDcrmChan() {
    non_dcrm_workers = make([]*RpcReqNonDcrmWorker,RpcMaxNonDcrmWorker)
    RpcReqNonDcrmQueue = make(chan RpcReq,RpcMaxNonDcrmQueue)
    reqdispatcher := NewReqNonDcrmDispatcher(RpcMaxNonDcrmWorker)
    reqdispatcher.Run()

    cryptocoinsconfig.Init()
    cryptocoins.Init()
}

type ReqNonDcrmDispatcher struct {
    // A pool of workers channels that are registered with the dispatcher
    WorkerPool chan chan RpcReq
}

func NewReqNonDcrmDispatcher(maxWorkers int) *ReqNonDcrmDispatcher {
    pool := make(chan chan RpcReq, maxWorkers)
    return &ReqNonDcrmDispatcher{WorkerPool: pool}
}

func (d *ReqNonDcrmDispatcher) Run() {
// starting n number of workers
    for i := 0; i < RpcMaxNonDcrmWorker; i++ {
	worker := NewRpcReqNonDcrmWorker(d.WorkerPool)
	worker.id = i
	non_dcrm_workers[i] = worker
	worker.Start()
    }

    go d.dispatch()
}

func (d *ReqNonDcrmDispatcher) dispatch() {
    for {
	select {
	    case req := <-RpcReqNonDcrmQueue:
	    // a job request has been received
	    go func(req RpcReq) {
		// try to obtain a worker job channel that is available.
		// this will block until a worker is idle
		reqChannel := <-d.WorkerPool

		// dispatch the job to the worker job channel
		reqChannel <- req
	    }(req)
	}
    }
}

func NewRpcReqNonDcrmWorker(workerPool chan chan RpcReq) *RpcReqNonDcrmWorker {
    return &RpcReqNonDcrmWorker{
    RpcReqWorkerPool: workerPool,
    RpcReqChannel: make(chan RpcReq),
    rpcquit:       make(chan bool),
    dcrmret:	make(chan string,1),
    retres:list.New(),
    ch:		   make(chan interface{})}
}

type RpcReqNonDcrmWorker struct {
    RpcReqWorkerPool  chan chan RpcReq
    RpcReqChannel  chan RpcReq
    rpcquit        chan bool

    id int
    dcrmret chan string

    ch chan interface{}
    retres *list.List
}

func (w *RpcReqNonDcrmWorker) Clear() {

    log.Debug("========RpcReqNonDcrmWorker.Clear===============","w.id",w.id)
    var next *list.Element
    for e := w.retres.Front(); e != nil; e = next {
        next = e.Next()
        w.retres.Remove(e)
    }

    if len(w.ch) == 1 {
	<-w.ch
    }
    if len(w.dcrmret) == 1 {
	<-w.dcrmret
    }
    if len(w.rpcquit) == 1 {
	<-w.rpcquit
    }
}

func (w *RpcReqNonDcrmWorker) Start() {
    go func() {

	for {
	    // register the current worker into the worker queue.
	    w.RpcReqWorkerPool <- w.RpcReqChannel
	    select {
		    case req := <-w.RpcReqChannel:
			    req.rpcdata.Run(w.id,req.ch)
			    log.Debug("========RpcReqNonDcrmWorker.Start.Run finish.===============","w.id",w.id)
			    ////
			    log.Debug("========RpcReqNonDcrmWorker.Start.Clear===============","w.id",w.id)
			    w.Clear()
			    /////

		    case <-w.rpcquit:
			// we have received a signal to stop
			    return
		}
	}
    }()
}

func (w *RpcReqNonDcrmWorker) Stop() {
    go func() {
	w.rpcquit <- true
    }()
}

///////dcrm/////////

/*func getworkerid(msgprex string,enode string) int {//fun-e-xx-i-enode1-j-enode2-k
    
    prexs := strings.Split(msgprex,"-")
    if len(prexs) < 3 {
	log.Debug("============getworkerid,len(prexs) < 3===========","msgprex",msgprex)
	return -1
    }

    s := prexs[:3]
    prex := strings.Join(s,"-")
    wid,exsit := types.GetDcrmRpcWorkersDataKReady(prex)
    if exsit == false {
	log.Debug("============getworkerid,exsit is false.===========")
	return -1
    }

    id,_ := strconv.Atoi(wid)
    return id
}
*/

//rpc-req
type ReqDispatcher struct {
    // A pool of workers channels that are registered with the dispatcher
    WorkerPool chan chan RpcReq
}

type RpcReqWorker struct {
    RpcReqWorkerPool  chan chan RpcReq
    RpcReqChannel  chan RpcReq
    rpcquit        chan bool
    id int
    ch chan interface{}
    retres *list.List
    //
    msg_c1 *list.List
    splitmsg_c1 map[string]*list.List
    
    msg_kc *list.List
    splitmsg_kc map[string]*list.List
    
    msg_mkg *list.List
    splitmsg_mkg map[string]*list.List
    
    msg_mkw *list.List
    splitmsg_mkw map[string]*list.List
    
    msg_delta1 *list.List
    splitmsg_delta1 map[string]*list.List
    
    msg_d1_1 *list.List
    splitmsg_d1_1 map[string]*list.List
    
    msg_share1 *list.List
    splitmsg_share1 map[string]*list.List
    
    msg_zkfact *list.List
    splitmsg_zkfact map[string]*list.List
    
    msg_zku *list.List
    splitmsg_zku map[string]*list.List
    
    msg_mtazk1proof *list.List
    splitmsg_mtazk1proof map[string]*list.List
    
    msg_c11 *list.List
    splitmsg_c11 map[string]*list.List
    
    msg_d11_1 *list.List
    splitmsg_d11_1 map[string]*list.List
    
    msg_s1 *list.List
    splitmsg_s1 map[string]*list.List
    
    msg_ss1 *list.List
    splitmsg_ss1 map[string]*list.List

    pkx *list.List
    pky *list.List
    save *list.List
    
    bc1 chan bool
    bmkg chan bool
    bmkw chan bool
    bdelta1 chan bool
    bd1_1 chan bool
    bshare1 chan bool
    bzkfact chan bool
    bzku chan bool
    bmtazk1proof chan bool
    bkc chan bool
    bs1 chan bool
    bss1 chan bool
    bc11 chan bool
    bd11_1 chan bool

    sid string //save the txhash
}

//workers,RpcMaxWorker,RpcReqWorker,RpcReqQueue,RpcMaxQueue,ReqDispatcher
func InitChan() {
    workers = make([]*RpcReqWorker,RpcMaxWorker)
    RpcReqQueue = make(chan RpcReq,RpcMaxQueue)
    reqdispatcher := NewReqDispatcher(RpcMaxWorker)
    reqdispatcher.Run()
}

func NewReqDispatcher(maxWorkers int) *ReqDispatcher {
    pool := make(chan chan RpcReq, maxWorkers)
    return &ReqDispatcher{WorkerPool: pool}
}

func (d *ReqDispatcher) Run() {
// starting n number of workers
    for i := 0; i < RpcMaxWorker; i++ {
	worker := NewRpcReqWorker(d.WorkerPool)
	worker.id = i
	workers[i] = worker
	worker.Start()
    }

    go d.dispatch()
}

func (d *ReqDispatcher) dispatch() {
    for {
	select {
	    case req := <-RpcReqQueue:
	    // a job request has been received
	    go func(req RpcReq) {
		// try to obtain a worker job channel that is available.
		// this will block until a worker is idle
		reqChannel := <-d.WorkerPool

		// dispatch the job to the worker job channel
		reqChannel <- req
	    }(req)
	}
    }
}

func FindWorker(sid string) (*RpcReqWorker,error) {
    for i := 0; i < RpcMaxWorker; i++ {
	w := workers[i]

	//if len(w.sid) > 0 {
	  //  log.Debug("FindWorker","w.sid",w.sid,"sid",sid)
	//}

	if strings.EqualFold(w.sid,sid) {
	    //log.Debug("FindWorker,get the result.")
	    return w,nil
	}
    }

    time.Sleep(time.Duration(5)*time.Second) //1000 == 1s //TODO
    
    for i := 0; i < RpcMaxWorker; i++ {
	w := workers[i]
	if strings.EqualFold(w.sid,sid) {
	    //log.Debug("FindWorker,get the result.")
	    return w,nil
	}
    }

    return nil,GetRetErr(ErrNoFindWorker)
}

func NewRpcReqWorker(workerPool chan chan RpcReq) *RpcReqWorker {
    return &RpcReqWorker{
    RpcReqWorkerPool: workerPool,
    RpcReqChannel: make(chan RpcReq),
    rpcquit:       make(chan bool),
    retres:list.New(),
    ch:		   make(chan interface{}),
    msg_share1:list.New(),
    splitmsg_share1:make(map[string]*list.List),
    msg_zkfact:list.New(),
    splitmsg_zkfact:make(map[string]*list.List),
    msg_zku:list.New(),
    splitmsg_zku:make(map[string]*list.List),
    msg_mtazk1proof:list.New(),
    splitmsg_mtazk1proof:make(map[string]*list.List),
    msg_c1:list.New(),
    splitmsg_c1:make(map[string]*list.List),
    msg_d1_1:list.New(),
    splitmsg_d1_1:make(map[string]*list.List),
    msg_c11:list.New(),
    splitmsg_c11:make(map[string]*list.List),
    msg_kc:list.New(),
    splitmsg_kc:make(map[string]*list.List),
    msg_mkg:list.New(),
    splitmsg_mkg:make(map[string]*list.List),
    msg_mkw:list.New(),
    splitmsg_mkw:make(map[string]*list.List),
    msg_delta1:list.New(),
    splitmsg_delta1:make(map[string]*list.List),
    msg_d11_1:list.New(),
    splitmsg_d11_1:make(map[string]*list.List),
    msg_s1:list.New(),
    splitmsg_s1:make(map[string]*list.List),
    msg_ss1:list.New(),
    splitmsg_ss1:make(map[string]*list.List),
    
    pkx:list.New(),
    pky:list.New(),
    save:list.New(),
    
    bc1:make(chan bool,1),
    bd1_1:make(chan bool,1),
    bc11:make(chan bool,1),
    bkc:make(chan bool,1),
    bs1:make(chan bool,1),
    bss1:make(chan bool,1),
    bmkg:make(chan bool,1),
    bmkw:make(chan bool,1),
    bshare1:make(chan bool,1),
    bzkfact:make(chan bool,1),
    bzku:make(chan bool,1),
    bmtazk1proof:make(chan bool,1),
    bdelta1:make(chan bool,1),
    bd11_1:make(chan bool,1),

    sid:"",
    }
}

func (w *RpcReqWorker) Clear() {

    //log.Debug("===========RpcReqWorker.Clear===================","w.id",w.id)

    w.sid = ""
    
    var next *list.Element
    
    for e := w.msg_c1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_c1.Remove(e)
    }
    
    for e := w.msg_kc.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_kc.Remove(e)
    }

    for e := w.msg_mkg.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_mkg.Remove(e)
    }

    for e := w.msg_mkw.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_mkw.Remove(e)
    }

    for e := w.msg_delta1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_delta1.Remove(e)
    }

    for e := w.msg_d1_1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_d1_1.Remove(e)
    }

    for e := w.msg_share1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_share1.Remove(e)
    }

    for e := w.msg_zkfact.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_zkfact.Remove(e)
    }

    for e := w.msg_zku.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_zku.Remove(e)
    }

    for e := w.msg_mtazk1proof.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_mtazk1proof.Remove(e)
    }

    for e := w.msg_c11.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_c11.Remove(e)
    }

    for e := w.msg_d11_1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_d11_1.Remove(e)
    }

    for e := w.msg_s1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_s1.Remove(e)
    }

    for e := w.msg_ss1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_ss1.Remove(e)
    }

    for e := w.pkx.Front(); e != nil; e = next {
        next = e.Next()
        w.pkx.Remove(e)
    }

    for e := w.pky.Front(); e != nil; e = next {
        next = e.Next()
        w.pky.Remove(e)
    }

    for e := w.save.Front(); e != nil; e = next {
        next = e.Next()
        w.save.Remove(e)
    }

    for e := w.retres.Front(); e != nil; e = next {
        next = e.Next()
        w.retres.Remove(e)
    }

    if len(w.ch) == 1 {
	<-w.ch
    }
    if len(w.rpcquit) == 1 {
	<-w.rpcquit
    }
    if len(w.bshare1) == 1 {
	<-w.bshare1
    }
    if len(w.bzkfact) == 1 {
	<-w.bzkfact
    }
    if len(w.bzku) == 1 {
	<-w.bzku
    }
    if len(w.bmtazk1proof) == 1 {
	<-w.bmtazk1proof
    }
    if len(w.bc1) == 1 {
	<-w.bc1
    }
    if len(w.bd1_1) == 1 {
	<-w.bd1_1
    }
    if len(w.bc11) == 1 {
	<-w.bc11
    }
    if len(w.bkc) == 1 {
	<-w.bkc
    }
    if len(w.bs1) == 1 {
	<-w.bs1
    }
    if len(w.bss1) == 1 {
	<-w.bss1
    }
    if len(w.bmkg) == 1 {
	<-w.bmkg
    }
    if len(w.bmkw) == 1 {
	<-w.bmkw
    }
    if len(w.bdelta1) == 1 {
	<-w.bdelta1
    }
    if len(w.bd11_1) == 1 {
	<-w.bd11_1
    }

    //TODO
    w.splitmsg_c1 = make(map[string]*list.List)
    w.splitmsg_kc = make(map[string]*list.List)
    w.splitmsg_mkg = make(map[string]*list.List)
    w.splitmsg_mkw = make(map[string]*list.List)
    w.splitmsg_delta1 = make(map[string]*list.List)
    w.splitmsg_d1_1 = make(map[string]*list.List)
    w.splitmsg_share1 = make(map[string]*list.List)
    w.splitmsg_zkfact = make(map[string]*list.List)
    w.splitmsg_zku = make(map[string]*list.List)
    w.splitmsg_mtazk1proof = make(map[string]*list.List)
    w.splitmsg_c11 = make(map[string]*list.List)
    w.splitmsg_d11_1 = make(map[string]*list.List)
    w.splitmsg_s1 = make(map[string]*list.List)
    w.splitmsg_ss1 = make(map[string]*list.List)
}

func (w *RpcReqWorker) Clear2() {
    log.Debug("===========RpcReqWorker.Clear2===================","w.id",w.id)
    var next *list.Element
    
    for e := w.msg_c1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_c1.Remove(e)
    }
    
    for e := w.msg_kc.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_kc.Remove(e)
    }

    for e := w.msg_mkg.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_mkg.Remove(e)
    }

    for e := w.msg_mkw.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_mkw.Remove(e)
    }

    for e := w.msg_delta1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_delta1.Remove(e)
    }

    for e := w.msg_d1_1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_d1_1.Remove(e)
    }

    for e := w.msg_share1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_share1.Remove(e)
    }

    for e := w.msg_zkfact.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_zkfact.Remove(e)
    }

    for e := w.msg_zku.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_zku.Remove(e)
    }

    for e := w.msg_mtazk1proof.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_mtazk1proof.Remove(e)
    }

    for e := w.msg_c11.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_c11.Remove(e)
    }

    for e := w.msg_d11_1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_d11_1.Remove(e)
    }

    for e := w.msg_s1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_s1.Remove(e)
    }

    for e := w.msg_ss1.Front(); e != nil; e = next {
        next = e.Next()
        w.msg_ss1.Remove(e)
    }

    for e := w.retres.Front(); e != nil; e = next {
        next = e.Next()
        w.retres.Remove(e)
    }

    if len(w.ch) == 1 {
	<-w.ch
    }
    if len(w.rpcquit) == 1 {
	<-w.rpcquit
    }
    if len(w.bshare1) == 1 {
	<-w.bshare1
    }
    if len(w.bzkfact) == 1 {
	<-w.bzkfact
    }
    if len(w.bzku) == 1 {
	<-w.bzku
    }
    if len(w.bmtazk1proof) == 1 {
	<-w.bmtazk1proof
    }
    if len(w.bc1) == 1 {
	<-w.bc1
    }
    if len(w.bd1_1) == 1 {
	<-w.bd1_1
    }
    if len(w.bc11) == 1 {
	<-w.bc11
    }
    if len(w.bkc) == 1 {
	<-w.bkc
    }
    if len(w.bs1) == 1 {
	<-w.bs1
    }
    if len(w.bss1) == 1 {
	<-w.bss1
    }
    if len(w.bmkg) == 1 {
	<-w.bmkg
    }
    if len(w.bmkw) == 1 {
	<-w.bmkw
    }
    if len(w.bdelta1) == 1 {
	<-w.bdelta1
    }
    if len(w.bd11_1) == 1 {
	<-w.bd11_1
    }

    //TODO
    w.splitmsg_c1 = make(map[string]*list.List)
    w.splitmsg_kc = make(map[string]*list.List)
    w.splitmsg_mkg = make(map[string]*list.List)
    w.splitmsg_mkw = make(map[string]*list.List)
    w.splitmsg_delta1 = make(map[string]*list.List)
    w.splitmsg_d1_1 = make(map[string]*list.List)
    w.splitmsg_share1 = make(map[string]*list.List)
    w.splitmsg_zkfact = make(map[string]*list.List)
    w.splitmsg_zku = make(map[string]*list.List)
    w.splitmsg_mtazk1proof = make(map[string]*list.List)
    w.splitmsg_c11 = make(map[string]*list.List)
    w.splitmsg_d11_1 = make(map[string]*list.List)
    w.splitmsg_s1 = make(map[string]*list.List)
    w.splitmsg_ss1 = make(map[string]*list.List)
}

func (w *RpcReqWorker) Start() {
    go func() {

	for {
	    // register the current worker into the worker queue.
	    w.RpcReqWorkerPool <- w.RpcReqChannel
	    select {
		    case req := <-w.RpcReqChannel:
			    req.rpcdata.Run(w.id,req.ch)
			    //log.Debug("========RpcReqWorker.Start.Run finish.===============","w.id",w.id)
			    ///////clean msg_c1
			    //log.Debug("========RpcReqWorker.Start.Clear===============","w.id",w.id)
			    w.Clear()
			    ///////

		    case <-w.rpcquit:
			// we have received a signal to stop
			    return
		}
	}
    }()
}

func (w *RpcReqWorker) Stop() {
    go func() {
	w.rpcquit <- true
    }()
}
//rpc-req

//////////////////////////////////////

func init(){
	discover.RegisterSendCallback(DispenseSplitPrivKey)
	p2pdcrm.RegisterRecvCallback(call)
	p2pdcrm.RegisterCallback(call)
	p2pdcrm.RegisterDcrmCallback(dcrmcall)
	p2pdcrm.RegisterDcrmRetCallback(dcrmcallret)
	
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	log.Root().SetHandler(glogger)

	erc20_client = nil
	BTC_BLOCK_CONFIRMS = 1
	BTC_DEFAULT_FEE = 0.0005
	ETH_DEFAULT_FEE,_ = new(big.Int).SetString("10000000000000000",10)

	rpcs,_ = new(big.Int).SetString("0",10)

	OrderTest = "yes"
	
	ReorgNum = 10 //test net:10  main net:1000 
}

func GetGroupRes(wid int) RpcDcrmRes {
    if wid < 0 {
	res2 := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	return res2
    }

    var l *list.List
    if !IsInXvcGroup() && !IsInGroup() {
	w := non_dcrm_workers[wid]
	l = w.retres
    } else {
	w := workers[wid]
	l = w.retres
    }

    if l == nil {
	log.Debug("GetGroupRes,return res is nil.")
	res2 := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetNoResFromGroupMem)}
	return res2
    }

    var err error
    iter := l.Front()
    for iter != nil {
	ll := iter.Value.(*RpcDcrmRes)
	err = ll.Err
	if err == nil {
	    return (*ll)
	}
	iter = iter.Next()
    }

    iter = l.Front()
    for iter != nil {
	ll := iter.Value.(*RpcDcrmRes)
	err = ll.Err

	//
	//if (strings.Index(err.Error(),"confirming") + 1) != 0 { //found
	  //  log.Debug("GetGroupRes,confirming.")
	    //res2 := RpcDcrmRes{Ret:"",Err:err}
	    //return res2
	//}
	res2 := RpcDcrmRes{Ret:"",Err:err}
	return res2
	
	iter = iter.Next()
    }
    
    res2 := RpcDcrmRes{Ret:"",Err:nil}
    return res2
}

//===================node in group callback==============================
func dcrmcallret(msg interface{}) {
    log.Debug("==========dcrmcallret===========","msg",msg.(string),"NodeCnt",NodeCnt)
    res := msg.(string)
    if res == "" {
	return
    }
   
    NodeCnt = 3 //TODO

    ss := strings.Split(res,common.Sep)
    if len(ss) != 4 {
	return
    }

    status := ss[0]
    msgtype := ss[2]
    ret := ss[3]
    workid,err := strconv.Atoi(ss[1])
    if err != nil || workid < 0 {
	log.Debug("==========dcrmcallret,get workid error.===========","workid",ss[1],"status",status,"msg type",msgtype,"return value",ret,"NodeCnt",NodeCnt)
	return
    }

    //log.Debug("=============dcrmcallret============","status",status,"workid",workid,"msg type",msgtype,"return value",ret,"NodeCnt",NodeCnt)

    //success:workid:msgtype:ret
    if status == "success" {

	if !IsInXvcGroup() && !IsInGroup() {
	    w := non_dcrm_workers[workid]
	    //oldlen := w.retres.Len()
	    res2 := RpcDcrmRes{Ret:ret,Err:nil}
	    w.retres.PushBack(&res2)

	    //log.Debug("==========dcrmcallret==============","w.retres old len",oldlen,"w.retres new len",w.retres.Len(),"status",status,"workid",workid,"msg type",msgtype,"return value",ret,"NodeCnt",NodeCnt)

	    if msgtype == "get_matchres" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "get_dcrmgroup_count" {
		//if w.retres.Len() == NodeCnt {
		//    ret := GetGroupRes(w.retres)
		//    w.ch <- ret
		//}
	    }

	    if ss[2] == "rpc_order_create" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockout" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_get_real_account" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockin" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_confirm_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_req_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }
	} else {
	    w := workers[workid]
	    //oldlen := w.retres.Len()
	    res2 := RpcDcrmRes{Ret:ss[3],Err:nil}
	    w.retres.PushBack(&res2)

	    //log.Debug("===========dcrmcallret============","w.retres old len",oldlen,"w.retres new len",w.retres.Len(),"status",status,"workid",workid,"msg type",msgtype,"return value",ret,"NodeCnt",NodeCnt)

	    if ss[2] == "get_matchres" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "get_dcrmgroup_count" {
		//if w.retres.Len() == NodeCnt {
		//    ret := GetGroupRes(w.retres)
		//    w.ch <- ret
		//}
	    }

	    if ss[2] == "rpc_order_create" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockout" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }
		
	    if ss[2] == "rpc_get_real_account" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockin" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_confirm_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_req_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }
	}

	return
    }
    
    //fail:workid:msgtype:error
    if status == "fail" {

	if !IsInXvcGroup() && !IsInGroup() {
	    w := non_dcrm_workers[workid]
	    //oldlen := w.retres.Len()
	    var ret2 Err
	    ret2.Info = ret
	    res2 := RpcDcrmRes{Ret:"",Err:ret2}
	    w.retres.PushBack(&res2)

	    //log.Debug("==========dcrmcallret============","w.retres old len",oldlen,"w.retres new len",w.retres.Len(),"status",status,"workid",workid,"msg type",msgtype,"return value",ret,"NodeCnt",NodeCnt)
	    
	    if ss[2] == "get_matchres" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "get_dcrmgroup_count" {
		//if w.retres.Len() == NodeCnt {
		//    ret := GetGroupRes(w.retres)
		//    w.ch <- ret
		//}
	    }

	    if ss[2] == "rpc_order_create" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockout" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_get_real_account" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockin" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_confirm_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_req_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }
	} else {
	    w := workers[workid]
	    //oldlen := w.retres.Len()
	    var ret2 Err
	    ret2.Info = ret
	    res2 := RpcDcrmRes{Ret:"",Err:ret2}
	    w.retres.PushBack(&res2)

	    //log.Debug("============dcrmcallret===========","w.retres old len",oldlen,"w.retres new len",w.retres.Len(),"status",status,"workid",workid,"msg type",msgtype,"return value",ret,"NodeCnt",NodeCnt)

	    if ss[2] == "get_matchres" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "get_dcrmgroup_count" {
		//if w.retres.Len() == NodeCnt {
		//    ret := GetGroupRes(w.retres)
		//    w.ch <- ret
		//}
	    }

	    if ss[2] == "rpc_order_create" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockout" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_get_real_account" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_lockin" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_confirm_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }

	    if ss[2] == "rpc_req_dcrmaddr" {
		if w.retres.Len() == NodeCnt {
		    ret := GetGroupRes(workid)
		    w.ch <- ret
		}
	    }
	}
	
	return
    }
}

func dcrmcall(msg interface{}) <-chan string {
    log.Debug("================dcrmcall================","msg",msg.(string))
    ch := make(chan string, 1)
    
    s := msg.(string)
    v := RecvMsg{msg:s}
    rch := make(chan interface{},1)
    req := RpcReq{rpcdata:&v,ch:rch}
    RpcReqQueue <- req
    chret,cherr := GetChannelValue(sendtogroup_timeout,rch)
    //log.Debug("=============dcrmcall,return from RecvMsg.run=============","orig msg",s,"chret",chret,"cherr",cherr)
    if cherr != nil {
	log.Debug("=============dcrmcall,return fail status to sender node.============","orig msg",s)
	//fail:chret:error
	ret := ("fail"+common.Sep+chret+common.Sep+cherr.Error())
	ch <- ret 
	return ch
    }

    log.Debug("==============dcrmcall,return success status to sender node.===============","orig msg",s)
    //success:chret
    ret := ("success"+common.Sep+chret)
    ch <- ret 
    return ch
}

//=========================================

func call(msg interface{}) {
    s := msg.(string)
    //log.Debug("===========call=============","msg len",len(s),"msg",new(big.Int).SetBytes([]byte(s)))
    SetUpMsgList(s)
}

var parts = make(map[int]string)
func receiveSplitKey(msg interface{}){
	//log.Debug("==========receiveSplitKey==========")
	log.Debug("","get msg", msg)
	cur_enode = p2pdcrm.GetSelfID().String()
	log.Debug("","cur_enode", cur_enode)
	head := strings.Split(msg.(string), ":")[0]
	body := strings.Split(msg.(string), ":")[1]
	if a := strings.Split(body, "#"); len(a) > 1 {
		tmp2 = a[0]
		body = a[1]
	}
	p, _ := strconv.Atoi(strings.Split(head, "dcrmslash")[0])
	total, _ := strconv.Atoi(strings.Split(head, "dcrmslash")[1])
	parts[p] = body
	if len(parts) == total {
		var c string = ""
		for i := 1; i <= total; i++ {
			c += parts[i]
		}
		////bug
		groupReady := make(chan bool, 1)
		go waitGroup(groupReady)
		<- groupReady
		log.Debug("**********************************************************************************")
		peerscount, _ := p2pdcrm.GetGroup()
		////
		Init(tmp2,c,peerscount)
	}
}

func getgroupcount() int {
    peerscount, _ := p2pdcrm.GetGroup()
    return peerscount
}

func waitGroup(gr chan bool) {
    for {
	//log.Debug("==============================================================================================")
	gc := getgroupcount()
	if gc > 0 {
	    //log.Debug("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")
	    gr <- true
	    break
	}
    }
}

func Init(tmp string,c string,nodecnt int) {
    if init_times >= 1 {
	    return
    }

   NodeCnt = nodecnt
   enode_cnts = nodecnt //bug
    //log.Debug("=============Init,","the node count",NodeCnt,"","===========")
    GetEnodesInfo()
    InitChan()
    init_times = 1
 
    bmatchres_eth_btc = make(chan bool, 1)
    msg_matchres_eth_btc = make(chan string,NodeCnt-1)
    bmatchres_fsn_btc = make(chan bool, 1)
    msg_matchres_fsn_btc = make(chan string,NodeCnt-1)
    bmatchres_fsn_eth = make(chan bool, 1)
    msg_matchres_fsn_eth = make(chan string,NodeCnt-1)
}

//for eth 
type RPCTransaction struct {
	BlockHash        common.Hash     `json:"blockHash"`
	BlockNumber      *hexutil.Big    `json:"blockNumber"`
	From             common.Address  `json:"from"`
	Gas              hexutil.Uint64  `json:"gas"`
	GasPrice         *hexutil.Big    `json:"gasPrice"`
	Hash             common.Hash     `json:"hash"`
	Input            hexutil.Bytes   `json:"input"`
	Nonce            hexutil.Uint64  `json:"nonce"`
	To               *common.Address `json:"to"`
	TransactionIndex hexutil.Uint    `json:"transactionIndex"`
	Value            *hexutil.Big    `json:"value"`
	V                *hexutil.Big    `json:"v"`
	R                *hexutil.Big    `json:"r"`
	S                *hexutil.Big    `json:"s"`
}

/////////////////////for btc main chain
type Scriptparm struct {
    Asm string
    Hex string
    ReqSigs int64
    Type string
    Addresses []string
}

type Voutparm struct {
    Value float64
    N int64
    ScriptPubKey Scriptparm
}

//for btc main chain noinputs
type BtcTxResInfoNoInputs struct {
    Result GetTransactionResultNoInputs
    Error error 
    Id int
}

type VinparmNoInputs struct {
    Coinbase string
    Sequence int64
}

type GetTransactionResultNoInputs struct {
    Txid string
    Hash string
    Version int64
    Size int64
    Vsize int64
    Weight int64
    Locktime int64
    Vin []VinparmNoInputs
    Vout []Voutparm
    Hex string
    Blockhash string
    Confirmations   int64
    Time            int64
    BlockTime            int64
}

//for btc main chain noinputs
type BtcTxResInfo struct {
    Result GetTransactionResult
    Error error 
    Id int
}

type ScriptSigParam struct {
    Asm string 
    Hex string
}

type Vinparm struct {
    Txid string
    Vout int64
    ScriptSig ScriptSigParam
    Sequence int64
}

type GetTransactionResult struct {
    Txid string
    Hash string
    Version int64
    Size int64
    Vsize int64
    Weight int64
    Locktime int64
    Vin []Vinparm
    Vout []Voutparm
    Hex string
    Blockhash string
    Confirmations   int64
    Time            int64
    BlockTime  int64
}

func GetLockoutConfirmations(txhash string) (bool,error) {
    if txhash == "" {
	return false,GetRetErr(ErrParamError)
    }

    reqJson2 := "{\"jsonrpc\":\"1.0\",\"method\":\"getrawtransaction\",\"params\":[\"" + txhash + "\"" + "," + "true" + "],\"id\":1}";
    s := "http://"
    s += SERVER_HOST
    s += ":"
    s += strconv.Itoa(SERVER_PORT)
    ret := DoCurlRequest(s,"",reqJson2)
    log.Debug("=============GetLockoutConfirmations,","curl ret",ret,"","=============")
    
    var btcres_noinputs BtcTxResInfoNoInputs
    ok := json.Unmarshal([]byte(ret), &btcres_noinputs)
    log.Debug("=============GetLockoutConfirmations,","ok",ok,"","=============")
    if ok == nil && btcres_noinputs.Result.Confirmations >= BTC_BLOCK_CONFIRMS {
	return true,nil
    }
    var btcres BtcTxResInfo
    ok = json.Unmarshal([]byte(ret), &btcres)
    log.Debug("=============GetLockoutConfirmations,","ok",ok,"","=============")
    if ok == nil && btcres.Result.Confirmations >= BTC_BLOCK_CONFIRMS {
	return true,nil
    }

    if ok != nil {
	return false,GetRetErr(ErrOutsideTxFail) //real fail.
    }

    return false,nil
}

func DoCurlRequest (url, api, data string) string {
    var err error
    cmd := exec.Command("/bin/sh")
    in := bytes.NewBuffer(nil)
    cmd.Stdin = in
    var out bytes.Buffer
    cmd.Stdout = &out
    go func() {
	    s := "curl --user "
	    s += USER
	    s += ":"
	    s += PASSWD
	    s += " -H 'content-type:text/plain;' "
	    str := s + url + "/" + api
	    if len(data) > 0 {
		    str = str + " --data-binary " + "'" + data + "'"
	    }
	    in.WriteString(str)
    }()
    err = cmd.Start()
    if err != nil {
	    log.Debug(err.Error())
    }
    err = cmd.Wait()
    if err != nil {
	    log.Debug("Command finished with error: %v", err)
    }
    return out.String()
}

//msgprex: tx hash
func validate_txhash(msgprex string,tx string,lockinaddr string,hashkey string,realdcrmfrom string,ch chan interface{}) {

    log.Debug("===============validate_txhash===========","lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom)
    
    signtx := new(types.Transaction)
    err := signtx.UnmarshalJSON([]byte(tx))
    if err != nil {
	log.Debug("===============validate_txhash==================","err",err,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom)
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return
    }

    payload := signtx.Data()
    realtxdata,_ := types.GetRealTxData(string(payload))
    m := strings.Split(realtxdata,":")

    var cointype string
    var realdcrmto string
    var lockinvalue string
    
    if m[0] == "LOCKIN" {
	lockinvalue = m[2]
	cointype = m[3] 
	realdcrmto = lockinaddr
    }
    if m[0] == "LOCKOUT" {
	cointype = m[3]
	lockinvalue = m[2]
	realdcrmto = m[1]
	
	if realdcrmfrom == "" {
	    log.Debug("===============validate_txhash,it is lockout,choose real dcrm from fail.==================","lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrValidateRealDcrmFromFail)}
	    ch <- res
	    return
	}
    }

    log.Debug("==========validate_txhash============","lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)

    chandler := cryptocoins.NewCryptocoinHandler(cointype)
    if chandler == nil {
	    log.Debug("============validate_txhash, new coin handler, coin type not supported=============","lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCoinTypeNotSupported)}
	    ch <- res
	    return
    }

    // new validate transaction
    from, txOutputs, jsonstring, confirmed,fee,getTxErr := chandler.GetTransactionInfo(string(hashkey))
    // timeout TODO
    if getTxErr != nil {
	    log.Debug("===============validate_txhash,GetTransactionInfo error============","from",from,"confirmed",confirmed,"getTxErr",getTxErr,"fee",fee,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
	    res := RpcDcrmRes{Ret:"",Err:getTxErr}
	    ch <- res
	    return
    }

    if confirmed == false {
	log.Debug("============validate_txhash,outside tx status is confirming ==============","fee",fee,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
	var ret2 Err
	ret2.Info = "confirming"
	res := RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return
    }

    if jsonstring != "" {
	    var args map[string]string
	    json.Unmarshal([]byte(jsonstring),&args)
	    tokentype := args["tokenType"]
	    if strings.EqualFold(cointype, tokentype) == false {
		    tokenTypeErr := fmt.Errorf("token type error,\ngot %s,\nwant %s",tokentype,cointype)
		    log.Debug("=================validate_txhash,GetTransactionInfo error=================","tokenTypeErr",tokenTypeErr,"fee",fee,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
		    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrTokenTypeError)}
		    ch <- res
		    return
	    }
    }

    // verify: 1. from == realdcrmfrom, 2. transversal txOutputs, 3. to == realdcrmto && value == lockinvalue
    // TODO verify from bug, realdcrmfrom is xxx ?
    if strings.EqualFold(realdcrmfrom,"xxx") == false && strings.EqualFold(from, realdcrmfrom) == false {
	    log.Debug("=============validate_txhash, verify from address failed.======","fee",fee,"from",from,"realdcrmfrom",realdcrmfrom,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrValidateLIFromAddrFail)}
	    ch <- res
	    return
    }

    var pass_txout = false
    log.Debug("============validate_txhash============","txOutputs length",len(txOutputs),"fee",fee,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
    log.Debug("=============validate_txhash=================","txOutputs",txOutputs,"fee",fee,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"lockin value",lockinvalue,"cointype",cointype)
    var realin *big.Int = big.NewInt(0)
    for _, txOutput := range txOutputs {
	    if strings.EqualFold(txOutput.ToAddress,realdcrmto) == false {
		    continue
	    }
	    realin = realin.Add(txOutput.Amount,realin)
	    //if vvv := txOutput.Amount.String(); strings.EqualFold(vvv,lockinvalue) {
	    //	pass_txout = true
	    //	break
	    //}
    }
    if strings.EqualFold(realin.String(),lockinvalue) {
	    pass_txout = true
    }
    if pass_txout == false {
	    // fail
	    log.Debug("==============validate_txhash,validate value fail========","realin",realin.String(),"lockin value",lockinvalue,"fee",fee,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"cointype",cointype)
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrValidateLIValueFail)}
	    ch <- res
	    return
    }
    //if confirmed == false {
//	log.Debug("======== gaozhengxin: confirming ========")
//	var ret2 Err
//	ret2.Info = "confirming"
//	res := RpcDcrmRes{Ret:"",Err:ret2}
//	ch <- res
//	return
  //  }

  //save tx fee
    if m[0] == "LOCKOUT" {
	va := fmt.Sprintf("%v",fee.Val)
	types.SetDcrmLockoutFeeData(msgprex,va)
	WriteLockoutRealFeeToLocalDB(msgprex,va)
	log.Debug("==============validate_txhash,SetDcrmLockoutFeeData========","fee",va,"lockin value",lockinvalue,"lockin tx hash",msgprex,"lockin addr",lockinaddr,"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"realdcrmto",realdcrmto,"cointype",cointype)
	// ok
	res := RpcDcrmRes{Ret:va,Err:nil}
	ch <- res
	return
    }

    // ok
    res := RpcDcrmRes{Ret:"true",Err:nil}
    ch <- res
}

type SendRawTxRes struct {
    Hash common.Hash
    Err error
}

func IsInGroup() bool {
    cnt,enode := p2pdcrm.GetGroup()
    if cnt <= 0 || enode == "" {
	return false
    }

    nodes := strings.Split(enode,common.Sep2)
    for _,node := range nodes {
	node2, _ := discover.ParseNode(node)
	if node2.ID.String() == cur_enode {
	    return true
	}
    }

    return false
}

func IsInXvcGroup() bool {
    cnt,enode := p2pdcrm.GetGroup()
    if cnt <= 0 || enode == "" {
	return false
    }

    nodes := strings.Split(enode,common.Sep2)
    for _,node := range nodes {
	node2, _ := discover.ParseNode(node)
	if node2.ID.String() == cur_enode {
	    return true
	}
    }

    return false
}

func Validate_Txhash(wr WorkReq) (string,error) {

    //////////
    if IsInGroup() == false {
	return "true",nil
    }
    //////////

    rch := make(chan interface{},1)
    req := RpcReq{rpcdata:wr,ch:rch}
    RpcReqQueue <- req
    //ret := (<- rch).(RpcDcrmRes)
    ret,cherr := GetChannelValue(ch_t,rch)
    if cherr != nil {
	log.Debug("============Validate_Txhash,","get error",cherr.Error(),"","==============")
	return "",cherr 
    }
    return ret,cherr
}

func GetEnodesInfo() {
    enode_cnts,_ = p2pdcrm.GetEnodes()
    NodeCnt = enode_cnts
    cur_enode = p2pdcrm.GetSelfID().String()
}

//error type 1
type Err struct {
	Info  string
}

func (e Err) Error() string {
	return e.Info
}

//=============================================

func dcrm_confirmaddr(msgprex string,txhash_conaddr string,tx string,fusionaddr string,dcrmaddr string,hashkey string,cointype string,ch chan interface{}) {	
    GetEnodesInfo()

    log.Debug("===== fusion.go dcrm_confirmaddr =====", "cointype", cointype)
    if cryptocoins.IsCoinSupported(cointype) == false {
            res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCoinTypeNotSupported)}
            ch <- res
            return
    }

    if strings.EqualFold(cointype,"ALL") == true {
	    dcrmaddr = strings.TrimPrefix(dcrmaddr,"0x")
	    // gaozhengxin: not expected, handled at ethapi
    }

    has,_,err := IsFusionAccountExsitDcrmAddr(fusionaddr,cointype,dcrmaddr)
    if err == nil && has == true {
	log.Debug("the dcrm addr confirm validate success.")
	res := RpcDcrmRes{Ret:"true",Err:nil}
	ch <- res
	return
    }
    
    log.Debug("the dcrm addr confirm validate fail.")
    var ret2 Err
    ret2.Info = "the dcrm addr confirm validate fail."
    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrConfirmAddrFail)}
    ch <- res
}

//ec2
//msgprex = hash 
func dcrm_liloreqAddress(msgprex string,fusionaddr string,pubkey string,cointype string,ch chan interface{}) {

    GetEnodesInfo()

    cryptocoinHandler := cryptocoins.NewCryptocoinHandler(cointype)
    if strings.EqualFold(cointype, "ALL") == false && cryptocoinHandler == nil {
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCoinTypeNotSupported)}
	ch <- res
	return
    }

    log.Debug("===========dcrm_liloreqAddress,","enode_cnts",enode_cnts,"NodeCnt",NodeCnt,"","==============")
    if int32(enode_cnts) != int32(NodeCnt) {
	log.Debug("============the group is not ready.please try again.================")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGroupNotReady)}
	ch <- res
	return
    }

    log.Debug("=========================!!!Start!!!=======================")

    log.Debug("dcrm_liloreqAddress","msgprex",msgprex)
    wk,err := FindWorker(msgprex)
    if err != nil || wk == nil {
	log.Debug("dcrm_liloreqAddress","err",err)
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return
    }
    id := wk.id

    ok := KeyGenerate_ec2(msgprex,ch,id)
    if ok == false {
	log.Debug("========dcrm_liloreqAddress,addr generate fail.=========")
	return
    }

    /*spkx,cherr := GetChannelValue(ch_t,workers[id].pkx)
    if cherr != nil {
	log.Debug("get workers[id].pkx timeout.")
	var ret2 Err
	ret2.Info = "get workers[id].pkx timeout."
	res := RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return
    }*/
    iter := workers[id].pkx.Front()
    if iter == nil {
	log.Debug("get workers[id].pkx fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetGenPubkeyFail)}
	ch <- res
	return
    }
    spkx := iter.Value.(string)
    pkx := new(big.Int).SetBytes([]byte(spkx))
    /*spky,cherr := GetChannelValue(ch_t,workers[id].pky)
    if cherr != nil {
	log.Debug("get workers[id].pky timeout.")
	var ret2 Err
	ret2.Info = "get workers[id].pky timeout."
	res := RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return
    }*/
    iter = workers[id].pky.Front()
    if iter == nil {
	log.Debug("get workers[id].pky fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetGenPubkeyFail)}
	ch <- res
	return
    }
    spky := iter.Value.(string)
    pky := new(big.Int).SetBytes([]byte(spky))
    ys := secp256k1.S256().Marshal(pkx,pky)

    //get save
    /*save,cherr := GetChannelValue(ch_t,workers[id].save)
    if cherr != nil {
	log.Debug("get workers[id].save timeout.")
	var ret2 Err
	ret2.Info = "get workers[id].save timeout."
	res := RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return
    }*/
    iter = workers[id].save.Front()
    if iter == nil {
	log.Debug("get workers[id].save fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetGenSaveDataFail)}
	ch <- res
	return
    }
    save := iter.Value.(string)

    lock.Lock()
    //write db
    dir = GetDbDir()
    db,_ := ethdb.NewLDBDatabase(dir, 0, 0)
    if db == nil {
	log.Debug("==============create db fail.============")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCreateDbFail)}
	ch <- res
	lock.Unlock()
	return
    }

    var stmp string

    pubkeyhex := hex.EncodeToString(ys)
    if cryptocoinHandler != nil {
	    var addrErr error
	    stmp, addrErr = cryptocoinHandler.PublicKeyToAddress(pubkeyhex)
	    if addrErr != nil {
	    res := RpcDcrmRes{Ret:"",Err:addrErr}
		    ch <- res
		    return
    	}
    }

    if strings.EqualFold(cointype, "ALL") == true {
	    stmp = pubkeyhex
    }
    log.Debug("+++++++++++++++ ReqAddr stmp ++++++++++++++ ","stmp",stmp)
    
    hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusionaddr) + ":" + strings.ToLower(cointype))).Hex()
    s := []string{fusionaddr,pubkey,string(ys),save,hash} ////fusionaddr ??
    ss := strings.Join(s,common.Sep)
    log.Debug("============dcrm_liloreqAddress,","stmp",stmp,"","=========")
    db.Put([]byte(stmp),[]byte(ss))

    //////////
    if stmp != "" {
	if strings.EqualFold(cointype, "ALL") == true {
	    for _, coint := range cryptocoins.Cointypes {
		if strings.EqualFold(coint, "ALL") == false {
		    h := cryptocoins.NewCryptocoinHandler(coint)
		    if h == nil {
			    log.Debug("coin type not supported","",coint)
			    continue
		    }
		    dcrmAddr, err := h.PublicKeyToAddress(stmp)
		    if err != nil || dcrmAddr == "" {
			    log.Debug("fail to calculate address","error",err)
			    continue
		    }
		    db.Put([]byte(dcrmAddr),[]byte(ss))
		} 
	    }
	} 
    }
    //////////

    res := RpcDcrmRes{Ret:stmp,Err:nil}
    ch <- res

    db.Close()
    lock.Unlock()

    ///////////////
    lock2.Lock()
    path := GetDbDirForWriteDcrmAddr()
    db,_ = ethdb.NewLDBDatabase(path, 0, 0)
    if db == nil {
	log.Debug("==============dcrm_liloreqAddress,create db fail.============")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCreateDbFail)}
	ch <- res
	lock2.Unlock()
	return
    }

    if strings.EqualFold(cointype, "ALL") == true {
	log.Debug("==============dcrm_liloreqAddress,cointype is ALL.============")
	for _, coint := range cryptocoins.Cointypes {
	    if strings.EqualFold(coint, "ALL") == false {
		h := cryptocoins.NewCryptocoinHandler(coint)
		if h == nil {
			log.Debug("coin type not supported","",coint)
			continue
		}
		dcrmAddr, err := h.PublicKeyToAddress(stmp)
		if err != nil || dcrmAddr == "" {
			log.Debug("fail to calculate address","error",err)
			continue
		}
		//WriteDcrmAddrToLocalDB(fusion,coint,dcrmAddr)
		hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusionaddr) + ":" + strings.ToLower(coint))).Hex()
		log.Debug("========dcrm_liloreqAddress,1111111==============","hash",hash,"dcrmAddr",dcrmAddr,"fusionaddr",fusionaddr,"cointype",coint)
		has,_ := db.Has([]byte(hash))
		if has != true {
		    db.Put([]byte(hash),[]byte(dcrmAddr))
		    continue
		}
		
		value,_:= db.Get([]byte(hash))
		v := string(value)
		v += ":"
		v += dcrmAddr
		db.Put([]byte(hash),[]byte(v))
	    } else {
		//WriteDcrmAddrToLocalDB(fusion,"ALL",st)
		hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusionaddr) + ":" + strings.ToLower(cointype))).Hex()
		log.Debug("========dcrm_liloreqAddress,222222==============","hash",hash,"stmp",stmp,"fusionaddr",fusionaddr,"cointype",cointype)
		has,_ := db.Has([]byte(hash))
		if has != true {
		    db.Put([]byte(hash),[]byte(stmp))
		    continue
		}
		
		value,_:= db.Get([]byte(hash))
		v := string(value)
		v += ":"
		v += stmp
		db.Put([]byte(hash),[]byte(v))
		log.Debug("========dcrm_liloreqAddress 33333333 end========")
	    }
	}
    } else {
	log.Debug("WriteDcrmAddrToLocalDB","fusionaddr",fusionaddr,"cointype",cointype,"stmp",stmp)
	//WriteDcrmAddrToLocalDB(fusion,coi,st)
	hash := crypto.Keccak256Hash([]byte(strings.ToLower(fusionaddr) + ":" + strings.ToLower(cointype))).Hex()
	log.Debug("========dcrm_liloreqAddress,3333333==============","hash",hash,"stmp",stmp,"fusionaddr",fusionaddr,"cointype",cointype)
	has,_ := db.Has([]byte(hash))
	if has != true {
	    db.Put([]byte(hash),[]byte(stmp))
	} else {
	    value,_:= db.Get([]byte(hash))
	    v := string(value)
	    v += ":"
	    v += stmp
	    db.Put([]byte(hash),[]byte(v))
	}
    }
 
    db.Close()
    lock2.Unlock()
    //////////////////
}

func validate_lockout(wsid string,txhash_lockout string,lilotx string,fusionfrom string,dcrmfrom string,realfusionfrom string,realdcrmfrom string,lockoutto string,value string,cointype string,ch chan interface{}) {
    log.Debug("=============validate_lockout============","lockout tx hash",txhash_lockout,"cointype",cointype)
    //return value--->   lockout_tx_hash:realdcrmfrom

    val,ok := GetLockoutInfoFromLocalDB(txhash_lockout)
    if ok == nil && val != "" {
	log.Debug("=================validate_lockout,this lockout tx has handle before.=================","lockout tx hash",txhash_lockout,"cointype",cointype)
	res := RpcDcrmRes{Ret:val,Err:nil}
	ch <- res
	return
    }

    var ret2 Err
    chandler := cryptocoins.NewCryptocoinHandler(cointype)
    if chandler == nil {
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCoinTypeNotSupported)}
	    ch <- res
	    return
    }
    realdcrmpubkey := ""

    lock.Lock()
    //db
    dir = GetDbDir()
    db,_ := ethdb.NewLDBDatabase(dir, 0, 0)
    if db == nil {
        log.Debug("===========validate_lockout,open db fail.=============","lockout tx hash",txhash_lockout,"cointype",cointype)
        res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCreateDbFail)}
        ch <- res
        lock.Unlock()
        return
    }
    //
    has,_ := db.Has([]byte(realdcrmfrom))
    if has == false {
        log.Debug("===========validate_lockout,can not get save data.=============","lockout tx hash",txhash_lockout,"cointype",cointype)
        res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetGenSaveDataFail)}
        ch <- res
        db.Close()
        lock.Unlock()
        return
    }

    data,_ := db.Get([]byte(realdcrmfrom))
    datas := strings.Split(string(data),common.Sep)

    realdcrmpubkey = hex.EncodeToString([]byte(datas[2]))

    db.Close()
    lock.Unlock()

    log.Debug("==========validate_lockout=============","get real dcrm pubkey",realdcrmpubkey,"lockout tx hash",txhash_lockout,"cointype",cointype)

// --------------------------------------

    amount, _ := new(big.Int).SetString(value,10)
    jsonstring := "" // TODO erc20
    // For EOS, realdcrmpubkey is needed to calculate userkey,
    // but is not used as real transaction maker.
    // The real transaction maker is eospubkey.
    var eosaccount string
    if strings.EqualFold(cointype,"EOS") {
	    //realdcrmfrom, _, _ = GetEosAccount()
	    //if realdcrmfrom == "" {
	    eosaccount, _, _ = GetEosAccount()
	    if eosaccount == "" {
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetRealEosUserFail)}
		ch <- res
		return
	    }
    }

    log.Debug("============validate_lockout===========","realdcrmfrom",realdcrmfrom,"lockout tx hash",txhash_lockout,"cointype",cointype)
    log.Debug("============validate_lockout===========","realdcrmpubkey",realdcrmpubkey,"lockout tx hash",txhash_lockout,"cointype",cointype)
    log.Debug("============validate_lockout===========","value",value,"amount",amount,"lockout tx hash",txhash_lockout,"cointype",cointype)

    var lockouttx interface{}
    var digests []string
    var buildTxErr error
    if strings.EqualFold(cointype,"EOS") {
    	lockouttx, digests, buildTxErr = chandler.BuildUnsignedTransaction(eosaccount,realdcrmpubkey,lockoutto,amount,jsonstring)
    } else {
	lockouttx, digests, buildTxErr = chandler.BuildUnsignedTransaction(realdcrmfrom,realdcrmpubkey,lockoutto,amount,jsonstring)
    }
    
    log.Debug("==========validate_lockout==========","unsigned outside tx hash",lockouttx,"lockout tx hash",txhash_lockout,"cointype",cointype)
    
    if buildTxErr != nil {
	    res := RpcDcrmRes{Ret:"",Err:buildTxErr}
	    ch <- res
	    return
    }
    rch := make(chan interface{}, 1)
    var sigs []string
    var bak_sigs []string
    for k, digest := range digests {
	    log.Debug("========validate_lockout=========","call dcrm_sign times",k,"lockout tx hash",txhash_lockout,"cointype",cointype)

	    bak_sig := dcrm_sign(wsid,"xxx",digest,realdcrmfrom,cointype,rch)
	    ret,cherr := GetChannelValue(ch_t,rch)
	    if cherr != nil {
		    res := RpcDcrmRes{Ret:"",Err:cherr}
		    ch <- res
		    return
	    }
	    //bug
	    rets := []rune(ret)
	    if len(rets) != 130 {
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrDcrmSigWrongSize)}
		ch <- res
		return
	    }
	    sigs = append(sigs, string(ret))
	    if bak_sig != "" {
		bak_sigs = append(bak_sigs, bak_sig)
	    }
    }

    log.Debug("==========validate_lockout==========","get dcrm sigs",sigs,"get dcrm bak sigs",bak_sigs,"lockout tx hash",txhash_lockout,"cointype",cointype)
    
    signedTx, err := chandler.MakeSignedTransaction(sigs, lockouttx)
    if err != nil {
	    ret2.Info = err.Error()
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
    }

    log.Debug("===========validate_lockout============","signed outside Tx",signedTx,"lockout tx hash",txhash_lockout,"cointype",cointype)
    
    lockout_tx_hash, err := chandler.SubmitTransaction(signedTx)
    
    log.Debug("===========validate_lockout============","submit to outside tx hash",lockout_tx_hash,"err",err,"lockout tx hash",txhash_lockout,"cointype",cointype)

    /////////add for bak sig
    if err != nil && len(bak_sigs) != 0 {
	log.Debug("===========validate_lockout,send tx to outside net fail,try bak sig=============.","lockout tx hash",txhash_lockout,"cointype",cointype)

	signedTx, err = chandler.MakeSignedTransaction(bak_sigs, lockouttx)
	if err != nil {
		ret2.Info = err.Error()
		res := RpcDcrmRes{Ret:"",Err:ret2}
		ch <- res
		return
	}
	log.Debug("===========validate_lockout,for bak  sign ============","signed outside Tx",signedTx,"lockout tx hash",txhash_lockout,"cointype",cointype)
	
	lockout_tx_hash, err = chandler.SubmitTransaction(signedTx)
	
	log.Debug("===========validate_lockout,for bak sign ============","submit to outside tx hash",lockout_tx_hash,"err",err,"lockout tx hash",txhash_lockout,"cointype",cointype)
    }
    /////////

    if lockout_tx_hash != "" {
	retva := lockout_tx_hash + common.Sep10 + realfusionfrom + common.Sep10 + realdcrmfrom
	WriteLockoutInfoToLocalDB(txhash_lockout,retva)
	res := RpcDcrmRes{Ret:retva,Err:err}
	ch <- res
	return
    }

    if err != nil {
	    ret2.Info = err.Error()
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
    }
    
    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSendTxToNetFail)}
    ch <- res
    return
}

//ec2
//msgprex = hash 
//return value is the backup for dcrm sig.
func dcrm_sign(msgprex string,sig string,txhash string,dcrmaddr string,cointype string,ch chan interface{}) string {

    if strings.EqualFold(cointype,"EOS") == true {
	lock.Lock()
	//db
	dir = GetEosDbDir()
	db,_ := ethdb.NewLDBDatabase(dir, 0, 0)
	if db == nil {
	    log.Debug("===========open db fail.=============")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCreateDbFail)}
	    ch <- res
	    lock.Unlock()
	    return ""
	}
	// Retrieve eospubkey
	eosstr,_ := db.Get([]byte("eossettings"))
	eosstrs := strings.Split(string(eosstr),":")
	log.Debug("======== get eos settings, ","eosstr",eosstr,"","========")
	if len(eosstrs) != 5 {
	    var ret2 Err
	    ret2.Info = "get eos settings error: "+string(eosstr)
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return ""
	}
	pubhex := eosstrs[3]
	dcrmpks, _ := hex.DecodeString(pubhex)
	dcrmpkx,dcrmpky := secp256k1.S256().Unmarshal(dcrmpks[:])
	dcrmaddr = pubhex
	db.Close()
	lock.Unlock()
	log.Debug("======== dcrm_sign eos","pkx",dcrmpkx,"pkx",dcrmpky,"","========")

	lock.Lock()
	dir = GetDbDir()
	db,_ = ethdb.NewLDBDatabase(dir, 0, 0)
	if db == nil {
	    log.Debug("===========open db fail.=============")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCreateDbFail)}
	    ch <- res
	    lock.Unlock()
	    return ""
	}

	log.Debug("======== get dcrm data,","dcrmaddr",dcrmaddr,"","========")
	data,_ := db.Get([]byte(dcrmaddr))
	datas := strings.Split(string(data),common.Sep)
	log.Debug("======== get dcrm data,","datas",datas,"","========")
	save := datas[3] 

	txhashs := []rune(txhash)
	if string(txhashs[0:2]) == "0x" {
		txhash = string(txhashs[2:])
	}

	db.Close()
	lock.Unlock()

	w,err := FindWorker(msgprex)
	if w == nil || err != nil {
	    log.Debug("===========get worker fail.=============")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrNoFindWorker)}
	    ch <- res
	    return ""
	}
	id := w.id

	var ch1 = make(chan interface{}, 1)
	var flag = false
	var ret string
	var bak_sig string
	//25-->1
	for i := 0; i < 1; i++ {
		bak_sig = Sign_ec2(msgprex,save,txhash,cointype,dcrmpkx,dcrmpky,ch1,id)
		ret,_ = GetChannelValue(ch_t,ch1)
		log.Debug("======== dcrm_sign eos","signature",ret,"bak sig",bak_sig,"","========")
		//if ret != "" && eos.IsCanonical([]byte(ret)) == true {
		if ret == "" {
			w := workers[id]
			w.Clear2()
			continue
		}
		b, _ := hex.DecodeString(ret)
		if eos.IsCanonical(b) == true {
			flag = true
			break
		}
		w := workers[id]
		w.Clear2()
	}
	if flag == false {
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrDcrmSigFail)}
		ch <- res
		return ""
	}

	res := RpcDcrmRes{Ret:ret,Err:nil}
	ch <- res
	return bak_sig
    }
    // TODO emend signature for TRON and other okashii protocols
    av := cryptocoins.NewAddressValidator(cointype)
    match := av.IsValidAddress(dcrmaddr)
    if match == false {
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrInvalidDcrmAddr)}
	    ch <- res
	    return ""
    }

    GetEnodesInfo() 
    
    if int32(enode_cnts) != int32(NodeCnt) {
	log.Debug("============the net group is not ready.please try again.================")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGroupNotReady)}
	ch <- res
	return ""
    }

    log.Debug("===================!!!Start!!!====================")

    lock.Lock()
    //db
    dir = GetDbDir()
    db,_ := ethdb.NewLDBDatabase(dir, 0, 0)
    if db == nil {
	log.Debug("===========open db fail.=============")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrCreateDbFail)}
	ch <- res
	lock.Unlock()
	return ""
    }
    //
    log.Debug("=========dcrm_sign,","dcrmaddr",dcrmaddr,"","==============")
    has,_ := db.Has([]byte(dcrmaddr))
    if has == false {
	log.Debug("===========get generate save data fail.=============")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetGenSaveDataFail)}
	ch <- res
	db.Close()
	lock.Unlock()
	return ""
    }

    data,_ := db.Get([]byte(dcrmaddr))
    datas := strings.Split(string(data),common.Sep)

    save := datas[3] 
    
    dcrmpub := datas[2]
    dcrmpks := []byte(dcrmpub)
    dcrmpkx,dcrmpky := secp256k1.S256().Unmarshal(dcrmpks[:])

    txhashs := []rune(txhash)
    if string(txhashs[0:2]) == "0x" {
	txhash = string(txhashs[2:])
    }

    db.Close()
    lock.Unlock()

    w,err := FindWorker(msgprex)
    if w == nil || err != nil {
	log.Debug("===========get worker fail.=============")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrNoFindWorker)}
	ch <- res
	return ""
    }
    id := w.id

    //Sign_ec2(msgprex,save,txhash,cointype,dcrmpkx,dcrmpky,ch,id)
    if strings.EqualFold(cointype,"EVT1") == true {
	log.Debug("======== dcrm_sign ready to call Sign_ec2","msgprex",msgprex,"save",save,"txhash",txhash,"cointype",cointype,"pkx",dcrmpkx,"pky",dcrmpky,"id",id)
	log.Debug("!!! token type is EVT1 !!!")
	var ch1 = make(chan interface{}, 1)
	var flag = false
	var ret string
	var cherr error
	var bak_sig string
	//25-->1
	for i := 0; i < 1; i++ {
		bak_sig = Sign_ec2(msgprex,save,txhash,cointype,dcrmpkx,dcrmpky,ch1,id)
		ret, cherr = GetChannelValue(ch_t,ch1)
		if cherr != nil {
			log.Debug("======== dcrm_sign evt","cherr",cherr)
			time.Sleep(time.Duration(1)*time.Second) //1000 == 1s
			w := workers[id]
			w.Clear2()
			continue
		}
		log.Debug("======== dcrm_sign evt","signature",ret,"","========")
		//if ret != "" && eos.IsCanonical([]byte(ret)) == true {
		if ret == "" {
			w := workers[id]
			w.Clear2()
			continue
		}
		b, _ := hex.DecodeString(ret)
		if eos.IsCanonical(b) == true {
			fmt.Printf("\nret is a canonical signature\n")
			flag = true
			break
		}
		w := workers[id]
		w.Clear2()
	}
	log.Debug("======== dcrm_sign evt","got rsv flag",flag,"ret",ret,"","========")
	if flag == false {
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrDcrmSigFail)}
		ch <- res
		return ""
	}
	//ch <- ret
	res := RpcDcrmRes{Ret:ret,Err:cherr}
	ch <- res
	return bak_sig
    } else {
	    bak_sig := Sign_ec2(msgprex,save,txhash,cointype,dcrmpkx,dcrmpky,ch,id)
	    return bak_sig
    }

    return ""
}

//msg:  hash-enode:C1:X1:X2
func DisMsg(msg string) {

    if msg == "" {
	return
    }

    //orderbook matchres
    mm := strings.Split(msg, common.Sep)
    if len(mm) < 3 {
	return
    }
    
    mms := mm[0]
    prexs := strings.Split(mms,"-")
    if len(prexs) < 2 {
	return
    }

    if mm[1] == "GETRES" {
	/*log.Debug("=========DisMsg,it is GETRES.=============")

	ress := strings.Split(mm[2],sepob)
	trade := ress[3]
	if  strings.EqualFold(trade,"ETH/BTC") {
	    msg_matchres_eth_btc <-ress[2]
	    if len(msg_matchres_eth_btc) == (NodeCnt-1) {
		log.Debug("=========DisMsg,get msg.=============")
		bmatchres_eth_btc <- true
	    }
	} else if  strings.EqualFold(trade,"FSN/BTC") {
	    msg_matchres_fsn_btc <-ress[2]
	    if len(msg_matchres_fsn_btc) == (NodeCnt-1) {
		log.Debug("=========DisMsg,get msg.=============")
		bmatchres_fsn_btc <- true
	    }
	} else if  strings.EqualFold(trade,"FSN/ETH") {
	    msg_matchres_fsn_eth <-ress[2]
	    if len(msg_matchres_fsn_eth) == (NodeCnt-1) {
		log.Debug("=========DisMsg,get msg.=============")
		bmatchres_fsn_eth <- true
	    }
	}*/	

	return
    }

    //ec2
    IsEc2 := true
    if IsEc2 == true {
	//msg:  hash-enode:C1:X1:X2

	w,err := FindWorker(prexs[0])
	if err != nil || w == nil {
	    return
	}
	//log.Debug("=============DisMsg============","msg prex",prexs[0],"get worker id",w.id)

	msgCode := mm[1]
	//log.Debug("=========DisMsg,it is ec2.=============","msgCode",msgCode)
	switch msgCode {
	case "C1":
	    //log.Debug("=========DisMsg,it is ec2 and it is C1.=============","len msg_c1",w.msg_c1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_c1.Len() >= (NodeCnt-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_c1.PushBack(msg)
	    //log.Debug("=========DisMsg,C1 msg.=============","len c1",w.msg_c1.Len(),"nodecnt-1",(NodeCnt-1))
	    if w.msg_c1.Len() == (NodeCnt-1) {
		log.Debug("=========DisMsg,get all C1 msg.=============")
		w.bc1 <- true
	    }
	case "D1":
	    //log.Debug("=========DisMsg,it is ec2 and it is D1.=============","len msg_d1_1",w.msg_d1_1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_d1_1.Len() >= (NodeCnt-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_d1_1.PushBack(msg)
	    //log.Debug("=========DisMsg,D1 msg.=============","len d1",w.msg_d1_1.Len(),"nodecnt-1",(NodeCnt-1))
	    if w.msg_d1_1.Len() == (NodeCnt-1) {
		log.Debug("=========DisMsg,get all D1 msg.=============")
		w.bd1_1 <- true
	    }
	case "SHARE1":
	    //log.Debug("=========DisMsg,it is ec2 and it is SHARE1.=============","len msg_share1",w.msg_share1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_share1.Len() >= (NodeCnt-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_share1.PushBack(msg)
	    //log.Debug("=========DisMsg,SHARE1 msg.=============","len share1",w.msg_share1.Len(),"nodecnt-1",(NodeCnt-1))
	    if w.msg_share1.Len() == (NodeCnt-1) {
		log.Debug("=========DisMsg,get all SHARE1 msg.=============")
		w.bshare1 <- true
	    }
	case "ZKFACTPROOF":
	    //log.Debug("=========DisMsg,it is ec2 and it is ZKFACTPROOF.=============","len msg_zkfact",w.msg_zkfact.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_zkfact.Len() >= (NodeCnt-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_zkfact.PushBack(msg)
	    //log.Debug("=========DisMsg,ZKFACTPROOF msg.=============","len msg_zkfact",w.msg_zkfact.Len(),"nodecnt-1",(NodeCnt-1))
	    if w.msg_zkfact.Len() == (NodeCnt-1) {
		log.Debug("=========DisMsg,get all ZKFACTPROOF msg.=============")
		w.bzkfact <- true
	    }
	case "ZKUPROOF":
	    //log.Debug("=========DisMsg,it is ec2 and it is ZKUPROOF.=============","len msg_zku",w.msg_zku.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_zku.Len() >= (NodeCnt-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_zku.PushBack(msg)
	    //log.Debug("=========DisMsg,ZKUPROOF msg.=============","len msg_zku",w.msg_zku.Len(),"nodecnt-1",(NodeCnt-1))
	    if w.msg_zku.Len() == (NodeCnt-1) {
		log.Debug("=========DisMsg,get all ZKUPROOF msg.=============")
		w.bzku <- true
	    }
	case "MTAZK1PROOF":
	    //log.Debug("=========DisMsg,it is ec2 and it is MTAZK1PROOF.=============","len msg_mtazk1proof",w.msg_mtazk1proof.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_mtazk1proof.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_mtazk1proof.PushBack(msg)
	    //log.Debug("=========DisMsg,MTAZK1PROOF msg.=============","len msg_mtazk1proof",w.msg_mtazk1proof.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_mtazk1proof.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all MTAZK1PROOF msg.=============")
		w.bmtazk1proof <- true
	    }
	    //sign
       case "C11":
	    //log.Debug("=========DisMsg,it is ec2 and it is C11.=============","len msg_c11",w.msg_c11.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_c11.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_c11.PushBack(msg)
	    //log.Debug("=========DisMsg,C11 msg.=============","len msg_c11",w.msg_c11.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_c11.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all C11 msg.=============")
		w.bc11 <- true
	    }
       case "KC":
	    //log.Debug("=========DisMsg,it is ec2 and it is KC.=============","len msg_kc",w.msg_kc.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_kc.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_kc.PushBack(msg)
	    //log.Debug("=========DisMsg,KC msg.=============","len msg_kc",w.msg_kc.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_kc.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all KC msg.=============")
		w.bkc <- true
	    }
       case "MKG":
	    //log.Debug("=========DisMsg,it is ec2 and it is MKG.=============","len msg_mkg",w.msg_mkg.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_mkg.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_mkg.PushBack(msg)
	    //log.Debug("=========DisMsg,MKG msg.=============","len msg_mkg",w.msg_mkg.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_mkg.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all MKG msg.=============")
		w.bmkg <- true
	    }
       case "MKW":
	    //log.Debug("=========DisMsg,it is ec2 and it is MKW.=============","len msg_mkw",w.msg_mkw.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_mkw.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_mkw.PushBack(msg)
	    //log.Debug("=========DisMsg,MKW msg.=============","len msg_mkw",w.msg_mkw.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_mkw.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all MKW msg.=============")
		w.bmkw <- true
	    }
       case "DELTA1":
	    //log.Debug("=========DisMsg,it is ec2 and it is DELTA1.=============","len msg_delta1",w.msg_delta1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_delta1.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_delta1.PushBack(msg)
	    //log.Debug("=========DisMsg,DELTA1 msg.=============","len msg_delta1",w.msg_delta1.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_delta1.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all DELTA1 msg.=============")
		w.bdelta1 <- true
	    }
	case "D11":
	    //log.Debug("=========DisMsg,it is ec2 and it is D11.=============","len msg_d11_1",w.msg_d11_1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_d11_1.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_d11_1.PushBack(msg)
	    //log.Debug("=========DisMsg,D11 msg.=============","len msg_d11_1",w.msg_d11_1.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_d11_1.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all D11 msg.=============")
		w.bd11_1 <- true
	    }
	case "S1":
	    //log.Debug("=========DisMsg,it is ec2 and it is S1.=============","len msg_s1",w.msg_s1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_s1.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_s1.PushBack(msg)
	    //log.Debug("=========DisMsg,S1 msg.=============","len msg_s1",w.msg_s1.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_s1.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all S1 msg.=============")
		w.bs1 <- true
	    }
	case "SS1":
	    //log.Debug("=========DisMsg,it is ec2 and it is SS1.=============","len msg_ss1",w.msg_ss1.Len(),"len msg",len(msg))
	    ///bug
	    if w.msg_ss1.Len() >= (ThresHold-1) {
		//w.Clear()
		return
	    }
	    ///
	    w.msg_ss1.PushBack(msg)
	    //log.Debug("=========DisMsg,SS1 msg.=============","len msg_ss1",w.msg_ss1.Len(),"ThresHold-1",(ThresHold-1))
	    if w.msg_ss1.Len() == (ThresHold-1) {
		log.Debug("=========DisMsg,get all SS1 msg.=============")
		w.bss1 <- true
	    }
	default:
	    log.Debug("unkown msg code")
	}

	return
    }
}

//==================node in group callback=================================
func SetUpMsgList(msg string) {

    //log.Debug("==========SetUpMsgList,","receiv msg",msg,"","===================")
    mm := strings.Split(msg,"dcrmslash")
    if len(mm) >= 2 {
	receiveSplitKey(msg)
	return
    }

    v := RecvMsg{msg:msg}
    //rpc-req
    rch := make(chan interface{},1)
    //req := RpcReq{rpcstr:msg,ch:rch}
    req := RpcReq{rpcdata:&v,ch:rch}
    RpcReqQueue <- req
}
//==========================================================

type AccountListJson struct {
    ACCOUNTLIST []AccountListInfo
}

type AccountListInfo struct {
    COINTYPE string
    DCRMADDRESS string
    DCRMPUBKEY string
}
//============================================================
func Dcrm_LiLoReqAddress(wr WorkReq) (string, error) {
    rch := make(chan interface{},1)
    req := RpcReq{rpcdata:wr,ch:rch}
    RpcReqQueue <- req
    ret,cherr := GetChannelValue(ch_t,rch)
    if cherr != nil {
	log.Debug("Dcrm_LiLoReqAddress timeout.")
	return "",GetRetErr(ErrReqAddrTimeout)
    }
    return ret,cherr
}

func Dcrm_Sign(wr WorkReq) (string,error) {
    rch := make(chan interface{},1)
    req := RpcReq{rpcdata:wr,ch:rch}
    RpcReqQueue <- req
    ret,cherr := GetChannelValue(ch_t,rch)
    if cherr != nil {
	log.Debug("Dcrm_Sign get timeout.")
	log.Debug(cherr.Error())
	return "",cherr
    }
    return ret,cherr
    //rpc-req
}
//==============================================================

func IsCurNode(enodes string,cur string) bool {
    if enodes == "" || cur == "" {
	return false
    }

    s := []rune(enodes)
    en := strings.Split(string(s[8:]),"@")
    //log.Debug("=======IsCurNode,","en[0]",en[0],"cur",cur,"","============")
    if en[0] == cur {
	return true
    }

    return false
}

func DoubleHash(id string) *big.Int {
    // Generate the random num
    //rnd := random.GetRandomInt(256)

    // First, hash with the keccak256
    keccak256 := sha3.NewKeccak256()
    //keccak256.Write(rnd.Bytes())

    keccak256.Write([]byte(id))

    digestKeccak256 := keccak256.Sum(nil)

    //second, hash with the SHA3-256
    sha3256 := sha3.New256()

    sha3256.Write(digestKeccak256)

    digest := sha3256.Sum(nil)

    // convert the hash ([]byte) to big.Int
    digestBigInt := new(big.Int).SetBytes(digest)
    return digestBigInt
}

func Tool_DecimalByteSlice2HexString(DecimalSlice []byte) string {
    var sa = make([]string, 0)
    for _, v := range DecimalSlice {
        sa = append(sa, fmt.Sprintf("%02X", v))
    }
    ss := strings.Join(sa, "")
    return ss
}

func GetSignString(r *big.Int,s *big.Int,v int32,i int) string {
    rr :=  r.Bytes()
    sss :=  s.Bytes()

    //bug
    if len(rr) == 31 && len(sss) == 32 {
	log.Debug("======r len is 31===========")
	sigs := make([]byte,65)
	sigs[0] = byte(0)
	math.ReadBits(r,sigs[1:32])
	math.ReadBits(s,sigs[32:64])
	sigs[64] = byte(i)
	ret := Tool_DecimalByteSlice2HexString(sigs)
	return ret
    }
    if len(rr) == 31 && len(sss) == 31 {
	log.Debug("======r and s len is 31===========")
	sigs := make([]byte,65)
	sigs[0] = byte(0)
	sigs[32] = byte(0)
	math.ReadBits(r,sigs[1:32])
	math.ReadBits(s,sigs[33:64])
	sigs[64] = byte(i)
	ret := Tool_DecimalByteSlice2HexString(sigs)
	return ret
    }
    if len(rr) == 32 && len(sss) == 31 {
	log.Debug("======s len is 31===========")
	sigs := make([]byte,65)
	sigs[32] = byte(0)
	math.ReadBits(r,sigs[0:32])
	math.ReadBits(s,sigs[33:64])
	sigs[64] = byte(i)
	ret := Tool_DecimalByteSlice2HexString(sigs)
	return ret
    }
    //

    n := len(rr) + len(sss) + 1
    sigs := make([]byte,n)
    math.ReadBits(r,sigs[0:len(rr)])
    math.ReadBits(s,sigs[len(rr):len(rr)+len(sss)])

    sigs[len(rr)+len(sss)] = byte(i)
    ret := Tool_DecimalByteSlice2HexString(sigs)

    return ret
}

func Verify(r *big.Int,s *big.Int,v int32,message string,pkx *big.Int,pky *big.Int) bool {
    return Verify2(r,s,v,message,pkx,pky)
}

func GetEnodesByUid(uid *big.Int) string {
    _,nodes := p2pdcrm.GetEnodes()
    others := strings.Split(nodes,common.Sep2)
    for _,v := range others {
	id := DoubleHash(v)
	if id.Cmp(uid) == 0 {
	    return v
	}
    }

    return ""
}

type sortableIDSSlice []*big.Int

func (s sortableIDSSlice) Len() int {
	return len(s)
}

func (s sortableIDSSlice) Less(i, j int) bool {
	return s[i].Cmp(s[j]) <= 0
}

func (s sortableIDSSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func GetIds() sortableIDSSlice {
    var ids sortableIDSSlice
    _,nodes := p2pdcrm.GetEnodes()
    others := strings.Split(nodes,common.Sep2)
    for _,v := range others {
	uid := DoubleHash(v)
	ids = append(ids,uid)
    }
    sort.Sort(ids)
    return ids
}

//ec2
//msgprex = hash 
func KeyGenerate_ec2(msgprex string,ch chan interface{},id int) bool {
    gc := getgroupcount()
    if id < 0 || id >= RpcMaxWorker || id >= len(workers) || gc <= 0 {
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return false
    }

    w := workers[id]
    ns,_ := p2pdcrm.GetEnodes()
    if ns != NodeCnt {
	log.Debug("KeyGenerate_ec2,get nodes info error.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGroupNotReady)}
	ch <- res
	return false 
    }

    //1. generate their own "partial" private key secretly
    u1 := random.GetRandomIntFromZn(secp256k1.S256().N)

    // 2. calculate "partial" public key, make "pritial" public key commiment to get (C,D)
    u1Gx, u1Gy := secp256k1.S256().ScalarBaseMult(u1.Bytes())
    commitU1G := new(commit.Commitment).Commit(u1Gx, u1Gy)

    // 3. generate their own paillier public key and private key
    u1PaillierPk, u1PaillierSk := paillier.GenerateKeyPair(PaillierKeyLength)

    // 4. Broadcast
    // commitU1G.C, commitU2G.C, commitU3G.C, commitU4G.C, commitU5G.C
    // u1PaillierPk, u2PaillierPk, u3PaillierPk, u4PaillierPk, u5PaillierPk
    mp := []string{msgprex,cur_enode}
    enode := strings.Join(mp,"-")
    s0 := "C1"
    s1 := string(commitU1G.C.Bytes())
    s2 := u1PaillierPk.Length
    s3 := string(u1PaillierPk.N.Bytes()) 
    s4 := string(u1PaillierPk.G.Bytes()) 
    s5 := string(u1PaillierPk.N2.Bytes()) 
    ss := enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2 + common.Sep + s3 + common.Sep + s4 + common.Sep + s5
    log.Debug("================kg ec2 round one,send msg,code is C1==================")
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    // commitU1G.C, commitU2G.C, commitU3G.C, commitU4G.C, commitU5G.C
    // u1PaillierPk, u2PaillierPk, u3PaillierPk, u4PaillierPk, u5PaillierPk
     _,cherr := GetChannelValue(ch_t,w.bc1)
    if cherr != nil {
	log.Debug("get w.bc1 timeout.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetC1Timeout)}
	ch <- res
	return false 
    }
    log.Debug("================kg ec2 round one,receive msg,code is C1==================")

    // 2. generate their vss to get shares which is a set
    // [notes]
    // all nodes has their own id, in practival, we can take it as double hash of public key of fusion

    ids := GetIds()
    log.Debug("=========KeyGenerate_ec2========","ids",ids)

    u1PolyG, _, u1Shares, err := vss.Vss(u1, ids, ThresHold, NodeCnt)
    if err != nil {
	log.Debug(err.Error())
	res := RpcDcrmRes{Ret:"",Err:err}
	ch <- res
	return false 
    }
    log.Debug("================kg ec2 round one,get polyG/shares success.==================")

    // 3. send the the proper share to proper node 
    //example for u1:
    // Send u1Shares[0] to u1
    // Send u1Shares[1] to u2
    // Send u1Shares[2] to u3
    // Send u1Shares[3] to u4
    // Send u1Shares[4] to u5
    for _,id := range ids {
	enodes := GetEnodesByUid(id)

	if enodes == "" {
	    log.Debug("=========KeyGenerate_ec2,don't find proper enodes========")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetEnodeByUIdFail)}
	    ch <- res
	    return false
	}
	
	if IsCurNode(enodes,cur_enode) {
	    continue
	}

	for _,v := range u1Shares {
	    uid := vss.GetSharesId(v)
	    log.Debug("================kg ec2 round two,send msg,code is SHARE1==================","uid",uid,"id",id)
	    if uid.Cmp(id) == 0 {
		mp := []string{msgprex,cur_enode}
		enode := strings.Join(mp,"-")
		s0 := "SHARE1"
		s1 := strconv.Itoa(v.T) 
		s2 := string(v.Id.Bytes()) 
		s3 := string(v.Share.Bytes()) 
		ss := enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2 + common.Sep + s3
		log.Debug("================kg ec2 round two,send msg,code is SHARE1==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
		SendMsgToPeer(enodes,ss)
		break
	    }
	}
    }
    log.Debug("================kg ec2 round two,send share success.==================")

    // 4. Broadcast
    // commitU1G.D, commitU2G.D, commitU3G.D, commitU4G.D, commitU5G.D
    // u1PolyG, u2PolyG, u3PolyG, u4PolyG, u5PolyG
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "D1"
    dlen := len(commitU1G.D)
    s1 = strconv.Itoa(dlen)

    ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep
    for _,d := range commitU1G.D {
	ss += string(d.Bytes())
	ss += common.Sep
    }

    s2 = strconv.Itoa(u1PolyG.T)
    s3 = strconv.Itoa(u1PolyG.N)
    ss = ss + s2 + common.Sep + s3 + common.Sep

    pglen := 2*(len(u1PolyG.PolyG))
    log.Debug("=========KeyGenerate_ec2,","pglen",pglen,"","==========")
    s4 = strconv.Itoa(pglen)

    ss = ss + s4 + common.Sep

    for _,p := range u1PolyG.PolyG {
	for _,d := range p {
	    ss += string(d.Bytes())
	    ss += common.Sep
	}
    }
    ss = ss + "NULL"
    log.Debug("================kg ec2 round three,send msg,code is D1==================")
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    // commitU1G.D, commitU2G.D, commitU3G.D, commitU4G.D, commitU5G.D
    // u1PolyG, u2PolyG, u3PolyG, u4PolyG, u5PolyG
    _,cherr = GetChannelValue(ch_t,w.bd1_1)
    if cherr != nil {
	log.Debug("get w.bd1_1 timeout in keygenerate.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetD1Timeout)}
	ch <- res
	return false 
    }
    log.Debug("================kg ec2 round three,receiv msg,code is D1.==================")

    // 2. Receive Personal Data
    _,cherr = GetChannelValue(ch_t,w.bshare1)
    if cherr != nil {
	log.Debug("get w.bshare1 timeout in keygenerate.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetSHARE1Timeout)}
	ch <- res
	return false 
    }
    log.Debug("================kg ec2 round three,receiv msg,code is SHARE1.==================")
	 
    //var i int
    shares := make([]string,NodeCnt-1)
    /*for i=0;i<(NodeCnt-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_share1)
	if cherr != nil {
	    log.Debug("get w.msg_share1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_share1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return false
	}
	shares[i] = v
    }*/
    if w.msg_share1.Len() != (NodeCnt-1) {
	log.Debug("get w.msg_share1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllSHARE1Fail)}
	ch <- res
	return false
    }
    itmp := 0
    iter := w.msg_share1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	shares[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }
    
    var sstruct = make(map[string]*vss.ShareStruct)
    for _,v := range shares {
	mm := strings.Split(v, common.Sep)
	t,_ := strconv.Atoi(mm[2])
	ushare := &vss.ShareStruct{T:t,Id:new(big.Int).SetBytes([]byte(mm[3])),Share:new(big.Int).SetBytes([]byte(mm[4]))}
	prex := mm[0]
	prexs := strings.Split(prex,"-")
	sstruct[prexs[len(prexs)-1]] = ushare
    }
    for _,v := range u1Shares {
	uid := vss.GetSharesId(v)
	enodes := GetEnodesByUid(uid)
	//en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    sstruct[cur_enode] = v 
	    break
	}
    }

    ds := make([]string,NodeCnt-1)
    /*for i=0;i<(NodeCnt-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_d1_1)
	if cherr != nil {
	    log.Debug("get w.msg_d1_1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_d1_1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return false
	}
	ds[i] = v
    }*/
    if w.msg_d1_1.Len() != (NodeCnt-1) {
	log.Debug("get w.msg_d1_1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllD1Fail)}
	ch <- res
	return false
    }
    itmp = 0
    iter = w.msg_d1_1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	ds[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    var upg = make(map[string]*vss.PolyGStruct)
    for _,v := range ds {
	mm := strings.Split(v, common.Sep)
	dlen,_ := strconv.Atoi(mm[2])
	pglen,_ := strconv.Atoi(mm[3+dlen+2])
	pglen = (pglen/2)
	var pgss = make([][]*big.Int, 0)
	l := 0
	for j:=0;j<pglen;j++ {
	    l++
	    var gg = make([]*big.Int,0)
	    gg = append(gg,new(big.Int).SetBytes([]byte(mm[5+dlen+l])))
	    l++
	    gg = append(gg,new(big.Int).SetBytes([]byte(mm[5+dlen+l])))
	    pgss = append(pgss,gg)
	    log.Debug("=========KeyGenerate_ec2,","gg",gg,"pgss",pgss,"","========")
	}

	t,_ := strconv.Atoi(mm[3+dlen])
	n,_ := strconv.Atoi(mm[4+dlen])
	ps := &vss.PolyGStruct{T:t,N:n,PolyG:pgss}
	//pstruct = append(pstruct,ps)
	prex := mm[0]
	prexs := strings.Split(prex,"-")
	upg[prexs[len(prexs)-1]] = ps
    }
    upg[cur_enode] = u1PolyG

    // 3. verify the share
    log.Debug("[Key Generation ec2][Round 3] 3. u1 verify share:")
    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if sstruct[en[0]].Verify(upg[en[0]]) == false {
	    log.Debug("u1 verify share fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifySHARE1Fail)}
	    ch <- res
	    return false
	}
    }

    // 4.verify and de-commitment to get uG
    // for all nodes, construct the commitment by the receiving C and D
    cs := make([]string,NodeCnt-1)
    /*for i=0;i<(NodeCnt-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_c1)
	if cherr != nil {
	    log.Debug("get w.msg_c1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_c1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return false
	}
	cs[i] = v
    }*/
    if w.msg_c1.Len() != (NodeCnt-1) {
	log.Debug("get w.msg_c1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllC1Fail)}
	ch <- res
	return false
    }
    itmp = 0
    iter = w.msg_c1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	cs[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    var udecom = make(map[string]*commit.Commitment)
    for _,v := range cs {
	mm := strings.Split(v, common.Sep)
	prex := mm[0]
	prexs := strings.Split(prex,"-")
	for _,vv := range ds {
	    mmm := strings.Split(vv, common.Sep)
	    prex2 := mmm[0]
	    prexs2 := strings.Split(prex2,"-")
	    if prexs[len(prexs)-1] == prexs2[len(prexs2)-1] {
		dlen,_ := strconv.Atoi(mmm[2])
		var gg = make([]*big.Int,0)
		l := 0
		for j:=0;j<dlen;j++ {
		    l++
		    gg = append(gg,new(big.Int).SetBytes([]byte(mmm[2+l])))
		}
		deCommit := &commit.Commitment{C:new(big.Int).SetBytes([]byte(mm[2])), D:gg}
		log.Debug("=========KeyGenerate_ec2,","deCommit",deCommit,"","==========")
		udecom[prexs[len(prexs)-1]] = deCommit
		break
	    }
	}
    }
    deCommit_commitU1G := &commit.Commitment{C: commitU1G.C, D: commitU1G.D}
    udecom[cur_enode] = deCommit_commitU1G

    // for all nodes, verify the commitment
    log.Debug("[Key Generation ec2][Round 3] 4. all nodes verify commit:")
    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	log.Debug("===========KeyGenerate_ec2,","node",en[0],"deCommit",udecom[en[0]],"","==============")
	if udecom[en[0]].Verify() == false {
	    log.Debug("u1 verify commit in keygenerate fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrKeyGenVerifyCommitFail)}
	    ch <- res
	    return false
	}
    }

    // for all nodes, de-commitment
    var ug = make(map[string][]*big.Int)
    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	_, u1G := udecom[en[0]].DeCommit()
	ug[en[0]] = u1G
    }

    // for all nodes, calculate the public key
    var pkx *big.Int
    var pky *big.Int
    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	pkx = (ug[en[0]])[0]
	pky = (ug[en[0]])[1]
	break
    }

    for k,id := range ids {
	if k == 0 {
	    continue
	}

	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	pkx, pky = secp256k1.S256().Add(pkx, pky, (ug[en[0]])[0],(ug[en[0]])[1])
    }
    log.Debug("=========KeyGenerate_ec2,","pkx",pkx,"pky",pky,"","============")
    //w.pkx <- string(pkx.Bytes())
    //w.pky <- string(pky.Bytes())
    w.pkx.PushBack(string(pkx.Bytes()))
    w.pky.PushBack(string(pky.Bytes()))

    // 5. calculate the share of private key
    var skU1 *big.Int
    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	skU1 = sstruct[en[0]].Share
	break
    }

    for k,id := range ids {
	if k == 0 {
	    continue
	}

	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	skU1 = new(big.Int).Add(skU1,sstruct[en[0]].Share)
    }
    skU1 = new(big.Int).Mod(skU1, secp256k1.S256().N)
    log.Debug("=========KeyGenerate_ec2,","skU1",skU1,"","============")

    //save skU1/u1PaillierSk/u1PaillierPk/...
    ss = string(skU1.Bytes())
    ss = ss + common.Sep11
    s1 = u1PaillierSk.Length
    s2 = string(u1PaillierSk.L.Bytes()) 
    s3 = string(u1PaillierSk.U.Bytes())
    ss = ss + s1 + common.Sep11 + s2 + common.Sep11 + s3 + common.Sep11

    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    s1 = u1PaillierPk.Length
	    s2 = string(u1PaillierPk.N.Bytes()) 
	    s3 = string(u1PaillierPk.G.Bytes()) 
	    s4 = string(u1PaillierPk.N2.Bytes()) 
	    ss = ss + s1 + common.Sep11 + s2 + common.Sep11 + s3 + common.Sep11 + s4 + common.Sep11
	    continue
	}
	for _,v := range cs {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		s1 = mm[3] 
		s2 = mm[4] 
		s3 = mm[5] 
		s4 = mm[6] 
		ss = ss + s1 + common.Sep11 + s2 + common.Sep11 + s3 + common.Sep11 + s4 + common.Sep11
		break
	    }
	}
    }

    sstmp := ss //////
    tmp := ss

    ss = ss + "NULL"

    // 6. calculate the zk
    // ## add content: zk of paillier key, zk of u
    
    // zk of paillier key
    u1zkFactProof := u1PaillierSk.ZkFactProve()
    // zk of u
    u1zkUProof := schnorrZK.ZkUProve(u1)

    // 7. Broadcast zk
    // u1zkFactProof, u2zkFactProof, u3zkFactProof, u4zkFactProof, u5zkFactProof
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "ZKFACTPROOF"
    s1 = string(u1zkFactProof.H1.Bytes())
    s2 = string(u1zkFactProof.H2.Bytes())
    s3 = string(u1zkFactProof.Y.Bytes())
    s4 = string(u1zkFactProof.E.Bytes())
    s5 = string(u1zkFactProof.N.Bytes())
    ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2 + common.Sep + s3 + common.Sep + s4 + common.Sep + s5
    log.Debug("================kg ec2 round three,send msg,code is ZKFACTPROOF==================")
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast zk
    // u1zkFactProof, u2zkFactProof, u3zkFactProof, u4zkFactProof, u5zkFactProof
    _,cherr = GetChannelValue(ch_t,w.bzkfact)
    if cherr != nil {
	log.Debug("get w.bzkfact timeout in keygenerate.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetZKFACTPROOFTimeout)}
	ch <- res
	return false 
    }

    sstmp2 := s1 + common.Sep11 + s2 + common.Sep11 + s3 + common.Sep11 + s4 + common.Sep11 + s5

    // 8. Broadcast zk
    // u1zkUProof, u2zkUProof, u3zkUProof, u4zkUProof, u5zkUProof
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "ZKUPROOF"
    s1 = string(u1zkUProof.E.Bytes())
    s2 = string(u1zkUProof.S.Bytes())
    ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2
    log.Debug("================kg ec2 round three,send msg,code is ZKUPROOF==================")
    SendMsgToDcrmGroup(ss)

    // 9. Receive Broadcast zk
    // u1zkUProof, u2zkUProof, u3zkUProof, u4zkUProof, u5zkUProof
    _,cherr = GetChannelValue(ch_t,w.bzku)
    if cherr != nil {
	log.Debug("get w.bzku timeout in keygenerate.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetZKUPROOFTimeout)}
	ch <- res
	return false 
    }
    
    // 1. verify the zk
    // ## add content: verify zk of paillier key, zk of u
	
    // for all nodes, verify zk of paillier key
    zkfacts := make([]string,NodeCnt-1)
    /*for i=0;i<(NodeCnt-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_zkfact)
	if cherr != nil {
	    log.Debug("get w.msg_zkfact timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_zkfact timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
    
	    return false
	}
	zkfacts[i] = v
    }*/
    if w.msg_zkfact.Len() != (NodeCnt-1) {
	log.Debug("get w.msg_zkfact fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllZKFACTPROOFFail)}
	ch <- res
	return false
    }
    itmp = 0
    iter = w.msg_zkfact.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	zkfacts[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for k,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) { /////bug for save zkfact
	    sstmp = sstmp + sstmp2 + common.Sep11
	    continue
	}

	u1PaillierPk2 := GetPaillierPk(tmp,k)
	for _,v := range zkfacts {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		h1 := new(big.Int).SetBytes([]byte(mm[2]))
		h2 := new(big.Int).SetBytes([]byte(mm[3]))
		y := new(big.Int).SetBytes([]byte(mm[4]))
		e := new(big.Int).SetBytes([]byte(mm[5]))
		n := new(big.Int).SetBytes([]byte(mm[6]))
		zkFactProof := &paillier.ZkFactProof{H1: h1, H2: h2, Y: y, E: e,N:n}
		log.Debug("===============KeyGenerate_ec2,","zkFactProof",zkFactProof,"","=============")
		///////
		sstmp = sstmp + mm[2] + common.Sep11 + mm[3] + common.Sep11 + mm[4] + common.Sep11 + mm[5] + common.Sep11 + mm[6] + common.Sep11  ///for save zkfact
		//////

		if !u1PaillierPk2.ZkFactVerify(zkFactProof) {
		    log.Debug("zk fact verify fail in keygenerate.")
		    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyZKFACTPROOFFail)}
		    ch <- res
	    
		    return false 
		}

		break
	    }
	}
    }

    // for all nodes, verify zk of u
    zku := make([]string,NodeCnt-1)
    /*for i=0;i<(NodeCnt-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_zku)
	if cherr != nil {
	    log.Debug("get w.msg_zku timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_zku timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return false
	}
	zku[i] = v
    }*/
    if w.msg_zku.Len() != (NodeCnt-1) {
	log.Debug("get w.msg_zku fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllZKUPROOFFail)}
	ch <- res
	return false
    }
    itmp = 0
    iter = w.msg_zku.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	zku[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range ids {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	for _,v := range zku {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		e := new(big.Int).SetBytes([]byte(mm[2]))
		s := new(big.Int).SetBytes([]byte(mm[3]))
		zkUProof := &schnorrZK.ZkUProof{E: e, S: s}
		if !schnorrZK.ZkUVerify(ug[en[0]],zkUProof) {
		    log.Debug("zku verify fail in keygenerate.")
		    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyZKUPROOFFail)}
		    ch <- res
		    return false 
		}

		break
	    }
	}
    } 
    
    sstmp = sstmp + "NULL"
    //w.save <- sstmp
    //w.save:  sku1:UiSK:U1PK:U2PK:U3PK:....:UnPK:U1H1:U1H2:U1Y:U1E:U1N:U2H1:U2H2:U2Y:U2E:U2N:U3H1:U3H2:U3Y:U3E:U3N:......:NULL
    w.save.PushBack(sstmp)

    //======== 打印 pkx, pky,  u1 到文件, 测试合谋 ========
    //详见./cmd/PrivRecov/main.go
    fh, e := log.FileHandler(datadir+"/xxx-"+cur_enode,log.JSONFormat())
    fl := log.New()
    if e == nil {
        fl.SetHandler(fh)
        fl.Debug("!!!", "pkx", pkx, "pky", pky, "u1", u1)
    }
    //===================================================================

    return true
}

//msgprex = hash
//return value is the backup for the dcrm sig
func Sign_ec2(msgprex string,save string,message string,tokenType string,pkx *big.Int,pky *big.Int,ch chan interface{},id int) string {
    log.Debug("===================Sign_ec2====================")
    gc := getgroupcount()
    if id < 0 || id >= len(workers) || gc <= 0 {
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetWorkerIdError)}
	ch <- res
	return ""
    }
    w := workers[id]
    
    hashBytes, err2 := hex.DecodeString(message)
    if err2 != nil {
	log.Debug("=========Sign_ec2==========","hashBytes",hashBytes,"err2",err2)
	res := RpcDcrmRes{Ret:"",Err:err2}
	ch <- res
	return ""
    }


    log.Debug("===========Sign_ec2============","save len",len(save),"save",save)
    log.Debug("===========Sign_ec2============","pkx",pkx,"pky",pky)
    //w.Clear() //bug
    //log.Debug("===================Sign_ec2 clear worker finish.====================")
    
    // [Notes]
    // 1. assume the nodes who take part in the signature generation as follows
    ids := GetIds()
    idSign := ids[:ThresHold]
	
    // 1. map the share of private key to no-threshold share of private key
    var self *big.Int
    lambda1 := big.NewInt(1)
    for _,uid := range idSign {
	enodes := GetEnodesByUid(uid)
	if IsCurNode(enodes,cur_enode) {
	    self = uid
	    break
	}
    }

    if self == nil {
	return ""
    }

    log.Debug("===============Sign_ec2==============","ids",ids,"ThresHold",ThresHold,"idSign",idSign,"gc",gc,"self",self)
    for i,uid := range idSign {
	enodes := GetEnodesByUid(uid)
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	
	log.Debug("===============Sign_ec2==============","i",i,"idSign[i]",idSign[i])
	sub := new(big.Int).Sub(idSign[i], self)
	subInverse := new(big.Int).ModInverse(sub,secp256k1.S256().N)
	times := new(big.Int).Mul(subInverse, idSign[i])
	lambda1 = new(big.Int).Mul(lambda1, times)
	lambda1 = new(big.Int).Mod(lambda1, secp256k1.S256().N)
    }
    mm := strings.Split(save, common.Sep11)
    skU1 := new(big.Int).SetBytes([]byte(mm[0]))
    w1 := new(big.Int).Mul(lambda1, skU1)
    w1 = new(big.Int).Mod(w1,secp256k1.S256().N)
    
    // 2. select k and gamma randomly
    u1K := random.GetRandomIntFromZn(secp256k1.S256().N)
    log.Debug("Sign_ec2","u1K",u1K)
    u1Gamma := random.GetRandomIntFromZn(secp256k1.S256().N)
    
    // 3. make gamma*G commitment to get (C, D)
    u1GammaGx,u1GammaGy := secp256k1.S256().ScalarBaseMult(u1Gamma.Bytes())
    commitU1GammaG := new(commit.Commitment).Commit(u1GammaGx, u1GammaGy)

    // 4. Broadcast
    //	commitU1GammaG.C, commitU2GammaG.C, commitU3GammaG.C
    mp := []string{msgprex,cur_enode}
    enode := strings.Join(mp,"-")
    s0 := "C11"
    s1 := string(commitU1GammaG.C.Bytes())
    ss := enode + common.Sep + s0 + common.Sep + s1
    log.Debug("================sign ec2 round one,send msg,code is C11==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    //	commitU1GammaG.C, commitU2GammaG.C, commitU3GammaG.C
     _,cherr := GetChannelValue(ch_t,w.bc11)
    if cherr != nil {
	log.Debug("get w.bc11 timeout.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetC11Timeout)}
	ch <- res
	return ""
    }
    
    // 2. MtA(k, gamma) and MtA(k, w)
    // 2.1 encrypt c_k = E_paillier(k)
    var ukc = make(map[string]*big.Int)
    var ukc2 = make(map[string]*big.Int)
    var ukc3 = make(map[string]*paillier.PublicKey)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    u1PaillierPk := GetPaillierPk(save,k)
	    u1KCipher,u1R,_ := u1PaillierPk.Encrypt(u1K)
	    ukc[en[0]] = u1KCipher
	    ukc2[en[0]] = u1R
	    ukc3[en[0]] = u1PaillierPk
	    break
	}
    }

    // 2.2 calculate zk(k)
    var zk1proof = make(map[string]*MtAZK.MtAZK1Proof)
    var zkfactproof = make(map[string]*paillier.ZkFactProof)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	u1zkFactProof := GetZkFactProof(save,k)
	zkfactproof[en[0]] = u1zkFactProof
	if IsCurNode(enodes,cur_enode) {
	    u1u1MtAZK1Proof := MtAZK.MtAZK1Prove(u1K,ukc2[en[0]], ukc3[en[0]], u1zkFactProof)
	    zk1proof[en[0]] = u1u1MtAZK1Proof
	} else {
	    u1u1MtAZK1Proof := MtAZK.MtAZK1Prove(u1K,ukc2[cur_enode], ukc3[cur_enode], u1zkFactProof)
	    //zk1proof[en[0]] = u1u1MtAZK1Proof
	    mp := []string{msgprex,cur_enode}
	    enode := strings.Join(mp,"-")
	    s0 := "MTAZK1PROOF"
	    s1 := string(u1u1MtAZK1Proof.Z.Bytes()) 
	    s2 := string(u1u1MtAZK1Proof.U.Bytes()) 
	    s3 := string(u1u1MtAZK1Proof.W.Bytes()) 
	    s4 := string(u1u1MtAZK1Proof.S.Bytes()) 
	    s5 := string(u1u1MtAZK1Proof.S1.Bytes()) 
	    s6 := string(u1u1MtAZK1Proof.S2.Bytes()) 
	    ss := enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2 + common.Sep + s3 + common.Sep + s4 + common.Sep + s5 + common.Sep + s6
	    log.Debug("================sign ec2 round two,send msg,code is MTAZK1PROOF==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
	    SendMsgToPeer(enodes,ss)
	}
    }

    _,cherr = GetChannelValue(ch_t,w.bmtazk1proof)
    if cherr != nil {
	log.Debug("get w.bmtazk1proof timeout in sign.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetMTAZK1PROOFTimeout)}
	ch <- res
	return ""
    }

    // 2.3 Broadcast c_k, zk(k)
    // u1KCipher, u2KCipher, u3KCipher
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "KC"
    s1 = string(ukc[cur_enode].Bytes())
    ss = enode + common.Sep + s0 + common.Sep + s1
    log.Debug("================sign ec2 round two,send msg,code is KC==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
    SendMsgToDcrmGroup(ss)

    // 2.4 Receive Broadcast c_k, zk(k)
    // u1KCipher, u2KCipher, u3KCipher
     _,cherr = GetChannelValue(ch_t,w.bkc)
    if cherr != nil {
	log.Debug("get w.bkc timeout.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetKCTimeout)}
	ch <- res
	return ""
    }

    var i int
    kcs := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_kc)
	if cherr != nil {
	    log.Debug("get w.msg_kc timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_kc timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	kcs[i] = v
    }*/
    if w.msg_kc.Len() != (ThresHold-1) {
	log.Debug("get w.msg_kc fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllKCFail)}
	ch <- res
	return ""
    }
    itmp := 0
    iter := w.msg_kc.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	kcs[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range kcs {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		kc := new(big.Int).SetBytes([]byte(mm[2]))
		ukc[en[0]] = kc
		break
	    }
	}
    }
   
    // example for u1, receive: u1u1MtAZK1Proof from u1, u2u1MtAZK1Proof from u2, u3u1MtAZK1Proof from u3
    mtazk1s := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_mtazk1proof)
	if cherr != nil {
	    log.Debug("get w.msg_mtazk1proof timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_mtazk1proof timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	mtazk1s[i] = v
    }*/
    if w.msg_mtazk1proof.Len() != (ThresHold-1) {
	log.Debug("get w.msg_mtazk1proof fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllMTAZK1PROOFFail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_mtazk1proof.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	mtazk1s[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range mtazk1s {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		z := new(big.Int).SetBytes([]byte(mm[2]))
		u := new(big.Int).SetBytes([]byte(mm[3]))
		w := new(big.Int).SetBytes([]byte(mm[4]))
		s := new(big.Int).SetBytes([]byte(mm[5]))
		s1 := new(big.Int).SetBytes([]byte(mm[6]))
		s2 := new(big.Int).SetBytes([]byte(mm[7]))
		mtAZK1Proof := &MtAZK.MtAZK1Proof{Z: z, U: u, W: w, S: s, S1: s1, S2: s2}
		zk1proof[en[0]] = mtAZK1Proof
		break
	    }
	}
    }

    // 2.5 verify zk(k)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    u1rlt1 := zk1proof[cur_enode].MtAZK1Verify(ukc[cur_enode],ukc3[cur_enode],zkfactproof[cur_enode])
	    if !u1rlt1 {
		log.Debug("self verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }
	} else {
	    if len(en) <= 0 {
		log.Debug("verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }

	    _,exsit := zk1proof[en[0]]
	    if exsit == false {
		log.Debug("verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }

	    _,exsit = ukc[en[0]]
	    if exsit == false {
		log.Debug("verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }
	    
	    u1PaillierPk := GetPaillierPk(save,k)
	    if u1PaillierPk == nil {
		log.Debug("verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }

	    _,exsit = zkfactproof[cur_enode]
	    if exsit == false {
		log.Debug("verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }

	    u1rlt1 := zk1proof[en[0]].MtAZK1Verify(ukc[en[0]],u1PaillierPk,zkfactproof[cur_enode])
	    if !u1rlt1 {
		log.Debug("verify MTAZK1PROOF fail.")
		res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMTAZK1PROOFFail)}
		ch <- res
		return ""
	    }
	}
    }

    // 2.6
    // select betaStar randomly, and calculate beta, MtA(k, gamma)
    // select betaStar randomly, and calculate beta, MtA(k, w)
    
    // [Notes]
    // 1. betaStar is in [1, paillier.N - secp256k1.N^2]
    NSalt := new(big.Int).Lsh(big.NewInt(1), uint(PaillierKeyLength-PaillierKeyLength/10))
    NSubN2 := new(big.Int).Mul(secp256k1.S256().N, secp256k1.S256().N)
    NSubN2 = new(big.Int).Sub(NSalt, NSubN2)
    // 2. MinusOne
    MinusOne := big.NewInt(-1)
    
    betaU1Star := make([]*big.Int,ThresHold)
    betaU1 := make([]*big.Int,ThresHold)
    for i=0;i<ThresHold;i++ {
	beta1U1Star := random.GetRandomIntFromZn(NSubN2)
	beta1U1 := new(big.Int).Mul(MinusOne, beta1U1Star)
	betaU1Star[i] = beta1U1Star
	betaU1[i] = beta1U1
    }

    vU1Star := make([]*big.Int,ThresHold)
    vU1 := make([]*big.Int,ThresHold)
    for i=0;i<ThresHold;i++ {
	v1U1Star := random.GetRandomIntFromZn(NSubN2)
	v1U1 := new(big.Int).Mul(MinusOne, v1U1Star)
	vU1Star[i] = v1U1Star
	vU1[i] = v1U1
    }

    // 2.7
    // send c_kGamma to proper node, MtA(k, gamma)   zk
    var mkg = make(map[string]*big.Int)
    var mkg_mtazk2 = make(map[string]*MtAZK.MtAZK2Proof)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    u1PaillierPk := GetPaillierPk(save,k)
	    u1KGamma1Cipher := u1PaillierPk.HomoMul(ukc[en[0]], u1Gamma)
	    beta1U1StarCipher, u1BetaR1,_ := u1PaillierPk.Encrypt(betaU1Star[k])
	    u1KGamma1Cipher = u1PaillierPk.HomoAdd(u1KGamma1Cipher, beta1U1StarCipher) // send to u1
	    u1u1MtAZK2Proof := MtAZK.MtAZK2Prove(u1Gamma, betaU1Star[k], u1BetaR1, ukc[cur_enode],ukc3[cur_enode], zkfactproof[cur_enode])
	    mkg[en[0]] = u1KGamma1Cipher
	    mkg_mtazk2[en[0]] = u1u1MtAZK2Proof
	    continue
	}
	
	u2PaillierPk := GetPaillierPk(save,k)
	u2KGamma1Cipher := u2PaillierPk.HomoMul(ukc[en[0]], u1Gamma)
	beta2U1StarCipher, u2BetaR1,_ := u2PaillierPk.Encrypt(betaU1Star[k])
	u2KGamma1Cipher = u2PaillierPk.HomoAdd(u2KGamma1Cipher, beta2U1StarCipher) // send to u2
	u2u1MtAZK2Proof := MtAZK.MtAZK2Prove(u1Gamma, betaU1Star[k], u2BetaR1, ukc[en[0]],u2PaillierPk,zkfactproof[cur_enode])
	mp = []string{msgprex,cur_enode}
	enode = strings.Join(mp,"-")
	s0 = "MKG"
	s1 = string(u2KGamma1Cipher.Bytes()) 
	//////
	s2 := string(u2u1MtAZK2Proof.Z.Bytes())
	s3 := string(u2u1MtAZK2Proof.ZBar.Bytes())
	s4 := string(u2u1MtAZK2Proof.T.Bytes())
	s5 := string(u2u1MtAZK2Proof.V.Bytes())
	s6 := string(u2u1MtAZK2Proof.W.Bytes())
	s7 := string(u2u1MtAZK2Proof.S.Bytes())
	s8 := string(u2u1MtAZK2Proof.S1.Bytes())
	s9 := string(u2u1MtAZK2Proof.S2.Bytes())
	s10 := string(u2u1MtAZK2Proof.T1.Bytes())
	s11 := string(u2u1MtAZK2Proof.T2.Bytes())
	///////
	ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2 + common.Sep + s3 + common.Sep + s4 + common.Sep + s5 + common.Sep + s6 + common.Sep + s7 + common.Sep + s8 + common.Sep + s9 + common.Sep + s10 + common.Sep + s11
	log.Debug("================sign ec2 round three,send msg,code is MKG==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
	SendMsgToPeer(enodes,ss)
    }
    
    // 2.8
    // send c_kw to proper node, MtA(k, w)   zk
    var mkw = make(map[string]*big.Int)
    var mkw_mtazk2 = make(map[string]*MtAZK.MtAZK2Proof)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    u1PaillierPk := GetPaillierPk(save,k)
	    u1Kw1Cipher := u1PaillierPk.HomoMul(ukc[en[0]], w1)
	    v1U1StarCipher, u1VR1,_ := u1PaillierPk.Encrypt(vU1Star[k])
	    u1Kw1Cipher = u1PaillierPk.HomoAdd(u1Kw1Cipher, v1U1StarCipher) // send to u1
	    u1u1MtAZK2Proof2 := MtAZK.MtAZK2Prove(w1, vU1Star[k], u1VR1, ukc[cur_enode], ukc3[cur_enode], zkfactproof[cur_enode])
	    mkw[en[0]] = u1Kw1Cipher
	    mkw_mtazk2[en[0]] = u1u1MtAZK2Proof2
	    continue
	}
	
	u2PaillierPk := GetPaillierPk(save,k)
	u2Kw1Cipher := u2PaillierPk.HomoMul(ukc[en[0]], w1)
	v2U1StarCipher, u2VR1,_ := u2PaillierPk.Encrypt(vU1Star[k])
	u2Kw1Cipher = u2PaillierPk.HomoAdd(u2Kw1Cipher,v2U1StarCipher) // send to u2
	u2u1MtAZK2Proof2 := MtAZK.MtAZK2Prove(w1, vU1Star[k], u2VR1, ukc[en[0]], u2PaillierPk, zkfactproof[cur_enode])

	mp = []string{msgprex,cur_enode}
	enode = strings.Join(mp,"-")
	s0 = "MKW"
	s1 = string(u2Kw1Cipher.Bytes()) 
	//////
	s2 := string(u2u1MtAZK2Proof2.Z.Bytes())
	s3 := string(u2u1MtAZK2Proof2.ZBar.Bytes())
	s4 := string(u2u1MtAZK2Proof2.T.Bytes())
	s5 := string(u2u1MtAZK2Proof2.V.Bytes())
	s6 := string(u2u1MtAZK2Proof2.W.Bytes())
	s7 := string(u2u1MtAZK2Proof2.S.Bytes())
	s8 := string(u2u1MtAZK2Proof2.S1.Bytes())
	s9 := string(u2u1MtAZK2Proof2.S2.Bytes())
	s10 := string(u2u1MtAZK2Proof2.T1.Bytes())
	s11 := string(u2u1MtAZK2Proof2.T2.Bytes())
	///////

	ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2 + common.Sep + s3 + common.Sep + s4 + common.Sep + s5 + common.Sep + s6 + common.Sep + s7 + common.Sep + s8 + common.Sep + s9 + common.Sep + s10 + common.Sep + s11
	log.Debug("================sign ec2 round four,send msg,code is MKW==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
	SendMsgToPeer(enodes,ss)
    }

    // 2.9
    // receive c_kGamma from proper node, MtA(k, gamma)   zk
     _,cherr = GetChannelValue(ch_t,w.bmkg)
    if cherr != nil {
	log.Debug("get w.bmkg timeout.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetMKGTimeout)}
	ch <- res
	return ""
    }

    mkgs := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_mkg)
	if cherr != nil {
	    log.Debug("get w.msg_mkg timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_mkg timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	mkgs[i] = v
    }*/
    if w.msg_mkg.Len() != (ThresHold-1) {
	log.Debug("get w.msg_mkg fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllMKGFail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_mkg.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	mkgs[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range mkgs {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		kg := new(big.Int).SetBytes([]byte(mm[2]))
		mkg[en[0]] = kg
		
		z := new(big.Int).SetBytes([]byte(mm[3]))
		zbar := new(big.Int).SetBytes([]byte(mm[4]))
		t := new(big.Int).SetBytes([]byte(mm[5]))
		v := new(big.Int).SetBytes([]byte(mm[6]))
		w := new(big.Int).SetBytes([]byte(mm[7]))
		s := new(big.Int).SetBytes([]byte(mm[8]))
		s1 := new(big.Int).SetBytes([]byte(mm[9]))
		s2 := new(big.Int).SetBytes([]byte(mm[10]))
		t1 := new(big.Int).SetBytes([]byte(mm[11]))
		t2 := new(big.Int).SetBytes([]byte(mm[12]))
		mtAZK2Proof := &MtAZK.MtAZK2Proof{Z: z, ZBar: zbar, T: t, V: v, W: w, S: s, S1: s1, S2: s2, T1: t1, T2: t2}
		mkg_mtazk2[en[0]] = mtAZK2Proof
		break
	    }
	}
    }

    // 2.10
    // receive c_kw from proper node, MtA(k, w)    zk
    _,cherr = GetChannelValue(ch_t,w.bmkw)
    if cherr != nil {
	log.Debug("get w.bmkw timeout.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetMKWTimeout)}
	ch <- res
	return ""
    }

    mkws := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_mkw)
	if cherr != nil {
	    log.Debug("get w.msg_mkw timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_mkw timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	mkws[i] = v
    }*/
    if w.msg_mkw.Len() != (ThresHold-1) {
	log.Debug("get w.msg_mkw fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllMKWFail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_mkw.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	mkws[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range mkws {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		kw := new(big.Int).SetBytes([]byte(mm[2]))
		mkw[en[0]] = kw

		z := new(big.Int).SetBytes([]byte(mm[3]))
		zbar := new(big.Int).SetBytes([]byte(mm[4]))
		t := new(big.Int).SetBytes([]byte(mm[5]))
		v := new(big.Int).SetBytes([]byte(mm[6]))
		w := new(big.Int).SetBytes([]byte(mm[7]))
		s := new(big.Int).SetBytes([]byte(mm[8]))
		s1 := new(big.Int).SetBytes([]byte(mm[9]))
		s2 := new(big.Int).SetBytes([]byte(mm[10]))
		t1 := new(big.Int).SetBytes([]byte(mm[11]))
		t2 := new(big.Int).SetBytes([]byte(mm[12]))
		mtAZK2Proof := &MtAZK.MtAZK2Proof{Z: z, ZBar: zbar, T: t, V: v, W: w, S: s, S1: s1, S2: s2, T1: t1, T2: t2}
		mkw_mtazk2[en[0]] = mtAZK2Proof
		break
	    }
	}
    }
    
    // 2.11 verify zk
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	rlt111 := mkg_mtazk2[en[0]].MtAZK2Verify(ukc[cur_enode], mkg[en[0]],ukc3[cur_enode], zkfactproof[en[0]])
	if !rlt111 {
	    log.Debug("mkg mtazk2 verify fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMKGFail)}
	    ch <- res
	    return ""
	}

	rlt112 := mkw_mtazk2[en[0]].MtAZK2Verify(ukc[cur_enode], mkw[en[0]], ukc3[cur_enode], zkfactproof[en[0]])
	if !rlt112 {
	    log.Debug("mkw mtazk2 verify fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifyMKWFail)}
	    ch <- res
	    return ""
	}
    }
    
    // 2.12
    // decrypt c_kGamma to get alpha, MtA(k, gamma)
    // MtA(k, gamma)
    var index int
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	if IsCurNode(enodes,cur_enode) {
	    index = k
	    break
	}
    }

    u1PaillierSk := GetPaillierSk(save,index)
    if u1PaillierSk == nil {
	log.Debug("get paillier sk fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetPaillierPrivKeyFail)}
	ch <- res
	return ""
    }
    
    alpha1 := make([]*big.Int,ThresHold)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	alpha1U1, _ := u1PaillierSk.Decrypt(mkg[en[0]])
	alpha1[k] = alpha1U1
    }

    // 2.13
    // decrypt c_kw to get u, MtA(k, w)
    // MtA(k, w)
    uu1 := make([]*big.Int,ThresHold)
    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	u1U1, _ := u1PaillierSk.Decrypt(mkw[en[0]])
	uu1[k] = u1U1
    }

    // 2.14
    // calculate delta, MtA(k, gamma)
    delta1 := alpha1[0]
    for i=0;i<ThresHold;i++ {
	if i == 0 {
	    continue
	}
	delta1 = new(big.Int).Add(delta1,alpha1[i])
    }
    for i=0;i<ThresHold;i++ {
	delta1 = new(big.Int).Add(delta1, betaU1[i])
    }

    // 2.15
    // calculate sigma, MtA(k, w)
    sigma1 := uu1[0]
    for i=0;i<ThresHold;i++ {
	if i == 0 {
	    continue
	}
	sigma1 = new(big.Int).Add(sigma1,uu1[i])
    }
    for i=0;i<ThresHold;i++ {
	sigma1 = new(big.Int).Add(sigma1, vU1[i])
    }

    // 3. Broadcast
    // delta: delta1, delta2, delta3
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "DELTA1"
    zero,_ := new(big.Int).SetString("0",10)
    if delta1.Cmp(zero) < 0 { //bug
	s1 = "0" + common.Sep12 + string(delta1.Bytes())
    } else {
	s1 = string(delta1.Bytes())
    }
    ss = enode + common.Sep + s0 + common.Sep + s1
    log.Debug("================sign ec2 round five,send msg,code is DELTA1==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    // delta: delta1, delta2, delta3
     _,cherr = GetChannelValue(ch_t,w.bdelta1)
    if cherr != nil {
	log.Debug("get w.bdelta1 timeout.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetDELTA1Timeout)}
	ch <- res
	return ""
    }
    
    var delta1s = make(map[string]*big.Int)
    delta1s[cur_enode] = delta1
    log.Debug("===========Sign_ec2,","delta1",delta1,"","===========")

    dels := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_delta1)
	if cherr != nil {
	    log.Debug("get w.msg_delta1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_delta1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	dels[i] = v
    }*/
    if w.msg_delta1.Len() != (ThresHold-1) {
	log.Debug("get w.msg_delta1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllDELTA1Fail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_delta1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	dels[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for k,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range dels {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		tmps := strings.Split(mm[2], common.Sep12)
		if len(tmps) == 2 {
		    del := new(big.Int).SetBytes([]byte(tmps[1]))
		    del = new(big.Int).Sub(zero,del) //bug:-xxxxxxx
		    log.Debug("===========Sign_ec2,","k",k,"del",del,"","===========")
		    delta1s[en[0]] = del
		} else {
		    del := new(big.Int).SetBytes([]byte(mm[2]))
		    log.Debug("===========Sign_ec2,","k",k,"del",del,"","===========")
		    delta1s[en[0]] = del
		}
		break
	    }
	}
    }
    
    // 2. calculate deltaSum
    var deltaSum *big.Int
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	deltaSum = delta1s[en[0]]
	break
    }
    for k,id := range idSign {
	if k == 0 {
	    continue
	}

	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	//bug
	if deltaSum == nil || len(en) < 1 || delta1s[en[0]] == nil {
	    log.Debug("===============sign ec2,calc deltaSum error.================","deltaSum",deltaSum,"len(en)",len(en),"en[0]",en[0],"delta1s[en[0]]",delta1s[en[0]])
	    var ret2 Err
	    ret2.Info = "calc deltaSum error"
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return ""
	}
	deltaSum = new(big.Int).Add(deltaSum,delta1s[en[0]])
    }
    deltaSum = new(big.Int).Mod(deltaSum, secp256k1.S256().N)

    // 3. Broadcast
    // commitU1GammaG.D, commitU2GammaG.D, commitU3GammaG.D
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "D11"
    dlen := len(commitU1GammaG.D)
    s1 = strconv.Itoa(dlen)

    ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep
    for _,d := range commitU1GammaG.D {
	ss += string(d.Bytes())
	ss += common.Sep
    }
    ss = ss + "NULL"
    log.Debug("================sign ec2 round six,send msg,code is D11==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    // commitU1GammaG.D, commitU2GammaG.D, commitU3GammaG.D
    _,cherr = GetChannelValue(ch_t,w.bd11_1)
    if cherr != nil {
	log.Debug("get w.bd11_1 timeout in sign.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetD11Timeout)}
	ch <- res
	return ""
    }

    d11s := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_d11_1)
	if cherr != nil {
	    log.Debug("get w.msg_d11_1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_d11_1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	d11s[i] = v
    }*/
    if w.msg_d11_1.Len() != (ThresHold-1) {
	log.Debug("get w.msg_d11_1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllD11Fail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_d11_1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	d11s[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    c11s := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_c11)
	if cherr != nil {
	    log.Debug("get w.msg_c11 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_c11 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	c11s[i] = v
    }*/
    if w.msg_c11.Len() != (ThresHold-1) {
	log.Debug("get w.msg_c11 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllC11Fail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_c11.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	c11s[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    // 2. verify and de-commitment to get GammaG
    
    // for all nodes, construct the commitment by the receiving C and D
    var udecom = make(map[string]*commit.Commitment)
    for _,v := range c11s {
	mm := strings.Split(v, common.Sep)
	prex := mm[0]
	prexs := strings.Split(prex,"-")
	for _,vv := range d11s {
	    mmm := strings.Split(vv, common.Sep)
	    prex2 := mmm[0]
	    prexs2 := strings.Split(prex2,"-")
	    if prexs[len(prexs)-1] == prexs2[len(prexs2)-1] {
		dlen,_ := strconv.Atoi(mmm[2])
		var gg = make([]*big.Int,0)
		l := 0
		for j:=0;j<dlen;j++ {
		    l++
		    gg = append(gg,new(big.Int).SetBytes([]byte(mmm[2+l])))
		}
		deCommit := &commit.Commitment{C:new(big.Int).SetBytes([]byte(mm[2])), D:gg}
		log.Debug("=========Sign_ec2,","deCommit",deCommit,"","==========")
		udecom[prexs[len(prexs)-1]] = deCommit
		break
	    }
	}
    }
    deCommit_commitU1GammaG := &commit.Commitment{C: commitU1GammaG.C, D: commitU1GammaG.D}
    udecom[cur_enode] = deCommit_commitU1GammaG
    log.Debug("=========Sign_ec2,","deCommit_commitU1GammaG",deCommit_commitU1GammaG,"","==========")

    log.Debug("===========Sign_ec2,[Signature Generation][Round 4] 2. all nodes verify commit(GammaG):=============")

    // for all nodes, verify the commitment
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	//bug
	if len(en) <= 0 {
	    log.Debug("u1 verify commit in sign fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSignVerifyCommitFail)}
	    ch <- res
	    return ""
	}
	_,exsit := udecom[en[0]]
	if exsit == false {
	    log.Debug("u1 verify commit in sign fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSignVerifyCommitFail)}
	    ch <- res
	    return ""
	}
	//

	if udecom[en[0]].Verify() == false {
	    log.Debug("u1 verify commit in sign fail.")
	    res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSignVerifyCommitFail)}
	    ch <- res
	    return ""
	}
    }

    // for all nodes, de-commitment
    var ug = make(map[string][]*big.Int)
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	_, u1GammaG := udecom[en[0]].DeCommit()
	ug[en[0]] = u1GammaG
    }

    // for all nodes, calculate the GammaGSum
    var GammaGSumx *big.Int
    var GammaGSumy *big.Int
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	GammaGSumx = (ug[en[0]])[0]
	GammaGSumy = (ug[en[0]])[1]
	break
    }

    for k,id := range idSign {
	if k == 0 {
	    continue
	}

	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	GammaGSumx, GammaGSumy = secp256k1.S256().Add(GammaGSumx, GammaGSumy, (ug[en[0]])[0],(ug[en[0]])[1])
    }
    log.Debug("========Sign_ec2,","GammaGSumx",GammaGSumx,"GammaGSumy",GammaGSumy,"","===========")
	
    // 3. calculate deltaSum^-1 * GammaGSum
    deltaSumInverse := new(big.Int).ModInverse(deltaSum, secp256k1.S256().N)
    deltaGammaGx, deltaGammaGy := secp256k1.S256().ScalarMult(GammaGSumx, GammaGSumy, deltaSumInverse.Bytes())

    // 4. get r = deltaGammaGx
    r := deltaGammaGx

    if r.Cmp(zero) == 0 {
	log.Debug("sign error: r equal zero.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrREqualZero)}
	ch <- res
	return ""
    }
    
    // 5. calculate s
    mMtA,_ := new(big.Int).SetString(message,16)
    
    mk1 := new(big.Int).Mul(mMtA, u1K)
    rSigma1 := new(big.Int).Mul(deltaGammaGx, sigma1)
    us1 := new(big.Int).Add(mk1, rSigma1)
    us1 = new(big.Int).Mod(us1, secp256k1.S256().N)
    log.Debug("=========Sign_ec2,","us1",us1,"","==========")
    
    // 6. calculate S = s * R
    S1x, S1y := secp256k1.S256().ScalarMult(deltaGammaGx, deltaGammaGy, us1.Bytes())
    log.Debug("=========Sign_ec2,","S1x",S1x,"","==========")
    log.Debug("=========Sign_ec2,","S1y",S1y,"","==========")
    
    // 7. Broadcast
    // S: S1, S2, S3
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "S1"
    s1 = string(S1x.Bytes())
    s2 := string(S1y.Bytes())
    ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2
    log.Debug("================sign ec2 round seven,send msg,code is S1==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    // S: S1, S2, S3
    _,cherr = GetChannelValue(ch_t,w.bs1)
    if cherr != nil {
	log.Debug("get w.bs1 timeout in sign.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetS1Timeout)}
	ch <- res
	return ""
    }

    var s1s = make(map[string][]*big.Int)
    s1ss := []*big.Int{S1x,S1y}
    s1s[cur_enode] = s1ss

    us1s := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_s1)
	if cherr != nil {
	    log.Debug("get w.msg_s1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_s1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	us1s[i] = v
    }*/
    if w.msg_s1.Len() != (ThresHold-1) {
	log.Debug("get w.msg_s1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllS1Fail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_s1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	us1s[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range us1s {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		x := new(big.Int).SetBytes([]byte(mm[2]))
		y := new(big.Int).SetBytes([]byte(mm[3]))
		tmp := []*big.Int{x,y}
		s1s[en[0]] = tmp
		break
	    }
	}
    }

    // 2. calculate SAll
    var SAllx *big.Int
    var SAlly *big.Int
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	SAllx = (s1s[en[0]])[0]
	SAlly = (s1s[en[0]])[1]
	break
    }

    for k,id := range idSign {
	if k == 0 {
	    continue
	}

	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	SAllx, SAlly = secp256k1.S256().Add(SAllx, SAlly, (s1s[en[0]])[0],(s1s[en[0]])[1])
    }
    log.Debug("[Signature Generation][Test] verify SAll ?= m*G + r*PK:")
    log.Debug("========Sign_ec2,","SAllx",SAllx,"SAlly",SAlly,"","===========")
	
    // 3. verify SAll ?= m*G + r*PK
    mMtAGx, mMtAGy := secp256k1.S256().ScalarBaseMult(mMtA.Bytes())
    rMtAPKx, rMtAPKy := secp256k1.S256().ScalarMult(pkx, pky, deltaGammaGx.Bytes())
    SAllComputex, SAllComputey := secp256k1.S256().Add(mMtAGx, mMtAGy, rMtAPKx, rMtAPKy)
    log.Debug("========Sign_ec2,","SAllComputex",SAllComputex,"SAllComputey",SAllComputey,"","===========")

    if SAllx.Cmp(SAllComputex) != 0 || SAlly.Cmp(SAllComputey) != 0 {
	log.Debug("verify SAll != m*G + r*PK in sign ec2.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrVerifySAllFail)}
	ch <- res
	return ""
    }

    // 4. Broadcast
    // s: s1, s2, s3
    mp = []string{msgprex,cur_enode}
    enode = strings.Join(mp,"-")
    s0 = "SS1"
    s1 = string(us1.Bytes())
    ss = enode + common.Sep + s0 + common.Sep + s1
    log.Debug("================sign ec2 round eight,send msg,code is SS1==================","ss len",len(ss),"ss",new(big.Int).SetBytes([]byte(ss)))
    SendMsgToDcrmGroup(ss)

    // 1. Receive Broadcast
    // s: s1, s2, s3
    _,cherr = GetChannelValue(ch_t,w.bss1)
    if cherr != nil {
	log.Debug("get w.bss1 timeout in sign.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetSS1Timeout)}
	ch <- res
	return ""
    }

    var ss1s = make(map[string]*big.Int)
    ss1s[cur_enode] = us1

    uss1s := make([]string,ThresHold-1)
    /*for i=0;i<(ThresHold-1);i++ {
	v,cherr := GetChannelValue(ch_t,w.msg_ss1)
	if cherr != nil {
	    log.Debug("get w.msg_ss1 timeout.")
	    var ret2 Err
	    ret2.Info = "get w.msg_ss1 timeout."
	    res := RpcDcrmRes{Ret:"",Err:ret2}
	    ch <- res
	    return
	}
	uss1s[i] = v
    }*/
    if w.msg_ss1.Len() != (ThresHold-1) {
	log.Debug("get w.msg_ss1 fail.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrGetAllSS1Fail)}
	ch <- res
	return ""
    }
    itmp = 0
    iter = w.msg_ss1.Front()
    for iter != nil {
	mdss := iter.Value.(string)
	uss1s[itmp] = mdss 
	iter = iter.Next()
	itmp++
    }

    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	if IsCurNode(enodes,cur_enode) {
	    continue
	}
	for _,v := range uss1s {
	    mm := strings.Split(v, common.Sep)
	    prex := mm[0]
	    prexs := strings.Split(prex,"-")
	    if prexs[len(prexs)-1] == en[0] {
		tmp := new(big.Int).SetBytes([]byte(mm[2]))
		ss1s[en[0]] = tmp
		break
	    }
	}
    }

    // 2. calculate s
    var sSum *big.Int
    for _,id := range idSign {
	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	sSum = ss1s[en[0]]
	break
    }

    for k,id := range idSign {
	if k == 0 {
	    continue
	}

	enodes := GetEnodesByUid(id)
	en := strings.Split(string(enodes[8:]),"@")
	sSum = new(big.Int).Add(sSum,ss1s[en[0]])
    }
    sSum = new(big.Int).Mod(sSum, secp256k1.S256().N) 
   
    // 3. justify the s
    bb := false
    halfN := new(big.Int).Div(secp256k1.S256().N, big.NewInt(2))
    if sSum.Cmp(halfN) > 0 {
	bb = true
	sSum = new(big.Int).Sub(secp256k1.S256().N, sSum)
    }

    s := sSum
    if s.Cmp(zero) == 0 {
	log.Debug("sign error: s equal zero.")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrSEqualZero)}
	ch <- res
	return ""
    }

    log.Debug("==========Sign_ec2,","r",r,"===============")
    log.Debug("==========Sign_ec2,","s",s,"===============")
    
    // **[Test]  verify signature with MtA
    // ** verify the signature
    /*sSumInverse := new(big.Int).ModInverse(sSum, secp256k1.S256().N)
    mMtASInverse := new(big.Int).Mul(mMtA, sSumInverse)
    mMtASInverse = new(big.Int).Mod(mMtASInverse, secp256k1.S256().N)

    mMtASInverseGx, mMtASInverseGy := secp256k1.S256().ScalarBaseMult(mMtASInverse.Bytes())
    rSSumInverse := new(big.Int).Mul(deltaGammaGx, sSumInverse)
    rSSumInverse = new(big.Int).Mod(rSSumInverse, secp256k1.S256().N)

    rSSumInversePkx, rSSumInversePky := secp256k1.S256().ScalarMult(pkx, pky, rSSumInverse.Bytes())
    //computeRxMtA, computeRyMtA := secp256k1.S256().Add(mMtASInverseGx, mMtASInverseGy, rSSumInversePkx, rSSumInversePky) // m * sInverse * base point + r * sInverse * PK
    computeRxMtA,_ := secp256k1.S256().Add(mMtASInverseGx, mMtASInverseGy, rSSumInversePkx, rSSumInversePky) // m * sInverse * base point + r * sInverse * PK
    log.Debug("==========Sign_ec2,","computeRxMtA",computeRxMtA,"===============")
    if r.Cmp(computeRxMtA) != 0 {
	log.Debug("verify r != R.x in dcrm sign ec2.")
	var ret2 Err
	ret2.Info = "verify r != R.x in dcrm sign ec2."
	res := RpcDcrmRes{Ret:"",Err:ret2}
	ch <- res
	return
    }*/
    // **[End-Test]  verify signature with MtA

    signature := new(ECDSASignature)
    signature.New()
    signature.SetR(r)
    signature.SetS(s)

    //v
    recid := secp256k1.Get_ecdsa_sign_v(deltaGammaGx, deltaGammaGy)
    if tokenType == "ETH" && bb {
	recid ^=1
    }
    if tokenType == "BTC" && bb {
	recid ^= 1
    }

    ////check v
    ys := secp256k1.S256().Marshal(pkx,pky)
    pubkeyhex := hex.EncodeToString(ys)
    pbhs := []rune(pubkeyhex)
    if string(pbhs[0:2]) == "0x" {
	pubkeyhex = string(pbhs[2:])
    }
    log.Debug("Sign_ec2","pubkeyhex",pubkeyhex)
    log.Debug("=========Sign_ec2==========","hashBytes",hashBytes)
    
    rsvBytes1 := append(signature.GetR().Bytes(), signature.GetS().Bytes()...)
    for j := 0; j < (btcec.S256().H+1)*2; j++ {
	rsvBytes2 := append(rsvBytes1, byte(j))
	pkr, e := secp256k1.RecoverPubkey(hashBytes,rsvBytes2)
	log.Debug("Sign_ec2","pkr",string(pkr),"err",e)
	pkr2 := hex.EncodeToString(pkr)
	log.Debug("Sign_ec2","pkr2",pkr2)
	pbhs2 := []rune(pkr2)
	if string(pbhs2[0:2]) == "0x" {
		    pkr2 = string(pbhs2[2:])
	}
	if e == nil && strings.EqualFold(pkr2,pubkeyhex) {
		recid = j
		break
	}
    }
    ///// 
    signature.SetRecoveryParam(int32(recid))

    //===================================================
    if Verify(signature.GetR(),signature.GetS(),signature.GetRecoveryParam(),message,pkx,pky) == false {
	log.Debug("===================dcrm sign,verify is false=================")
	res := RpcDcrmRes{Ret:"",Err:GetRetErr(ErrDcrmSignVerifyFail)}
	ch <- res
	return ""
    }

    ///////////add for bak sig
    var signature3 string
    /*sig2 := new(ECDSASignature)
    sig2.New()
    sig2.SetR(r)
    sig2.SetS(s)
    recid ^= 1
    sig2.SetRecoveryParam(int32(recid))
    if Verify(sig2.GetR(),sig2.GetS(),sig2.GetRecoveryParam(),message,pkx,pky) {
	log.Debug("bak sig verify pass.")
	signature3 = GetSignString(sig2.GetR(),sig2.GetS(),sig2.GetRecoveryParam(),int(sig2.GetRecoveryParam()))
    log.Debug("======================","bak sig str",signature3,"","=============================")
    }*/
    ///////////

    signature2 := GetSignString(signature.GetR(),signature.GetS(),signature.GetRecoveryParam(),int(signature.GetRecoveryParam()))
    log.Debug("======================","r",signature.GetR(),"","=============================")
    log.Debug("======================","s",signature.GetS(),"","=============================")
    log.Debug("======================","signature str",signature2,"","=============================")
    res := RpcDcrmRes{Ret:signature2,Err:nil}
    ch <- res
    return signature3
}

func GetPaillierPk(save string,index int) *paillier.PublicKey {
    if save == "" || index < 0 {
	return nil
    }

    mm := strings.Split(save, common.Sep11)
    s := 4 + 4*index
    l := mm[s]
    n := new(big.Int).SetBytes([]byte(mm[s+1]))
    g := new(big.Int).SetBytes([]byte(mm[s+2]))
    n2 := new(big.Int).SetBytes([]byte(mm[s+3]))
    publicKey := &paillier.PublicKey{Length: l, N: n, G: g, N2: n2}
    return publicKey
}

func GetPaillierSk(save string,index int) *paillier.PrivateKey {
    publicKey := GetPaillierPk(save,index)
    if publicKey != nil {
	mm := strings.Split(save, common.Sep11)
	l := mm[1]
	ll := new(big.Int).SetBytes([]byte(mm[2]))
	uu := new(big.Int).SetBytes([]byte(mm[3]))
	privateKey := &paillier.PrivateKey{Length: l, PublicKey: *publicKey, L: ll, U: uu}
	return privateKey
    }

    return nil
}

func GetZkFactProof(save string,index int) *paillier.ZkFactProof {
    if save == "" || index < 0 {
	return nil
    }

    mm := strings.Split(save, common.Sep11)
    s := 4 + 4*NodeCnt + 5*index////????? TODO
    h1 := new(big.Int).SetBytes([]byte(mm[s]))
    h2 := new(big.Int).SetBytes([]byte(mm[s+1]))
    y := new(big.Int).SetBytes([]byte(mm[s+2]))
    e := new(big.Int).SetBytes([]byte(mm[s+3]))
    n := new(big.Int).SetBytes([]byte(mm[s+4]))
    zkFactProof := &paillier.ZkFactProof{H1: h1, H2: h2, Y: y, E: e,N: n}
    return zkFactProof
}

