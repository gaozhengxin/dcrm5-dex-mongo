package main

import (
//	"github.com/fusion/go-fusion/xprotocol/orderbook"
	"github.com/fusion/go-fusion/log"
//	"github.com/shopspring/decimal"
//	"github.com/fusion/go-fusion/common"
	"os"
	"fmt"
	"time"
	//"strconv"
//	"math/big"
)

func init() {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	log.Root().SetHandler(glogger)
}

func main() {
  a := time.Now().UTC()
  b := time.Now().Unix()
  c := time.Now().UnixNano()
  fmt.Println("a",a)
  fmt.Println("b",b)
  fmt.Println("c",c)
  return
  /*
  ob := orderbook.NewXvcOrderBook()
  fmt.Println("orderbook=",ob)

  p,_ := decimal.NewFromString("10.01")
  od1 := orderbook.NewXvcOrder("1",common.HexToAddress("1111"),"ETH/BTC","LimitOrder","Buy",time.Now().UTC(),decimal.New(100, 0), p,"GTE",nil)
  ob.InsertToOrderBook(od1)
  
  p,_ = decimal.NewFromString("10.00")
  od2 := orderbook.NewXvcOrder("2",common.HexToAddress("2222"),"ETH/BTC","LimitOrder","Buy",time.Now().UTC(),decimal.New(200, 0), p,"GTE",nil)
  ob.InsertToOrderBook(od2)

  p,_ = decimal.NewFromString("9.98")
  od3 := orderbook.NewXvcOrder("3",common.HexToAddress("3333"),"ETH/BTC","LimitOrder","Sell",time.Now().UTC(),decimal.New(100, 0), p,"GTE",nil)
  ob.InsertToOrderBook(od3)
  
  p,_ = decimal.NewFromString("9.97")
  od4 := orderbook.NewXvcOrder("4",common.HexToAddress("4444"),"ETH/BTC","LimitOrder","Sell",time.Now().UTC(),decimal.New(100, 0),p,"GTE",nil)
  ob.InsertToOrderBook(od4)
  
  fmt.Println("ob",ob)

  /////////
  a,_ := new(big.Int).SetString("4443524d434f4e4649524d414444523a636f696e2074797065206973206e6f7420737570706f727465642e3a54524f4e",16)
  fmt.Println("a=",a)
  s := string(a.Bytes())
  fmt.Println("s=",s)
*/
  /////////

}

