// Copyright 2017 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// faucet is a Fusion faucet backed by a light client.
package main

//go:generate go-bindata -nometadata -o website.go faucet.html
//go:generate gofmt -w -s website.go

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
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
	"github.com/fusion/go-fusion/core"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/ethclient"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/rpc"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/fusion/go-fusion/params"
	"golang.org/x/net/websocket"
)

const (
	faucetDbPath    = "~/.faucet/faucet.db"
	requestCoin      = 20
	refreshInterval = 5 * time.Minute
	quotaNumber     = 1000

	FAUCETTEST = false//true: donot check requesed 20 Coin every day
)

//./faucet --genesis genesis.json --webport 30499 --apiport 9902 --key node2/keystore/UTC--2018-10-12T11-33-28.769681948Z--0963a18ea497b7724340fdfe4ff6e060d3f9e388 --password passwd
var (
	// must
	genesisFlag      = flag.String("genesis", "", "Genesis json file to seed the chain with")
	apiPortFlag      = flag.Int("apiport", 30401, "Listener port for the rpc connection")
	webPortFlag      = flag.Int("webport", 30499, "Listener port for the HTTP API connection")
	netFlag          = flag.Uint64("networkid", 30400, "Network ID to use for the Fusion protocol")
	accJSONFlag      = flag.String("key", "", "Key json file to fund user requests with")
	accPassFlag      = flag.String("password", "", "Decryption password to access faucet funds")

	// maybe
	faucetServerFlag = flag.String("server", "127.0.0.1", "Connect server IP")
	bootFlag         = flag.String("bootnodes", "", "Comma separated bootnode enode URLs to seed with")

	ethPortFlag = flag.Int("ethport", 30303, "Listener port for the devp2p connection")
	statsFlag   = flag.String("ethstats", "", "Ethstats network monitoring auth string")
	netnameFlag = flag.String("name", "", "Network name to assign to the faucet")
	payoutFlag  = flag.Int("amount", 1, "Number of Ethers to pay out per user request")
	minutesFlag = flag.Int("minutes", 1440, "Number of minutes to wait between funding rounds")
	tiersFlag   = flag.Int("tiers", 3, "Number of funding tiers to enable (x3 time, x2.5 funds)")

	captchaToken  = flag.String("token", "", "Recaptcha site key to authenticate client side")
	captchaSecret = flag.String("secret", "", "Recaptcha secret key to authenticate server side")

	noauthFlag = flag.Bool("noauth", false, "Enables funding requests without authentication")
	logFlag    = flag.Int("verbosity", 3, "Log level to use for Fusion and the faucet")
)

var (
	txAccount = "0x0963a18ea497b7724340fdfe4ff6e060d3f9e388" //10240000 * e18
	ks          *keystore.KeyStore
	account     accounts.Account
	faucetdb    *faucet
	refresh     = time.NewTicker(refreshInterval)
	refreshDone = make(chan struct{})
	dbTime      = ""
)

type faucet struct {
	mutex sync.Mutex // protects db
	db    *leveldb.DB
	size  uint64
}

func initDb() {
	db, err := leveldb.OpenFile(faucetDbPath, nil)
	if _, corrupted := err.(*errors.ErrCorrupted); corrupted {
		db, err = leveldb.RecoverFile(faucetDbPath, nil)
	}
	// (Re)check for errors and abort if opening of the db failed
	if err != nil {
		fmt.Printf("Open file %+v failed.\n", faucetDbPath)
		return
	}
	faucetdb = &faucet{db: db, size: 0}
}

func main() {
	log.Debug("==== Faucet() ====\n")
	initDb()

	// Parse the flags and set up the logger to print everything requested
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*logFlag), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	// Construct the payout tiers
	amounts := make([]string, *tiersFlag)
	periods := make([]string, *tiersFlag)
	for i := 0; i < *tiersFlag; i++ {
		// Calculate the amount for the next tier and format it
		amount := float64(*payoutFlag) * math.Pow(2.5, float64(i))
		amounts[i] = fmt.Sprintf("%s Ethers", strconv.FormatFloat(amount, 'f', -1, 64))
		if amount == 1 {
			amounts[i] = strings.TrimSuffix(amounts[i], "s")
		}
		// Calculate the period for the next tier and format it
		period := *minutesFlag * int(math.Pow(3, float64(i)))
		periods[i] = fmt.Sprintf("%d mins", period)
		if period%60 == 0 {
			period /= 60
			periods[i] = fmt.Sprintf("%d hours", period)

			if period%24 == 0 {
				period /= 24
				periods[i] = fmt.Sprintf("%d days", period)
			}
		}
		if period == 1 {
			periods[i] = strings.TrimSuffix(periods[i], "s")
		}
	}
	// Load and parse the genesis block requested by the user
	blob, err := ioutil.ReadFile(*genesisFlag)
	if err != nil {
		log.Crit("Failed to read genesis block contents", "genesis", *genesisFlag, "err", err)
	}
	genesis := new(core.Genesis)
	if err = json.Unmarshal(blob, genesis); err != nil {
		log.Crit("Failed to parse genesis block json", "err", err)
	}
	// Load up the account key and decrypt its password
	if blob, err = ioutil.ReadFile(*accPassFlag); err != nil {
		log.Crit("Failed to read account password contents", "file", *accPassFlag, "err", err)
	}
	// Delete trailing newline in password
	pass := strings.TrimSuffix(string(blob), "\n")

	ks = keystore.NewKeyStore(filepath.Join(os.Getenv("HOME"), ".faucet", "keys"), keystore.StandardScryptN, keystore.StandardScryptP)
	account = ks.Accounts()[0]
	if blob, err = ioutil.ReadFile(*accJSONFlag); err != nil {
		log.Crit("Failed to read account key contents", "file", *accJSONFlag, "err", err)
	}
	acc, err := ks.Import(blob, pass, pass)
	if err != nil {
		log.Crit("Failed to import faucet signer account", "err", err)
	}
       txAccount = fmt.Sprintf("0x%x", account.Address)
       fmt.Printf("txAccount: %v\n", txAccount)
	ks.Unlock(acc, pass)

	if err := listenAndServe(*webPortFlag); err != nil {
		log.Crit("Failed to launch faucet API", "err", err)
	}
}

// listenAndServe registers the HTTP handlers for the faucet and boots it up
// for service user funding requests.
func listenAndServe(port int) error {
	if FAUCETTEST == false {
		go loop()
	}
	http.HandleFunc("/", webHandler)
	http.Handle("/faucet", websocket.Handler(apiHandler))

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

// webHandler handles all non-api requests, simply flattening and returning the
// faucet website.
func webHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("==== webHandler() ====")
	index := []byte("1")
	w.Write(index)
}

// apiHandler handles requests for Ether grants and transaction statuses.
func apiHandler(conn *websocket.Conn) {
	fmt.Printf("\n==== apiHandler() ====\n")
	// Start tracking the connection and drop at the end
	defer func() {
		conn.Close()
		refreshDone = make(chan struct{})
	}()

	rpcserver := fmt.Sprintf("http://%v:%v", *faucetServerFlag, *apiPortFlag)
	fmt.Printf("rpcserver: %+v\n", rpcserver)

	var coinNum = new(big.Int).Mul(big.NewInt(requestCoin), big.NewInt(1000000000000000000))
	for {
		time.Sleep(100 * time.Millisecond)
		// Gather the initial stats from the network to report
		var (
			result hexutil.Uint64
			nonce  uint64
		)
		var msg struct {
			Address     string `json:"address"`
			Tier    uint   `json:"tier"`
			Cointype string `json:"cointype"`
		}
		_ = websocket.JSON.Receive(conn, &msg)
		log.Debug("faucet", "JSON.Receive", msg)
		if len(msg.Address) == 0 {
			log.Debug("faucet, address is null\n")
			send(conn, map[string]string{"state": "ERR", "msg": "Account is nil"}, time.Second)
			if errs := send(conn, map[string]interface{}{
				"state":  "ERROR",
				"funded": nonce,
			}, time.Second); errs != nil {
				log.Warn("Failed to send stats to client", "err", errs)
				conn.Close()
				break
			}
			continue
		}
		if errh := common.IsHexAddress(msg.Address); errh != true {
			log.Debug("faucet, address is invalid.\n")
			send(conn, map[string]string{"state": "ERR", "msg": "Account is invalid"}, time.Second)
			continue
		}
		if faucetdb.size >= quotaNumber {
			log.Debug("faucet", "request coin quota is full.", "")
			send(conn, map[string]string{"state": "ERR", "msg": "Coin request quota is full, please try again tomorrow."}, time.Second)
			continue
		}
		if !FAUCETTEST {
			ret := keyIsExist([]byte(msg.Address))
			if ret == true {
				log.Debug("faucet", "account", msg.Address, "was had requested.", "")
				send(conn, map[string]string{"state": "ERR", "msg": "The account was had requested coin."}, time.Second)
				continue
			}
		}
		if msg.Cointype == "FSN" {
			log.Debug("faucet", "Address", msg.Address)
			log.Debug("faucet", "cointype", msg.Cointype)
			// Ensure the user didn't request funds too recently
			clientc, errc := rpc.Dial(rpcserver)
			if errc != nil {
				log.Debug("client connection error:\n")
				continue
			}
			errc = clientc.CallContext(context.Background(), &result, "eth_getTransactionCount", common.HexToAddress(txAccount), "pending")
			nonce = uint64(result)
			log.Debug("faucet", "nonce", nonce)
			tx := types.NewTransaction(nonce, common.HexToAddress(msg.Address), coinNum, 21000, big.NewInt(41000), nil)
			chainid := params.FsnChainConfig.ChainID.Int64()
			signed, err := ks.SignTx(account, tx, big.NewInt(chainid))
			if err != nil {
				if err = send(conn, map[string]string{"state": "ERR", "msg": "SignTx failed."}, time.Second); err != nil {
					log.Warn("Failed to Sign transaction", "err", err)
					return
				}
			}
			// Submit the transaction and mark as funded if successful
			log.Debug("faucet", "HTTP-RPC client connected", rpcserver)

			log.Debug("Faucet", "addr", msg.Address, "coinNum", coinNum)
			// Send RawTransaction to ethereum network
			client, err := ethclient.Dial(rpcserver)
			if err != nil {
				log.Debug("client connection error.\n")
				continue
			}
			err = client.SendTransaction(context.Background(), signed)
			if err != nil {
				send(conn, map[string]string{"state": "ERR", "msg": "Send Transaction Failed."}, time.Second)
				log.Trace("faucet", "client send error", err)
			} else {
				send(conn, map[string]string{"state": "OK", "msg": "Send Transaction Successed.\nIt takes about 1~2 blocks (time) to get to the account."}, time.Second)
				log.Debug("faucet", "client send", "success")
				if putKeyToDb([]byte(msg.Address)) != nil {
					log.Debug("PutKeyToDb account: %+v failed.\n", msg.Address)
				}
			}
		}
	}
}

// sends transmits a data packet to the remote end of the websocket, but also
// setting a write deadline to prevent waiting forever on the node.
func send(conn *websocket.Conn, value interface{}, timeout time.Duration) error {
	log.Debug("faucet", "send, value", value)
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	conn.SetWriteDeadline(time.Now().Add(timeout))
	return websocket.JSON.Send(conn, value)
}

func loop() {
	log.Debug("==== loop() ====\n")
	refreshDone = nil
	dbTime = getDbDate()

	for {
		select {
		case <-refresh.C:
			refreshDb()
		case <-refreshDone:
			faucetdb.db.Close()
			break
		}
	}
}

func refreshDb() {
	if dbTime == "" {
		return
	}
	at := fmt.Sprintf("%+v", time.Now())
	n := strings.Split(at, " ")
	if dbTime != string(n[0]) {
		dbTime = string(n[0])
		emptyDb()
	}
}

func emptyDb() {
	log.Debug("==== emptyDb() ====\n")
	iter := faucetdb.db.NewIterator(nil, nil)
	defer iter.Release()
	for iter.Next() {
		faucetdb.db.Delete(iter.Key(), nil)
	}
	faucetdb.size = 0
}

func getDbDate() string {
	log.Debug("==== getDbDate() ====\n")
	iter := faucetdb.db.NewIterator(nil, nil)
	defer iter.Release()
	for iter.Next() {
		d := strings.Split(string(iter.Value()), " ")
		return string(d[0])
	}
	return ""
}

func keyIsExist(key []byte) bool {
	_, err := faucetdb.db.Get(key, nil)
	if err != nil {
		return false
	}
	return true
}

func putKeyToDb(key []byte) error {
	at := fmt.Sprintf("%+v", time.Now())
	err := faucetdb.db.Put(key, []byte(at), nil)
	if err != nil {
		log.Debug("putKeyToDb", "put, key", key, "faied", "")
		return err
	}
	faucetdb.size += 1
	return nil
}
