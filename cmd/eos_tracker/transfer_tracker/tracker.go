package tracker

import (
	"github.com/fusion/go-fusion/cmd/eos_tracker/dao"
	"github.com/fusion/go-fusion/cmd/eos_tracker/config"
	"github.com/fusion/go-fusion/cmd/eos_tracker/utils"
	"encoding/json"
	"fmt"
	//"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins/rpcutils"
	"github.com/fusion/go-fusion/log"
)

var dbPath string
var err error

var CalculateFee func(ram, cpu, net int64) uint32

func calculateFee (ram, cpu, net int64) uint32 {
	if CalculateFee == nil {
		return defaultCalculateFee(ram, cpu, net)
	} else {
		return CalculateFee(ram, cpu, net)
	}
}

func defaultCalculateFee(ram, cpu, net int64) uint32 {
	// 第一次lockout到账户A消耗ram, 第二次以后lockout到账户A不消耗ram, ram用完要购买
	// cpu net是可再生的, 用eos抵押的, 可以赎回, 所以不扣手续费
	var ramprice float64 = 0.0014 //TODO
	fee := uint32(ramprice * float64(ram))
	if fee == 0 {
		return 1
	} else {
		return fee
	}
}

func Run (reinit *bool) {

	exists, err := PathExists(config.DbPath)
	if err != nil {
		panic(err)
	}
	if !exists || *reinit {
		initDb()
	}
	//log.Printf("database path is %v\n", config.DbPath)

	defer func() {
		if r := recover(); r != nil {
			//log.Println(r)
		}
	} ()

	for {
		cursor := dao.Get("cursor")
		pos, e := strconv.Atoi(cursor)
		if e != nil {
			err = fmt.Errorf("cursor not found\ntry add reinit\n")
			//log.Fatal(err)
		}
		//log.Printf("pos: %v\n",pos)
		scan(pos)
		time.Sleep(5 * time.Second)
	}
}

func scan(pos int) {
	api := "v1/history/get_actions"
	offset := 99
	var almostutd = false
// 从pos位置开始查action记录, 每次查offset-1条, 查完更新pos位置, 返回结果的长度小于offset-1表示已经达到顶部
// 返回成功执行的tra actions
	for !almostutd {
		data := `{"pos":` + strconv.Itoa(pos) + `,"offset":` + strconv.Itoa(offset) + `,"account_name":"` + config.EOS_ACCOUNT + `"}`
	//log.Println(data)
		ret := rpcutils.DoCurlRequest(config.NODEOS, api, data)
	//log.Println(ret)
		trs, totallen, err := parseResult(ret)
		//log.Printf("total len is %v\n", totallen)
		if totallen > 0 {
			l := totallen
			pos += l
			dao.Put("cursor", strconv.Itoa(pos))
			handleActions(trs)
			if l < offset + 1 {
				almostutd = true
			}
		}
		if err != nil {
			//log.Print(err)
			time.Sleep(1 * time.Second)
			continue
		}
		time.Sleep(1 * time.Second)
	}
}

var TryTimes = 100

func handleActions(trs []trace) {
	//log.Printf("handling actions %v\n", len(trs))
	//log.Printf("handling actions %+v\n", trs)
	addTxs(trs)
	var wg sync.WaitGroup
	for i := 0; i < len(trs); i++ {
		wg.Add(1)
		 func() {
			//log.Printf("checking transaction %v\n", trs[i].txid)
			defer wg.Add(-1)
			checkTransaction(trs[i])
		}()
	}
	wg.Wait()
}

func decodeRam(tf trace) int64 {
	defer func () {
		if e := recover(); e != nil {
			//log.Print(e)
		}
	} ()
	//只计算eosbase帐号消耗的ram
	if len(tf.accountramdeltas) > 0 {
		for _, x := range tf.accountramdeltas {
			if x.(map[string]interface{})["account"] == config.EOS_ACCOUNT {
				ram := tf.accountramdeltas[0].(map[string]interface{})["delta"].(float64)
				return int64(ram)
			}
		}
	}
	return 0
}

// 检查交易是否成功
func checkTransaction(tf trace) {
	api := "v1/history/get_transaction"
	data := `{"id":` + `"` + tf.txid + `"` + `,"block_num_hint":` + `"` + strconv.FormatFloat(tf.blockNum, 'f', -1, 64) + `"}`
	//log.Println(data)

	var count uint32 = 0

	var m = make(map[int]func(index int, ch chan bool))

	ch := make(chan bool)

	for i := 0; i < TryTimes; i++ {
		m[i] = func (index int, ch chan bool) {
			defer func() {
				atomic.AddUint32(&count,1)
				if r := recover(); r != nil {
					//log.Println(r)
				}
			} ()
			for {
				if !atomic.CompareAndSwapUint32(&count, uint32(index), uint32(index)) {
					time.Sleep(500 * time.Millisecond)
				} else {
					break
				}
			}
			//log.Printf("check transaction %v, try time: %v\n", tf.txid, index)
			/*ok, isOpen := <-ch
			log.Printf("============ %v %v ============\n", ok, isOpen)
			if ok || !isOpen {
				log.Println("channel is closed")
				return
			}*/
			ret := rpcutils.DoCurlRequest(config.NODEOS, api, data)
			//log.Println(ret)
			log.Debug("============================checkTransaction=========================================","get req result from full node",ret,"tx hash",tf.txid)
			var trxstr interface{}
			err := json.Unmarshal([]byte(ret),&trxstr)
			if err != nil {
				//log.Printf("check transaction %v fail, try times: %v, error: %v\n", tf.txid, i, err)
			}

			traces := trxstr.(map[string]interface{})["traces"].([]interface{})
			//log.Printf("traces: %+v\n", traces)
			var flag bool = false
			for _, trace := range traces {
				from := trace.(map[string]interface{})["act"].(map[string]interface{})["data"].(map[string]interface{})["from"].(string)
				to := trace.(map[string]interface{})["act"].(map[string]interface{})["data"].(map[string]interface{})["to"].(string)
				memo := trace.(map[string]interface{})["act"].(map[string]interface{})["data"].(map[string]interface{})["memo"].(string)
				quantity := trace.(map[string]interface{})["act"].(map[string]interface{})["data"].(map[string]interface{})["quantity"].(string)
				quantity = utils.ParseQuantity(quantity)

				//log.Printf("from: %v\nto: %v\nmemo: %v\nquantity: %v\n", from, to, memo, quantity)
				if tf.realfrom == from && tf.realto == to && tf.memo == memo && strings.TrimPrefix(tf.quantity,"-") == quantity {
				    log.Debug("=======================checkTransaction======================","current trace",trace,"tx hash",tf.txid)
					flag = true
					break
				}
			}
			if flag == false {
				//log.Printf("check transaction error, transfer is not contained in transaction %+v\n", tf.txid)
				return
			}

			status := trxstr.(map[string]interface{})["trx"].(map[string]interface{})["receipt"].(map[string]interface{})["status"].(string)
			last_irreversible_block := trxstr.(map[string]interface{})["last_irreversible_block"].(float64)
			block_num := trxstr.(map[string]interface{})["block_num"].(float64)

			confirmed := (block_num <= last_irreversible_block) && status == "executed"
			//log.Printf("!!!!!!!!!! confirmed: %v\n", confirmed)
			log.Debug("=====================checkTransaction===========","blocknum",block_num,"last_irreversible_block",last_irreversible_block,"status",status,"confirmed",confirmed,"tx hash",tf.txid)
			if confirmed {
				addTx(tf, true)
				ch <- true
				close(ch)
				return
			} else {
				time.Sleep(5 * time.Second)
				return
			}
		}
	}
	for i, fn := range m {
		go fn(i, ch)
	}
}

// get_actions接口获得的结果
type trace struct {
	seq float64;
	receiver string
	blockNum float64;
	txid string;
	memo string;
	quantity string;
	realfrom string
	realto string;
	accountramdeltas []interface{};
}

func parseResult(ret string) (tfs []trace, totallen int, err error) {
	var result map[string]interface{}
	err = json.Unmarshal([]byte(ret),&result)
	if err != nil {
		err = fmt.Errorf("failed parsing rpc response")
		return
	}
	defer func() {
		if r := recover(); r != nil {
			//log.Println(r)
		}
	} ()
	actions := result["actions"].([]interface{})
	totallen = len(actions)
	for _, action := range actions {
		receipt := action.(map[string]interface{})["action_trace"].(map[string]interface{})["receipt"]
		receiver := receipt.(map[string]interface{})["receiver"].(string)
		// lockin transfer有2条trace, receiver分别是from帐号和to帐号(eosbase)
		// 每个lockout transfer有3条trace, receiver分别是系统帐号eosio.token, from帐号(eosbase), to帐号
		// receiver为eosio.token的trace中包含所有需要的信息, 包括ram消耗
		// 从receiver为eosbase的trace中获取交易金额
		// 从receiver为eosio.token的trace中获取ram消耗
		// 过滤其他的trace, 防止数据重复
		if receiver != "eosio.token" && receiver != config.EOS_ACCOUNT {
			//log.Printf("receiver is %v\n", receiver)
			//log.Println("!!!!!!!! ignored !!!!!!!!")
			continue
		}
		//log.Println("!!!!!!!!!! a trace to be handled !!!!!!!!")
		act := action.(map[string]interface{})["action_trace"].(map[string]interface{})["act"]
		name := act.(map[string]interface{})["name"].(string)
		if name != "transfer" {
			continue
		}
		data := act.(map[string]interface{})["data"].(map[string]interface{})
		from := data["from"].(string)
		to := data["to"].(string)
		quantity := data["quantity"].(string)
		memo := data["memo"].(string)
		if !utils.IsUserKey(memo) {
			continue
		}

		quantity = utils.ParseQuantity(quantity)
		if from == config.EOS_ACCOUNT {
			quantity = "-" + quantity
			// withdraw
		} else if to == config.EOS_ACCOUNT{
			// deposit
		} else {
			continue  // 不可能
		}

		tf := new(trace)
		tf.receiver = receiver
		tf.realfrom = from
		tf.realto = to
		tf.quantity = quantity
		tf.memo = memo
		tf.seq = action.(map[string]interface{})["account_action_seq"].(float64)
		tf.blockNum = action.(map[string]interface{})["block_num"].(float64)
		tf.txid = action.(map[string]interface{})["action_trace"].(map[string]interface{})["trx_id"].(string)
		tf.accountramdeltas = action.(map[string]interface{})["action_trace"].(map[string]interface{})["account_ram_deltas"].([]interface{})
		tfs = append(tfs, *tf)
	}
	return
}

func addTx(tf trace, confirmed bool) {
	defer func() {
		if r := recover(); r != nil {
			//log.Println(r)
		}
	} ()
	//log.Printf("add transaction %v, confirmed: %v\n", tf.txid, confirmed)
	if strings.HasPrefix(tf.quantity,"-") {
		// lockout
		amt := strings.TrimPrefix(tf.quantity,"-")

		transaction := dao.GetTx(tf.txid)

		// 如果tf的receiver是eosio.token, 扣除fee
		// 如果tf的receiver是eosbase, 修改余额, 增减交易金额, 并更新交易记录, 根据trace增加一条tx Output
		if tf.receiver == "eosio.token" {
			ram := decodeRam(tf)
			fee := uint32(0)
			if ram > 0 && confirmed {
				fee = calculateFee(int64(ram),0,0)
				//log.Printf("transaction fee for %v is %v, payed by %v\n", tf.txid, fee, tf.memo)
				updateBalance(tf.memo, "-"+strconv.Itoa(int(fee)))
			}

			if transaction == nil {
				dao.PutTx(&dao.Transaction{TxHash:tf.txid,FromAddress:tf.memo,Confirmed:confirmed,Fee:fee})
			} else {
				transaction.Confirmed = confirmed
				if confirmed {
					transaction.Fee = transaction.Fee + fee
				}
				dao.PutTx(transaction)
			}

		} else if tf.receiver == config.EOS_ACCOUNT {

			if transaction == nil {
				dao.PutTx(&dao.Transaction{TxHash:tf.txid,FromAddress:tf.memo,TxOutputs:[]dao.TxOutput{dao.TxOutput{Amount:amt,ToAddress:tf.realto,Seq:tf.seq}},Confirmed:confirmed})
			} else {
				transaction.Confirmed = confirmed
				has := false
				for _, out := range transaction.TxOutputs {
					if out.Seq == tf.seq {
						has = true
						break
					}
				}
				if !has {
					transaction.TxOutputs = append(transaction.TxOutputs, dao.TxOutput{Amount:amt,ToAddress:tf.realto,Seq:tf.seq})
				}
				dao.PutTx(transaction)
			}

			if confirmed {
				//log.Printf("updating %v 's balance, balance change is %v\n", tf.memo, tf.quantity)
				updateBalance(tf.memo, tf.quantity)
			}

		}

	} else {
		// lockin
		transaction := dao.GetTx(tf.txid)
		if transaction == nil {
		    log.Debug("=============addTx,transaction is nil=================","confirmed",confirmed,"tx hash",tf.txid)
			dao.PutTx(&dao.Transaction{TxHash:tf.txid,FromAddress:tf.realfrom,TxOutputs:[]dao.TxOutput{dao.TxOutput{Amount:tf.quantity,ToAddress:tf.memo,Seq:tf.seq}},Confirmed:confirmed})
		} else {
		    log.Debug("=============addTx,transaction is not nil=================","confirmed",confirmed,"tx hash",tf.txid)
			transaction.Confirmed = confirmed
			has := false
			for _, out := range transaction.TxOutputs {
				if out.Seq == tf.seq {
					has = true
					break
				}
			}
			if !has {
				transaction.TxOutputs = append(transaction.TxOutputs, dao.TxOutput{Amount:tf.quantity,ToAddress:tf.memo,Seq:tf.seq})
			}
			dao.PutTx(transaction)
		}

		if confirmed {
			//log.Printf("updating %v 's balance, balance change is %v\n", tf.memo, tf.quantity)
			updateBalance(tf.memo, tf.quantity)
		}
	}
}

// 保存成功执行的trace, 更新trace所属的transaction的TxOutputs字段
func addTxs(traces []trace) {
	for _, tf := range traces {
		defer func() {
			if r := recover(); r != nil {
				//log.Println(r)
			}
		} ()
		addTx(tf, false)
	}
}

func updateBalance(userkey string, quantity string) {
	defer func() {
		if r := recover(); r != nil {
			//log.Println(r)
		}
	} ()
	//log.Printf("updating balance for %s...\n", userkey)
	//log.Printf("balance change is %v\n", quantity)
	err = dao.UpdateBalance(userkey, quantity)
	if err != nil {
		//log.Println(err)
	}
}

func PathExists(path string) (bool, error) {
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func initDb() {
	fmt.Println("initiating database")
	err = os.RemoveAll(config.DbPath)
	if err != nil {
		//log.Fatal(err)
	}
	dao.Open()
	dao.Put("cursor","0")
}
