package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fusion/go-fusion/accounts"
	"github.com/fusion/go-fusion/accounts/keystore"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/common/hexutil"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/ethclient"
	"github.com/fusion/go-fusion/internal/ethapi"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/params"
	//"github.com/fusion/go-fusion/rpc"
)

//./dexorder --key node3/keystore/UTC--2019-04-29T03-45-23.940116300Z--33ee8b7a872701794afca3357c1cfc1791865541 --password passwd --rpcport 9909 --t 5000 --p 2 --q 1,20 --side sell --trade ETH/BTC --price 0.01 --a 6

var (
	serverFlag      = flag.String("server", "127.0.0.1", "Connect server IP")
	rpcPortFlag     = flag.Uint("rpcport", 9901, "Listener port for the rpc connection")
	accountJSONFlag = flag.String("key", "", "Key json file to fund user requests with")
	accountPassFlag = flag.String("password", "", "Decryption password to access dextest funds")
	sideFlag        = flag.String("side", "buy", "buy/sell")
	tradeFlag       = flag.String("trade", "ETH/BTC", "Transaction pairs: ETH/BTC,XVC/BTC,XVC/ETH")
	roundFlag       = flag.Uint64("p", 1, "palrallel per one time")
	priceFlag       = flag.Float64("price", 0.0, "Transaction price")
	quantityFlag    = flag.String("q", "1,100", "Quantity of order")
	timerTxFlag     = flag.Uint64("t", 5000, "timer to buy/sell: Milliseconds")
	timerPriceFlag  = flag.Uint64("timerp", 5, "timer to getPrice: Seconds")
	accuracyFlag    = flag.Uint("a", 3, "accuracy to number (1-6)")
)

var (
	binanceAPI        = "https://api.binance.com/api/v3/ticker/price"
	ks                *keystore.KeyStore
	account           accounts.Account
	price             *Price
	rpcServer         = ""
	priceOfBinance    = 0.0
	timeConst         = 5 * time.Second
	timePriceConst    = 1 * time.Second
	quantityBaseConst = 1
	quantityStepConst = 100
)

type Price struct {
	price string
	sync.Mutex
}

func main() {
	flag.Parse()
	price = &Price{price: ""}
	timeConst = time.Duration(*timerTxFlag) * time.Millisecond
	timePriceConst = time.Duration(*timerPriceFlag) * time.Second
	//if !types.IsValideTrade(*tradeFlag) {
	//	fmt.Printf("getPrice, error Tx pair: %s\n", *tradeFlag)
	//	return
	//}
	errs := parseSideArgs(*sideFlag)
	if errs != nil {
		return
	}
	errq := parseQuantityArgs(*quantityFlag)
	if errq != nil {
		return
	}
	err := unlockAccount(*accountJSONFlag, *accountPassFlag)
	if err != nil {
		return
	}

	rpcServer = fmt.Sprintf("http://%v:%v", *serverFlag, *rpcPortFlag)
	fmt.Printf("rpcServer: %+v\n", rpcServer)

	if *priceFlag <= 0.0 {
		go getPrice()
	} else {
		priceOfBinance = *priceFlag
	}

	sendTimer := time.NewTicker(timeConst)
	defer sendTimer.Stop()
	for {
		go sendTx()
		<-sendTimer.C
	}
}

type priceJson struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

func sendTx() {
	pr := priceOfBinance

	if pr == 0.0 {
		return
	}
	// Send tx
	client, err := ethclient.Dial(rpcServer)
	if err != nil {
		log.Debug("client connection error.\n")
		fmt.Printf("client connection error, ethclient.Dial\n")
		return
	}
	defer client.Close()

	i := uint64(0)
	acc := 1000000.0
	if (*accuracyFlag) <= 6 {
		acc = math.Pow10((int)(*accuracyFlag))
	}
	for ; i < *roundFlag; i++ {
		rand.Seed(time.Now().UnixNano())
		//if strings.EqualFold(*sideFlag, "buy") == true {
		r := rand.Float64() * pr * 0.2
		prR := pr + r
		//}
		pri := fmt.Sprintf("%f", prR)
		num := fmt.Sprintf("%f", float64(rand.Intn(quantityStepConst)+quantityBaseConst)+(float64)(rand.Intn(int(acc + 0.5)))/acc)
		go sendOrder(client, account.Address.String(), *sideFlag, pri, num)
	}
}

func sendOrder(client *ethclient.Client, fusionaddr, side, price, num string) {
	nonce := uint64(0)
	fromaddr, _ := new(big.Int).SetString(fusionaddr, 0)
	txfrom := common.BytesToAddress(fromaddr.Bytes())

	toaddr := new(common.Address)
	*toaddr = types.XvcPrecompileAddr
	args := ethapi.SendTxArgs{From: txfrom, To: toaddr}
	timestamp := fmt.Sprintf("%v", time.Now().UnixNano())
	str := "ORDER" + ":" + fusionaddr + ":" + *tradeFlag + ":" + "LimitOrder" + ":" + side + ":" + price + ":" + num + ":" + "GTE" + ":" + timestamp
	fmt.Printf("%v\n", str)
	args.Data = new(hexutil.Bytes)
	args.Input = new(hexutil.Bytes)
	*args.Data = []byte(str)
	*args.Input = []byte(str)
	args.Value = new(hexutil.Big)

	// Assemble the transaction and sign with the wallet
	tx := types.NewTransaction(nonce, *(args.To), (*big.Int)(args.Value), 90000, big.NewInt(41000), *args.Data)
	chainID := params.FsnChainConfig.ChainID
	signed, err := ks.SignTx(account, tx, chainID)
	if err != nil {
		log.Warn("Failed to Sign transaction", "err", err)
		return
	}

	err = client.SendTransaction(context.Background(), signed)
	if err != nil {
		log.Trace("sendOrder", "client send error", err)
		return
	}
}

func getPrice() {
	tradeString := strings.Replace(*tradeFlag, "/", "", 1)
	tradeString = strings.Replace(tradeString, "XVC", "BNB", 3)
	url := fmt.Sprintf("%s?symbol=%s", binanceAPI, tradeString)
	fmt.Printf("API '%s'\n", url)
	priceTimer := time.NewTicker(timePriceConst)
	defer priceTimer.Stop()
	for {
		resp, _ := http.Get(url)
		if resp != nil {
			body, _ := ioutil.ReadAll(resp.Body)
			if body != nil {
				price.price = string(body)
				var pj priceJson
				if errj := json.Unmarshal([]byte(price.price), &pj); errj == nil {
					priceOfBinance, _ = strconv.ParseFloat(pj.Price, 64)
					//fmt.Printf("priceOfBinance = %v\n", priceOfBinance)
				}
			}
			resp.Body.Close()
		}
		<-priceTimer.C
	}
}

func unlockAccount(accountJSONFlag, accountPassFlag string) error {
	if accountJSONFlag == "" || accountPassFlag == "" {
		return fmt.Errorf("error", "accountJSONFlag", accountJSONFlag, "accountPassFlag", accountPassFlag)
	}
	// Load up the account key and decrypt its password
	blob, err := ioutil.ReadFile(accountPassFlag)
	if err != nil {
		log.Crit("Failed to read account password contents", "file", accountPassFlag, "err", err)
		return err
	}
	// Delete trailing newline in password
	pass := strings.TrimSuffix(string(blob), "\n")

	ks = keystore.NewKeyStore(filepath.Join(os.Getenv("HOME"), ".testAPI", "keys"), keystore.StandardScryptN, keystore.StandardScryptP)

	blob, err = ioutil.ReadFile(accountJSONFlag)
	if err != nil {
		log.Crit("Failed to read account key contents", "file", accountJSONFlag, "err", err)
		return err
	}
	account, err = ks.Import(blob, pass, pass)
	if err != nil {
		log.Crit("Failed to import dextest signer account", "err", err)
		return err
	}
	ks.Unlock(account, pass)
	return nil
}

func parseQuantityArgs(quantityFlag string) error {
	q0 := quantityBaseConst
	q1 := quantityStepConst
	q := strings.Split(quantityFlag, ",")
	if len(q) == 1 {
		qtem, _ := strconv.Atoi(q[0])
		if qtem > 1 {
			q1 = qtem
		}
	} else if len(q) == 2 {
		qtem1, _ := strconv.Atoi(q[0])
		qtem2, _ := strconv.Atoi(q[1])
		if qtem1 < qtem2 {
			q0 = qtem1
			q1 = qtem2
		} else if qtem1 == qtem2 {
			if qtem1 > 1 {
				q1 = qtem1
			}
		} else {
			log.Warn("quantityFlag", "params(default)", "1,100")
		}
	} else {
		log.Warn("quantityFlag", "params(default)", "1,100")
	}
	quantityBaseConst = q0
	quantityStepConst = q1 - q0 + 1
	return nil
}

func parseSideArgs(sideFlag string) error {
	if strings.EqualFold(sideFlag, "buy") == false && strings.EqualFold(sideFlag, "sell") == false {
		return fmt.Errorf("error", "sideFlag", sideFlag)
	}
	return nil
}
