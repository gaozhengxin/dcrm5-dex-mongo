package main

import (
	"github.com/fusion/go-fusion/cmd/eos_tracker/dao"
	"github.com/fusion/go-fusion/cmd/eos_tracker/transfer_tracker"
	"github.com/fusion/go-fusion/cmd/eos_tracker/config"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
)

func musthas (name string) {
	if flg := flag.Lookup(name); flg == nil || flg.Value.String() == flg.DefValue {
		log.Fatal("must has param " + name)
	}
}

func main () {

	port := flag.String("port","7000","port")
	dbpath := flag.String("dbpath", "./eosdata", "database path")
	reinit := flag.Bool("reinit", false, "reinit")
	eb := flag.String("eosbase", "", "Eos base account")
	flag.Parse()
	path := "0.0.0.0:" + *port

	musthas("eosbase")
	config.SetEosBase(*eb)

	dbPath, err := filepath.Abs(*dbpath)
	if err != nil {
		fmt.Println("get absolute path error.")
		panic(err)
	}
	config.SetDbPath(dbPath)

	defer dao.Close()
	go tracker.Run(reinit)
	http.HandleFunc("/get_balance", GetBalance)
	http.HandleFunc("/get_tx", GetTransaction)
	go http.ListenAndServe(path, nil)
	fmt.Printf("service is running on %s\n", path)
	select{}
}

type Resp struct {
	Code string `json:"code"`
	Msg string `json:"Msg,omitempty"`
	Balance string `json:"balance,omitempty"`
}

type TxResp struct {
	Code string `json:"code"`
	Msg string `json:"Msg,omitempty"`
	Tx string `json:"Tx,omitempty"`
}

func GetBalance (writer http.ResponseWriter, request *http.Request) {
	request.ParseForm()
	userKey, ok := request.Form["user_key"]
	var result Resp
	if !ok {
		result.Code = "401"
		result.Msg = "no user_key"
	} else {
		result.Code = "200"
		result.Balance = dao.GetBalance(userKey[0])
	}
	if err := json.NewEncoder(writer).Encode(result); err != nil {
		log.Fatal(err)
	}
}

func GetTransaction (writer http.ResponseWriter, request *http.Request) {
	request.ParseForm()
	txhash, ok := request.Form["txhash"]
	var result TxResp
	if !ok {
		result.Code = "401"
		result.Msg = "no txhash"
	} else {
		result.Code = "200"
		result.Tx = dao.GetTx(txhash[0]).String()
	}
	if err := json.NewEncoder(writer).Encode(result); err != nil {
		log.Fatal(err)
	}

}
