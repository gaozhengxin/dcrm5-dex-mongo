// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"
	"strings"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/common/prque"
	"github.com/fusion/go-fusion/core/state"
	"github.com/fusion/go-fusion/core/types"
	"github.com/fusion/go-fusion/event"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/metrics"
	"github.com/fusion/go-fusion/params"
	"github.com/fusion/go-fusion/crypto/dcrm"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/core/rawdb"
	"github.com/fusion/go-fusion/core/vm"
	//"github.com/fusion/go-fusion/mongodb"
	"github.com/shopspring/decimal"
	"regexp"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
	"container/list"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSender = errors.New("invalid sender")

	// ErrNonceTooLow is returned if the nonce of a transaction is lower than the
	// one present in the local chain.
	ErrNonceTooLow = errors.New("nonce too low")

	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrUnderpriced = errors.New("transaction underpriced")

	// ErrReplaceUnderpriced is returned if a transaction is attempted to be replaced
	// with a different one without the required price bump.
	ErrReplaceUnderpriced = errors.New("replacement transaction underpriced")

	// ErrInsufficientFunds is returned if the total cost of executing a transaction
	// is higher than the balance of the user's account.
	ErrInsufficientFunds = errors.New("insufficient funds for gas * price + value")

	// ErrIntrinsicGas is returned if the transaction is specified to use less gas
	// than required to start the invocation.
	ErrIntrinsicGas = errors.New("intrinsic gas too low")

	// ErrGasLimit is returned if a transaction's requested gas limit exceeds the
	// maximum allowance of the current block.
	ErrGasLimit = errors.New("exceeds block gas limit")

	// ErrNegativeValue is a sanity error to ensure noone is able to specify a
	// transaction with a negative value.
	ErrNegativeValue = errors.New("negative value")

	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrOversizedData = errors.New("oversized data")
)

var (
	evictionInterval    = time.Minute     // Time interval to check for evictable transactions
	statsReportInterval = 8 * time.Second // Time interval to report transaction pool stats
)

var (
	// Metrics for the pending pool
	pendingDiscardCounter   = metrics.NewRegisteredCounter("txpool/pending/discard", nil)
	pendingReplaceCounter   = metrics.NewRegisteredCounter("txpool/pending/replace", nil)
	pendingRateLimitCounter = metrics.NewRegisteredCounter("txpool/pending/ratelimit", nil) // Dropped due to rate limiting
	pendingNofundsCounter   = metrics.NewRegisteredCounter("txpool/pending/nofunds", nil)   // Dropped due to out-of-funds

	// Metrics for the queued pool
	queuedDiscardCounter   = metrics.NewRegisteredCounter("txpool/queued/discard", nil)
	queuedReplaceCounter   = metrics.NewRegisteredCounter("txpool/queued/replace", nil)
	queuedRateLimitCounter = metrics.NewRegisteredCounter("txpool/queued/ratelimit", nil) // Dropped due to rate limiting
	queuedNofundsCounter   = metrics.NewRegisteredCounter("txpool/queued/nofunds", nil)   // Dropped due to out-of-funds

	// General tx metrics
	invalidTxCounter     = metrics.NewRegisteredCounter("txpool/invalid", nil)
	underpricedTxCounter = metrics.NewRegisteredCounter("txpool/underpriced", nil)
	cancelordercb   func(string)
	updateobcb   func(string)
)

//for orderbook
func RegisterCancelOrdercb(recvObFunc func(string)) {
	cancelordercb = recvObFunc
}

func RegisterUpdateOBcb(recvObFunc func(string)) {
	updateobcb = recvObFunc
}

//////////////
var (
    FSN     *TxPool 
)

// TxStatus is the current status of a transaction as seen by the pool.
type TxStatus uint

const (
	TxStatusUnknown TxStatus = iota
	TxStatusQueued
	TxStatusPending
	TxStatusIncluded
)

// blockChain provides the state of blockchain and current gas limit to do
// some pre checks in tx pool and event subscribers.
type blockChain interface {
	CurrentBlock() *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription
}

// TxPoolConfig are the configuration parameters of the transaction pool.
type TxPoolConfig struct {
	Locals    []common.Address // Addresses that should be treated by default as local
	NoLocals  bool             // Whether local transaction handling should be disabled
	Journal   string           // Journal of local transactions to survive node restarts
	Rejournal time.Duration    // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultTxPoolConfig = TxPoolConfig{
	Journal:   "transactions.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *TxPoolConfig) sanitize() TxPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid txpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	if conf.PriceLimit < 1 {
		log.Warn("Sanitizing invalid txpool price limit", "provided", conf.PriceLimit, "updated", DefaultTxPoolConfig.PriceLimit)
		conf.PriceLimit = DefaultTxPoolConfig.PriceLimit
	}
	if conf.PriceBump < 1 {
		log.Warn("Sanitizing invalid txpool price bump", "provided", conf.PriceBump, "updated", DefaultTxPoolConfig.PriceBump)
		conf.PriceBump = DefaultTxPoolConfig.PriceBump
	}
	return conf
}

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type TxPool struct {
	config       TxPoolConfig
	chainconfig  *params.ChainConfig
	chain        blockChain
	gasPrice     *big.Int
	txFeed       event.Feed
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent
	chainHeadSub event.Subscription
	signer       types.Signer
	mu           sync.RWMutex

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas uint64              // Current gas limit for transaction caps

	locals  *accountSet // Set of local transaction to exempt from eviction rules
	journal *txJournal  // Journal of local transaction to back up to disk

	pending map[common.Address]*txList   // All currently processable transactions
	queue   map[common.Address]*txList   // Queued but non-processable transactions
	beats   map[common.Address]time.Time // Last heartbeat from each known account
	all     *txLookup                    // All transactions to allow lookups
	priced  *txPricedList                // All transactions sorted by price

	wg sync.WaitGroup // for shutdown sync

	homestead bool
	
	ALO *list.List  //TODO
	Los *list.List
}

// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewTxPool(config TxPoolConfig, chainconfig *params.ChainConfig, chain blockChain) *TxPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	config = (&config).sanitize()

	//log.Debug("===================NewTxPool==================","chain id",chainconfig.ChainID)
	// Create the transaction pool with its initial settings
	pool := &TxPool{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,
		signer:      types.NewEIP155Signer(chainconfig.ChainID),
		pending:     make(map[common.Address]*txList),
		queue:       make(map[common.Address]*txList),
		beats:       make(map[common.Address]time.Time),
		all:         newTxLookup(),
		chainHeadCh: make(chan ChainHeadEvent, chainHeadChanSize),
		gasPrice:    new(big.Int).SetUint64(config.PriceLimit),
		ALO: list.New(),
		Los: list.New(),
	}
	pool.locals = newAccountSet(pool.signer)
	for _, addr := range config.Locals {
		log.Info("Setting new local account", "address", addr)
		pool.locals.add(addr)
	}
	pool.priced = newTxPricedList(pool.all)
	pool.reset(nil, chain.CurrentBlock().Header())

	// If local transactions and journaling is enabled, load from disk
	if !config.NoLocals && config.Journal != "" {
		pool.journal = newTxJournal(config.Journal)

		if err := pool.journal.load(pool.AddLocals); err != nil {
			log.Warn("Failed to load transaction journal", "err", err)
		}
		if err := pool.journal.rotate(pool.local()); err != nil {
			log.Warn("Failed to rotate transaction journal", "err", err)
		}
	}
	// Subscribe events from blockchain
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	FSN = pool

	return pool
}

// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *TxPool) loop() {
	defer pool.wg.Done()

	// Start the stats reporting and transaction eviction tickers
	var prevPending, prevQueued, prevStales int

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()

	// Track the previous head headers for transaction reorgs
	head := pool.chain.CurrentBlock()

	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
			if ev.Block != nil {
				pool.mu.Lock()
				if pool.chainconfig.IsHomestead(ev.Block.Number()) {
					pool.homestead = true
				}

				//log.Info("============get chain head ch and start reset txpool.==============")
				pool.reset(head.Header(), ev.Block.Header())
				head = ev.Block

				pool.mu.Unlock()
			}
		// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

		// Handle stats reporting ticks
		case <-report.C:
			pool.mu.RLock()
			pending, queued := pool.stats()
			stales := pool.priced.stales
			pool.mu.RUnlock()
			//log.Debug("===========txpool.loop,","pending",pending,"prevPending",prevPending,"queued",queued,"prevQueued",prevQueued,"stales",stales,"prevStales",prevStales,"","==============")

			if pending != prevPending || queued != prevQueued || stales != prevStales {
				log.Debug("Transaction pool status report", "executable", pending, "queued", queued, "stales", stales)
				prevPending, prevQueued, prevStales = pending, queued, stales
			}

		// Handle inactive account transaction eviction
		case <-evict.C:
			pool.mu.Lock()
			for addr := range pool.queue {
				// Skip local transactions from the eviction mechanism
				if pool.locals.contains(addr) {
					continue
				}
				// Any non-locals old enough should be removed
				if time.Since(pool.beats[addr]) > pool.config.Lifetime {
					for _, tx := range pool.queue[addr].Flatten() {
						pool.removeTx(tx.Hash(), true)
					}
				}
			}
			pool.mu.Unlock()

		// Handle local transaction journal rotation
		case <-journal.C:
			if pool.journal != nil {
				pool.mu.Lock()
				if err := pool.journal.rotate(pool.local()); err != nil {
					log.Warn("Failed to rotate local tx journal", "err", err)
				}
				pool.mu.Unlock()
			}
		}
	}
}

// lockedReset is a wrapper around reset to allow calling it in a thread safe
// manner. This method is only ever used in the tester!
func (pool *TxPool) lockedReset(oldHead, newHead *types.Header) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.reset(oldHead, newHead)
}

// reset retrieves the current state of the blockchain and ensures the content
// of the transaction pool is valid with regard to the chain state.
func (pool *TxPool) reset(oldHead, newHead *types.Header) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject types.Transactions

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number.Uint64()
		newNum := newHead.Number.Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory
			var discarded, included types.Transactions

			var (
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.Number.Uint64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.Number.Uint64())
			)
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.Transactions()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					reorgordercb(reinject)
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					reorgordercb(reinject)
					return
				}
			}
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.Transactions()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					reorgordercb(reinject)
					return
				}
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					reorgordercb(reinject)
					return
				}
			}
			reinject = types.TxDifference(discarded, included)
			//for _,re := range reinject {
			  //  from,_ := pool.GetFrom(re)
			    //log.Info("========pool.reset,reinject tx.=========","tx nonce",re.Nonce(),"tx hash",re.Hash(),"tx from",from)
			//}
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil {
		newHead = pool.chain.CurrentBlock().Header() // Special case during testing
	}
	statedb, err := pool.chain.StateAt(newHead.Root)
	if err != nil {
		log.Error("Failed to reset txpool state", "err", err)
		reorgordercb(reinject)
		return
	}
	pool.currentState = statedb
	pool.pendingState = state.ManageState(statedb)
	pool.currentMaxGas = newHead.GasLimit
	log.Debug("============txpool.reset=================","current Max Gaslimit",pool.currentMaxGas)

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))
	senderCacher.recover(pool.signer, reinject)
	pool.addTxsLocked(reinject, false)

	// validate the pool of pending transactions, this will remove
	// any transactions that have been included in the block or
	// have been invalidated because of another transaction (e.g.
	// higher gas price)
	pool.demoteUnexecutables()

	// Update all accounts to the latest known pending nonce
	for addr, list := range pool.pending {
		txs := list.Flatten() // Heavy but will be cached and is needed by the miner anyway
		pool.pendingState.SetNonce(addr, txs[len(txs)-1].Nonce()+1)
	}
	// Check the queue and move transactions over to the pending if possible
	// or remove those that have become invalid
	pool.promoteExecutables(nil)

	////
	//log.Info("==============txpool.reset,call ReOrgOrder==================")
	reorgordercb(reinject)
}

func (pool *TxPool) WaitStop() {
	waitstopcb()
}

func (pool *TxPool) ExsitInPending(tx *types.Transaction) bool {
    for _, list := range pool.pending {
	txs2 := list.Flatten()
	for _,v := range txs2 {
	    if strings.EqualFold(tx.Hash().Hex(),v.Hash().Hex()) {
		return true
	    }
	}
    }

    return false
}

// Stop terminates the transaction pool.
func (pool *TxPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	if pool.journal != nil {
		pool.journal.close()
	}
	log.Info("Transaction pool stopped")
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent and
// starts sending event to the given channel.
func (pool *TxPool) SubscribeNewTxsEvent(ch chan<- NewTxsEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *TxPool) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}

// SetGasPrice updates the minimum price required by the transaction pool for a
// new transaction, and drops all transactions below this threshold.
func (pool *TxPool) SetGasPrice(price *big.Int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.gasPrice = price
	for _, tx := range pool.priced.Cap(price, pool.locals) {
		pool.removeTx(tx.Hash(), false)
	}
	log.Info("Transaction pool price threshold updated", "price", price)
}

// State returns the virtual managed state of the transaction pool.
func (pool *TxPool) State() *state.ManagedState {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingState
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) Stats() (int, int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.stats()
}

// stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) stats() (int, int) {
	pending := 0
	for _, list := range pool.pending {
		pending += list.Len()
	}
	queued := 0
	for _, list := range pool.queue {
		queued += list.Len()
	}
	return pending, queued
}

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (pool *TxPool) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	queued := make(map[common.Address]types.Transactions)
	for addr, list := range pool.queue {
		queued[addr] = list.Flatten()
	}
	return pending, queued
}

// Pending retrieves all currently processable transactions, grouped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) Pending() (map[common.Address]types.Transactions, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	return pending, nil
}

// Locals retrieves the accounts currently considered local by the pool.
func (pool *TxPool) Locals() []common.Address {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	return pool.locals.flatten()
}

// local retrieves all currently known local transactions, grouped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) local() map[common.Address]types.Transactions {
	txs := make(map[common.Address]types.Transactions)
	for addr := range pool.locals.accounts {
		if pending := pool.pending[addr]; pending != nil {
			txs[addr] = append(txs[addr], pending.Flatten()...)
		}
		if queued := pool.queue[addr]; queued != nil {
			txs[addr] = append(txs[addr], queued.Flatten()...)
		}
	}
	return txs
}

// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPool) validateTx(tx *types.Transaction, local bool) error {
        //log.Debug("========validateTx========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	// Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	if types.IsXvcTx(tx) { //TODO
	    /*if tx.Size() > 160*1024 {
		log.Debug("========validateTx,fail:tx is order tx and tx.size>160*1024===========","tx.Size()",tx.Size(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrOversizedData
	    }*//// TODO  tmp
	} else if tx.Size() > 32*1024 {
	    log.Debug("========validateTx,fail:tx.size>32*1024===========","tx.Size()",tx.Size(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrOversizedData
	}
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	//log.Debug("=============validateTx,step 1===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	if tx.Value().Sign() < 0 {
	    log.Debug("===========validateTx,fail:value < 0======","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrNegativeValue
	}
	// Ensure the transaction doesn't exceed the current block limit gas.
	//log.Debug("=============validateTx,step 2===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	if pool.currentMaxGas < tx.Gas() {
	    log.Debug("=========validateTx,fail:currentMaxGas < tx.Gas=======","currentMaxGas",pool.currentMaxGas,"tx.Gas",tx.Gas(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrGasLimit
	}
	//log.Debug("=============validateTx,step 3.===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	// Make sure the transaction is signed properly
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
	    log.Debug("==========validateTx,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrInvalidSender
	}
	// Drop non-local transactions under our own minimal accepted gas price
	//log.Debug("=============validateTx,step 4.===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	local = local || pool.locals.contains(from) // account may be local even if the transaction arrived from the network
        //log.Debug("============validateTx========","pool.gasPrice",pool.gasPrice,"tx.GasPrice()",tx.GasPrice(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	if !local && pool.gasPrice.Cmp(tx.GasPrice()) > 0 {
	    log.Debug("===========validateTx,fail: !local && pool.gasPrice.Cmp(tx.GasPrice()) > 0=============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrUnderpriced
	}
	
	// Ensure the transaction adheres to nonce ordering
	//log.Debug("=============validateTx,step 5.===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	if !types.IsDcrmLockIn(tx) && !types.IsDcrmConfirmAddr(tx) && !types.IsXvcTx(tx) && pool.currentState.GetNonce(from) > tx.Nonce() {
	    log.Debug("===================validateTx,fail: ErrNonceTooLow=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrNonceTooLow
	}
	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	//log.Debug("=============validateTx,step 6===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	if !types.IsDcrmLockIn(tx) && !types.IsDcrmConfirmAddr(tx) && !types.IsXvcTx(tx) && pool.currentState.GetBalance(from,"FSN").Cmp(tx.Cost()) < 0 {
		log.Debug("===============validateTx,ErrInsufficientFunds","from",from.Hex(),"","=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrInsufficientFunds
	}

	//log.Debug("=============validateTx,step 7.===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	intrGas, err := IntrinsicGas(tx.Data(), tx.To() == nil, pool.homestead)
	if err != nil {
	    log.Debug("===================validateTx,fail: intrGas error.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return err
	}
	
	//log.Debug("=============validateTx,step 8.===========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	if !types.IsDcrmLockIn(tx) && !types.IsDcrmConfirmAddr(tx) && !types.IsXvcTx(tx) &&  tx.Gas() < intrGas {
	    log.Debug("===================validateTx,fail: tx.Gas() < intrGas=================","intrGas",intrGas,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return ErrIntrinsicGas
	}

	//log.Debug("===============validateTx=============finish.","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return nil
}

func isValidBtcValue(s string) bool {
    if s == "" {
	return false
    }

    nums := []rune(s)
    for k,_ := range nums {
	if string(nums[k:k+1]) != "0" && string(nums[k:k+1]) != "1" && string(nums[k:k+1]) != "2" && string(nums[k:k+1]) != "3" && string(nums[k:k+1]) != "4" && string(nums[k:k+1]) != "5" && string(nums[k:k+1]) != "6" && string(nums[k:k+1]) != "7" && string(nums[k:k+1]) != "8" && string(nums[k:k+1]) != "9" {
	    return false
	}
    }

    return true
}

func isValidBtcValue2(s string) bool {
    if s == "" {
	return false
    }

    i := 0
    nums := []rune(s)
    for k,_ := range nums {
	if string(nums[k:k+1]) == "." {
	    i++
	    if k == 0 || k == (len(nums)-1) {
		return false
	    }
	    if i >= 2 {
		return false
	    }

	} else if string(nums[k:k+1]) != "0" && string(nums[k:k+1]) != "1" && string(nums[k:k+1]) != "2" && string(nums[k:k+1]) != "3" && string(nums[k:k+1]) != "4" && string(nums[k:k+1]) != "5" && string(nums[k:k+1]) != "6" && string(nums[k:k+1]) != "7" && string(nums[k:k+1]) != "8" && string(nums[k:k+1]) != "9" {
	    return false
	}
    }

    return true
}

func isDecimalNumber(s string) bool {
    if s == "" {
	return false
    }

    nums := []rune(s)
    for k,_ := range nums {
	if string(nums[k:k+1]) != "0" && string(nums[k:k+1]) != "1" && string(nums[k:k+1]) != "2" && string(nums[k:k+1]) != "3" && string(nums[k:k+1]) != "4" && string(nums[k:k+1]) != "5" && string(nums[k:k+1]) != "6" && string(nums[k:k+1]) != "7" && string(nums[k:k+1]) != "8" && string(nums[k:k+1]) != "9" {
	    return false
	}
    }

    return true
}

//transaction
func (pool *TxPool) checkTransaction(tx *types.Transaction) (bool,error) {
    if !types.IsDcrmTransaction(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    if len(inputs) == 0 {
	return false,dcrm.GetRetErr(dcrm.ErrTxDataError)
    }

    fusionto:= inputs[1]
    value := inputs[2]
    cointype := inputs[3]

    if len(inputs) < 4 || fusionto == "" || value == "" || cointype == "" {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========checkTransaction,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    if dcrm.IsValidFusionAddr(fusionto) == false {
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }
    
    if !isDecimalNumber(value) {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    if cryptocoins.IsCoinSupported(cointype) == false {
	return false, dcrm.GetRetErr(dcrm.ErrCoinTypeNotSupported)
    }

    //check value
    if reg, _ := regexp.Compile("^[0-9]+?$"); reg.Match([]byte(value)) == false {
	    log.Debug("==========checkTransaction=================","err",dcrm.GetRetErr(dcrm.ErrParamError),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }
    amount,_ := new(big.Int).SetString(value,10)

    chandler := cryptocoins.NewCryptocoinHandler(cointype)
    if chandler == nil {
	return false, dcrm.GetRetErr(dcrm.ErrCoinTypeNotSupported)
    }

    if strings.EqualFold(cointype,"ETH") == true || strings.EqualFold(cointype,"ERC20GUSD") == true || strings.EqualFold(cointype,"BNB") == true || strings.EqualFold(cointype,"ERC20MKR") == true || strings.EqualFold(cointype,"ERC20HT") == true || strings.EqualFold(cointype,"ERC20BNT") == true || strings.EqualFold(cointype,"ERC20RMBT") {
	if !isDecimalNumber(value) {
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
	}

	//amount,_ := new(big.Int).SetString(value,10)

	 ret := pool.currentState.GetBalance(from,cointype)
	 if ret != nil {
	    if ret.Cmp(amount) < 0 {
		return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
	    }

	    return true,nil
	 } else {
		return false,err
	 }
    }

    if strings.EqualFold(cointype,"BTC") {
	if !isValidBtcValue(value) {
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
	}

	//amount,_ := new(big.Int).SetString(value,10)
	one,_ := new(big.Int).SetString("1",10)
	if amount.Cmp(one) < 0 {
	    //return false,errors.New("value must great than or equal 1 satoshis.")
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
	}
	 
	 ret := pool.currentState.GetBalance(from,cointype)
	if ret != nil {
	    if ret.Cmp(amount) < 0 {
		return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
	    }

	    return true,nil
	} else {
		return false,err
	}
    }

    ret := pool.currentState.GetBalance(from,cointype)
    if ret != nil {
	if ret.Cmp(amount) < 0 {
	    return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
	}

    } else {
	return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
    }

    return true,nil
}

func (pool *TxPool) validateTransaction(tx *types.Transaction) (bool,error) {
    if !types.IsDcrmTransaction(tx) {
	return true,nil 
    }

    return true,nil
}

//lockout
func (pool *TxPool) checkLockout(tx *types.Transaction) (bool,error) {
    //log.Debug("============txPool.checkLockout=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
    if !types.IsDcrmLockOut(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    if len(inputs) == 0 {
	log.Debug("==========checkLockout=================","err",dcrm.GetRetErr(dcrm.ErrTxDataError),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrTxDataError)
    }

    lockoutto := inputs[1]
    value := inputs[2]
    cointype := inputs[3]

    if len(inputs) < 4 || lockoutto == "" || value == "" || cointype == "" {
	log.Debug("==========checkLockout=================","err",dcrm.GetRetErr(dcrm.ErrParamError),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    if cryptocoins.IsCoinSupported(cointype) == false {
	return false, dcrm.GetRetErr(dcrm.ErrCoinTypeNotSupported)
    }

    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========checkLockout=================","err",ErrInvalidSender,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    //check value
    if reg, _ := regexp.Compile("^[0-9]+?$"); reg.Match([]byte(value)) == false {
	    log.Debug("==========checkLockout=================","err",dcrm.GetRetErr(dcrm.ErrParamError),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }
    amount,_ := new(big.Int).SetString(value,10)

    if chandler := cryptocoins.NewCryptocoinHandler(cointype); chandler.IsToken() == false {
	//============case1: coin=========
	ret := pool.currentState.GetBalance(from,cointype)
	if ret != nil {
	     //chandler := cryptocoins.NewCryptocoinHandler(cointype)
	     fee := chandler.GetDefaultFee()
	     //total := new(big.Int).Add(amount,fee)
	     total := new(big.Int).Add(amount,fee.Val)
	     //log.Debug("==========checkLockout=================","from",from,"cointype",cointype,"get dcrm addr balance",ret,"lockout value",amount,"fee",fee,"amount+fee",total,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    if ret.Cmp(total) < 0 {
		log.Debug("==========checkLockout=================","err",dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
	    }
	} else {
	    log.Debug("==========checkLockout=================","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}
 } else {
     //============case2: token========
     fee:= chandler.GetDefaultFee()
     ret1 := pool.currentState.GetBalance(from,cointype)
     ret2 := pool.currentState.GetBalance(from,fee.Cointype)
     if ret1 != nil && ret2 != nil {
	 //log.Debug("========checkLockout, token balance and coin balance========", "token balance", ret1, "coin balance", ret2)
	 if ret1.Cmp(amount) < 0 {
		 // token余额不足
		 log.Debug("==========checkLockout,token balance insufficient=================","err",dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		 return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
	 }
	 if ret2.Cmp(fee.Val) < 0 {
		 // fee不足
		 log.Debug("==========checkLockout,insufficient fund to pay fee=================","err",dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		 return false,dcrm.GetRetErr(dcrm.ErrInsufficientDcrmFunds)
	 }
     } else {
	 log.Debug("==========checkLockout,get token balance/coin balance error.=================","coin balance",ret1,"token balance",ret2,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	 return false,errors.New("get token balance/coin balance error.")
     }
 }

    //lockoutto
    // check lockoutto address
    av := cryptocoins.NewAddressValidator(cointype)
    if match := av.IsValidAddress(lockoutto); !match {
	log.Debug("==========checkLockout=================","err",dcrm.GetRetErr(dcrm.ErrInvalidAddrToLO),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrInvalidAddrToLO)
    }

    //bug for lockout to self
    dcrmaddr := pool.currentState.GetDcrmAddr(from,cointype)
    //log.Debug("checkLockout","dcrmaddr",dcrmaddr,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
    //log.Debug("checkLockout","lockoutto",lockoutto,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
    if strings.EqualFold(dcrmaddr,lockoutto) == true {
	return false,dcrm.GetRetErr(dcrm.ErrLOToSelf)
    }

    //from must be fusion account in tx inside.
    if dcrm.ChainDb() != nil {
	hashkey := crypto.Keccak256Hash([]byte(strings.ToLower(from.Hex()+cointype)))
	if tx2,_,_,_ := rawdb.ReadTransaction(dcrm.ChainDb(),hashkey); tx2 != nil {
	    return false,dcrm.GetRetErr(dcrm.ErrFromNotFusionAccount)
	}
    }

    //check choose real account
    _,err = pool.GetRealAccount(tx)
    if err != nil {
	return false,err
    }

    return true,nil
}

func (pool *TxPool) GetRealAccount(tx *types.Transaction) (bool,error) {
    //log.Debug("============GetRealAccount=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))

    if !types.IsDcrmLockOut(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    
    lockoutto := inputs[1]
    value := inputs[2]
    cointype := inputs[3]
    
    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========txPool.GetRealAccount,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    hash := crypto.Keccak256Hash([]byte(tx.Hash().Hex() + ":" + strings.ToLower(cointype))).Hex()
    val,err := dcrm.GetLockoutInfoFromLocalDB(hash)
    if err == nil && val != "" {
	retvas := strings.Split(val,common.Sep10)
	if len(retvas) >= 2 {
	    log.Debug("========GetRealAccount,has already get real account==========","val",val,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return true,nil
	}
    }

    dcrmfrom := pool.currentState.GetDcrmAddr(from,cointype)
    result,err := tx.MarshalJSON()
    if err != nil {
	return false,dcrm.GetRetErr(dcrm.ErrInvalidTx)
    }

    if dcrmfrom == "" {
	dcrmfrom = "xxx"
    }

    msg := tx.Hash().Hex() + common.Sep9 + string(result) + common.Sep9 + from.Hex() + common.Sep9 + dcrmfrom + common.Sep9 + "xxx" + common.Sep9 + "xxx" + common.Sep9 + lockoutto + common.Sep9 + value + common.Sep9 + cointype 

    var errtmp error
    //for i:= 0;i<dcrm.TryTimes;i++ {
    for i:= 0;i<1;i++ {
	retva,err := dcrm.SendReqToGroup(msg,"rpc_get_real_account")
	if err == nil {
	    log.Debug("============txPool.GetRealAccount,get real account success.=================","i",i,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    hash = crypto.Keccak256Hash([]byte(tx.Hash().Hex() + ":" + strings.ToLower(cointype))).Hex()
	    dcrm.WriteLockoutInfoToLocalDB(hash,retva)
	    return true,nil
	}
	
	errtmp = err
	time.Sleep(time.Duration(1000000000))
    }
    
    if errtmp != nil {
	    log.Debug("=============GetRealAccount,get real account fail.==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false, errtmp
    }
    
    return true,nil
}

func (pool *TxPool) GetSigner() types.Signer {
    return pool.signer
}

func (pool *TxPool) ValidateLockout(tx *types.Transaction) (bool,error) {
    //log.Debug("============txPool.validateLockout=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))

    if !types.IsDcrmLockOut(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    
    lockoutto := inputs[1]
    value := inputs[2]
    cointype := inputs[3]
    
    from, err := types.Sender(pool.GetSigner(), tx)
    if err != nil {
	log.Debug("==========txPool.ValidateLockout,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    dcrmfrom := pool.currentState.GetDcrmAddr(from,cointype)
    result,err := tx.MarshalJSON()
    if err != nil {
	return false,dcrm.GetRetErr(dcrm.ErrInvalidTx)
    }

    if dcrmfrom == "" {
	dcrmfrom = "xxx"
    }

    msg := tx.Hash().Hex() + common.Sep9 + string(result) + common.Sep9 + from.Hex() + common.Sep9 + dcrmfrom + common.Sep9 + "xxx" + common.Sep9 + "xxx" + common.Sep9 + lockoutto + common.Sep9 + value + common.Sep9 + cointype 

    var errtmp error
    for i:= 0;i<dcrm.TryTimes;i++ {
	retva,err := dcrm.SendReqToGroup(msg,"rpc_lockout")
	if err == nil {
	    log.Debug("============txPool.ValidateLockout,send tx success.=================","i",i,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    types.SetDcrmValidateData(tx.Hash().Hex(),retva)//bug
	    return true,nil
	}
	
	errtmp = err
	time.Sleep(time.Duration(1000000000))
    }
    
    if errtmp != nil {
	    log.Debug("=============ValidateLockout,send tx fail.==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false, errtmp
    }
    
    return true,nil
}

//lockin
func (pool *TxPool) checkLockin(tx *types.Transaction) (bool,error) {
    if !types.IsDcrmLockIn(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    if len(inputs) == 0 {
	return false,dcrm.GetRetErr(dcrm.ErrTxDataError)
    }

    hashkey := inputs[1]
    value := inputs[2]
    cointype := inputs[3]

    if len(inputs) < 4 || hashkey == "" || value == "" || cointype == "" {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    if cryptocoins.IsCoinSupported(cointype) == false {
	return false, dcrm.GetRetErr(dcrm.ErrCoinTypeNotSupported)
    }

    if strings.EqualFold(cointype,"ETH") == true || strings.EqualFold(cointype,"ERC20GUSD") == true || strings.EqualFold(cointype,"ERC20BNB") == true || strings.EqualFold(cointype,"ERC20MKR") == true || strings.EqualFold(cointype,"ERC20HT") == true || strings.EqualFold(cointype,"ERC20BNT") == true || strings.EqualFold(cointype,"ERC20RMBT") {
	if !isDecimalNumber(value) {
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
	}
    }

    if strings.EqualFold(cointype,"BTC") {
	if !isValidBtcValue(value) {
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
	}

	amount,_ := new(big.Int).SetString(value,10)

	one,_ := new(big.Int).SetString("1",10)
	if amount.Cmp(one) < 0 {
	    //return false,errors.New("value must great than or equal 1 satoshis.")
	    return false,dcrm.GetRetErr(dcrm.ErrParamError)
	}
    }

    hashkeys := []rune(hashkey)
    if  (strings.EqualFold(cointype,"ETH") == true || strings.EqualFold(cointype,"ERC20GUSD") == true || strings.EqualFold(cointype,"ERC20BNB") == true || strings.EqualFold(cointype,"ERC20MKR") == true || strings.EqualFold(cointype,"ERC20HT") == true || strings.EqualFold(cointype,"ERC20BNT") == true || strings.EqualFold(cointype,"ERC20RMBT")) && string(hashkeys[0:2]) != "0x" {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========checkLockin,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    cc := strings.ToUpper(cointype)
    if cryptocoins.IsEVT(cc) {
	    cc = "EVT1"
    }
    if cryptocoins.IsErc20(cc) {
	    cc = "ETH"
    }
    if cryptocoins.IsOmni(cc) {
	    cc = "BTC"
    }
    if cryptocoins.IsBEP2(cc) {
	    log.Debug("========checkLockin========","cc",cc)
	    cc = "BNB"
    }
    //ret := pool.currentState.GetDcrmAddr(from,cointype)
    ret := pool.currentState.GetDcrmAddr(from,cc)
    log.Debug("========checkLockin========","ret",ret)
    if ret == "" {
	return false,dcrm.GetRetErr(dcrm.ErrDcrmAddrNotConfirmed)
    }

    //log.Debug("checkLockin","ret",ret,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
    if av := cryptocoins.NewDcrmAddressValidator(cointype); av.IsValidAddress(ret) == false {
	    return false, dcrm.GetRetErr(dcrm.ErrInvalidDcrmAddr)//errors.New("dcrm address is not the right format.")
     }

    ////////
    tmp := crypto.Keccak256Hash([]byte(strings.ToLower(hashkey+cointype)))
    if txt,_,_,_ := rawdb.ReadTransaction(dcrm.ChainDb(), tmp); txt != nil {
	return false,dcrm.GetRetErr(dcrm.ErrDcrmAddrAlreadyLockIn)
    }

    ///////judge change or lockin for BTC
    if strings.EqualFold(cointype, "BTC") {
       chd := cryptocoins.NewCryptocoinHandler(cointype)
       fromAddr, _, _, _, _,_ := chd.GetTransactionInfo(string(hashkeys))
       //log.Debug("checkLockin","fromAddr",fromAddr,"ret",ret,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
       if strings.EqualFold(fromAddr,ret) {
	       return false,dcrm.GetRetErr(dcrm.ErrNotRealLockIn)
       }
    }

    return true,nil
}

func (pool *TxPool) ValidateLockin2(tx *types.Transaction,retva string) (bool,error) {
    if retva == "" {
	return false,dcrm.GetRetErr(dcrm.ErrHashKeyMiss)
    }

    retvas := strings.Split(retva,common.Sep10)
    if len(retvas) < 3 {
	return false,dcrm.GetRetErr(dcrm.ErrHashKeyMiss)
    }
    hashkey := retvas[0]
    realdcrmfrom := retvas[2]

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    
    value := inputs[2]
    cointype := inputs[3]
    
    //log.Debug("=============txPool.ValidateLockin2.==============","tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"cointype",cointype)
    
    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========txPool.ValidateLockin2,pool,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"cointype",cointype)
	return false,ErrInvalidSender
    }

    result,err := tx.MarshalJSON()
    
    msg := tx.Hash().Hex() + common.Sep9 + string(result) + common.Sep9 + from.Hex() + common.Sep9 + hashkey + common.Sep9 + value + common.Sep9 + cointype + common.Sep9 + inputs[1] + common.Sep9 + realdcrmfrom

    var errtmp error
    for i:=0;i<dcrm.TryTimes;i++ {
	va,err := dcrm.SendReqToGroup(msg,"rpc_lockin")
	if err == nil {
	    log.Debug("=============txPool.ValidateLockin2,return success.==============","real fee",va,"tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"cointype",cointype)
	    types.SetDcrmLockoutFeeData(tx.Hash().Hex(),va)
	    dcrm.WriteLockoutRealFeeToLocalDB(tx.Hash().Hex(),va)

	    go func(hash string) {
		 time.Sleep(time.Duration(150)*time.Second) //1000 == 1s
		 //types.DeleteDcrmLockoutFeeData(hash)
	     }(tx.Hash().Hex())  //TODO
	    return true,nil
	}

	errtmp = err
	if errtmp != nil && strings.EqualFold(errtmp.Error(),"confirming") {
	    break
	}

	time.Sleep(time.Duration(1000000000))
    }

    if errtmp != nil {
	log.Debug("=============txPool.ValidateLockin2,return fail.==============","err",errtmp,"tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"realdcrmfrom",realdcrmfrom,"cointype",cointype)
	return false, errtmp
    }
    
    return true,nil
}

func (pool *TxPool) ValidateLockin(tx *types.Transaction) (bool,error) {
    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    
    hashkey := inputs[1]
    value := inputs[2]
    cointype := inputs[3]
    
    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========ValidateLockin,pool,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"cointype",cointype)
	return false,ErrInvalidSender
    }
    
    log.Debug("==========ValidateLockin=================","tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"cointype",cointype)

    cc := strings.ToUpper(cointype)
    if cryptocoins.IsErc20(cc) {
	    cc = "ETH"
    } else if cryptocoins.IsOmni(cc) {
	    cc = "BTC"
    } else if cryptocoins.IsBEP2(cc) {
	    cc = "BNB"
    } else if cryptocoins.IsEVT(cc) {
	    cc = "EVT1"
    }
    //ret := pool.currentState.GetDcrmAddr(from,cointype)
    ret := pool.currentState.GetDcrmAddr(from,cc)
    result,err := tx.MarshalJSON()

    msg := tx.Hash().Hex() + common.Sep9 + string(result) + common.Sep9 + from.Hex() + common.Sep9 + hashkey + common.Sep9 + value + common.Sep9 + cointype + common.Sep9 + ret + common.Sep9 + "xxx"

    var errtmp error
    for i:=0;i<dcrm.TryTimesForLockin;i++ {
	_,err = dcrm.SendReqToGroup(msg,"rpc_lockin")
	if err == nil {
	    log.Debug("=============ValidateLockin,return success.=============","tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"cointype",cointype)
	    return true,nil
	}

	errtmp = err
	if errtmp != nil && strings.EqualFold(errtmp.Error(),"confirming") {
	    break
	}

	time.Sleep(time.Duration(1000000000))
    }

    if errtmp != nil {
	log.Debug("=============ValidateLockin,return error.=============","err",errtmp,"tx hash",tx.Hash(),"tx data",string(tx.Data()),"hashkey",hashkey,"cointype",cointype)
	return false, errtmp
    }

    return true,nil
}

//confirmaddr
func (pool *TxPool) checkConfirmAddr(tx *types.Transaction) (bool,error) {
    if !types.IsDcrmConfirmAddr(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    if len(inputs) == 0 {
	log.Debug("checkConfirmAddr,tx input data is empty.","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrTxDataError)
    }

    dcrmaddr := inputs[1]
    cointype := inputs[2]

    if len(inputs) < 3 || dcrmaddr == "" || cointype == "" {
	log.Debug("checkConfirmAddr,tx input data param error.","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    if cryptocoins.IsCoinSupported(cointype) == false {
	return false, dcrm.GetRetErr(dcrm.ErrCoinTypeNotSupported)
    }

    //if strings.EqualFold(cointype,"BTC") == true && dcrm.ValidateAddress(1,dcrmaddr) == false {
    if av := cryptocoins.NewDcrmAddressValidator(cointype); av.IsValidAddress(dcrmaddr) == false {
	log.Debug("checkConfirmAddr,dcrm addr is not the right format.","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrInvalidDcrmAddr)
    }

    if strings.EqualFold(cointype, "ALL") == true {
	    if reg, _ := regexp.Compile("^(0x)?[0-9a-f]{130}$"); reg.Match([]byte(dcrmaddr)) == false {
		    log.Debug("checkConfirmAddr,dcrm pubkey is not the right format","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    return false,dcrm.GetRetErr(dcrm.ErrInvalidDcrmPubkey)
	    }
    }

    if strings.EqualFold(cointype, "ALL") == true {
	    if reg, _ := regexp.Compile("^(0x)?04[0-9a-f]{128}"); reg.Match([]byte(dcrmaddr)) == false {
		    log.Debug("checkConfirmAddr,dcrm pubkey is not the right format","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		    return false,dcrm.GetRetErr(dcrm.ErrInvalidDcrmPubkey)
	    }
    }
    
    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========checkConfirmAddr,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    ret,err := pool.currentState.IsExsitDcrmAddress(from,cointype,dcrmaddr)
    if err == nil && ret == true {
	log.Debug("checkConfirmAddr,the account has confirmed dcrm address.","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,dcrm.GetRetErr(dcrm.ErrDcrmAddrAlreadyConfirmed)
    }

    return true,nil
}

func (pool *TxPool) validateConfirmAddr(tx *types.Transaction) (bool,error) {
    if !types.IsDcrmConfirmAddr(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")

    dcrmaddr := inputs[1]
    cointype := inputs[2]
    
    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========validateConfirmAddr,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    result,err := tx.MarshalJSON()

    msg := tx.Hash().Hex() + common.Sep9 + string(result) + common.Sep9 + from.Hex() + common.Sep9 + dcrmaddr + common.Sep9 + "xxxx" + common.Sep9 + cointype 
    _,err = dcrm.SendReqToGroup(msg,"rpc_confirm_dcrmaddr")
    if err != nil {
	    return false, err
    }

    return true,nil 
}

///////
func (pool *TxPool) validateDcrm(tx *types.Transaction) (bool,error) {
    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")

    if len(inputs) != 0 && inputs[0] == "DCRMCONFIRMADDR" {
	_,err := pool.checkConfirmAddr(tx)
	if err != nil {
	    log.Debug("validateDcrm,check confirm addr fail.","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}

	/*_,err = pool.validateConfirmAddr(tx)
	if err != nil {
	    log.Debug("validate confirm addr fail.")
	    return false,err
	}*/
    }
	
    if len(inputs) != 0 && inputs[0] == "LOCKIN" {
	_,err := pool.checkLockin(tx)
	if err != nil {
	    log.Debug("validateDcrm,check lockin fail.","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}
    }
    
    if len(inputs) != 0 && inputs[0] == "LOCKOUT" {
	
	e := pool.Los.PushBack(tx)

	_,err := pool.checkLockout(tx)
	if err != nil {
	    log.Debug("validateDcrm.checkLockout","err",err.Error(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    pool.Los.Remove(e)
	    return false,err
	}

	/*_,err = pool.validateLockout(tx)
	if err != nil {
	    log.Debug("validateDcrm.validateLockout","err",err.Error())
	    return false,err
	}*/
    }
   
    if len(inputs) != 0 && inputs[0] == "TRANSACTION" {
	_,err := pool.checkTransaction(tx)
	if err != nil {
	    log.Debug("validateDcrm,check transaction fail.","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}

	_,err = pool.validateTransaction(tx)
	if err != nil {
	    log.Debug("validateDcrm,validate transaction fail.","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}
    }

    return true,nil
}

func GetMarketTradeRuleParam(trade string) (decimal.Decimal,decimal.Decimal,decimal.Decimal,error) {
    /*switch trade {
    case "ETH/BTC":
	q,_ := decimal.NewFromString("0.001")
	p,_ := decimal.NewFromString("0.000001")
	amount,_ := decimal.NewFromString("0.0001")
	return q,p,amount,nil
    case "FSN/BTC":
	q,_ := decimal.NewFromString("1")
	p,_ := decimal.NewFromString("0.000001")
	amount,_ := decimal.NewFromString("0.0001")
	return q,p,amount,nil
    case "FSN/ETH":
	q,_ := decimal.NewFromString("1")
	p,_ := decimal.NewFromString("0.0001")
	amount,_ := decimal.NewFromString("0.01")
	return q,p,amount,nil
    default:
	return decimal.Zero,decimal.Zero,decimal.Zero,errors.New("get market trade rule param fail.")
    }*////tmp code

    return decimal.Zero,decimal.Zero,decimal.Zero,errors.New("get market trade rule param fail.")
}

/*func IsValideDealForOrder(side string,price decimal.Decimal,quantity decimal.Decimal,trade string) (bool,error) {
    ua,ub,err := GetUnit(trade)
    if err != nil {
	return false,err
    }

    if side == "Buy" {
	tmp1 := price.Mul(quantity)
	tmp1 = tmp1.Mul(ub)
	if !isDecimalNumber(tmp1.String()) {
	    return false,errors.New("balance must be integer.")
	}

	tmp1 = quantity.Mul(ua)
	fee := tmp1.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
	tmp1 = tmp1.Sub(fee)
	if !isDecimalNumber(tmp1.String()) {
	    return false,errors.New("balance must be integer.")
	}
    }

    if side == "Sell" {
	tmp1 := quantity.Mul(ua)
	if !isDecimalNumber(tmp1.String()) {
	    return false,errors.New("balance must be integer.")
	}
	tmp1 = price.Mul(quantity)
	tmp1 = tmp1.Mul(ub)
	fee := tmp1.Mul(dcrm.EXCHANGE_DEFAULT_FEE)  //TODO
	tmp1 = tmp1.Sub(fee)
	if !isDecimalNumber(tmp1.String()) {
	    return false,errors.New("balance must be integer.")
	}
    }

    return true,nil
}
*/

func (pool *TxPool) CalcBalanceOf(trade string,from common.Address) (decimal.Decimal,decimal.Decimal,decimal.Decimal,error) {
    /*fusion := decimal.Zero
    A := decimal.Zero
    B := decimal.Zero
    
    f := pool.currentState.GetBalance(from,"FSN")
    if f == nil {
	f,_ = new(big.Int).SetString("0",10)
    }
    va := fmt.Sprintf("%v",f)
    fusion,err := decimal.NewFromString(va)
    if err != nil {
	return decimal.Zero,decimal.Zero,decimal.Zero,err
    }
    
    if trade == "ETH/BTC" {
	 e := pool.currentState.GetBalance(from,"ETH")

	if e == nil {
	    e,_ = new(big.Int).SetString("0",10)
	}

	va = fmt.Sprintf("%v",e)
	A,err = decimal.NewFromString(va)
	if err != nil {
	    return decimal.Zero,decimal.Zero,decimal.Zero,err
	}
	 
	t := pool.currentState.GetBalance(from,"BTC")

	if t == nil {
	    t,_ = new(big.Int).SetString("0",10)
	}

	va = fmt.Sprintf("%v",t)
	B,err = decimal.NewFromString(va)
	if err != nil {
	    return decimal.Zero,decimal.Zero,decimal.Zero,err
	}

	return fusion,A,B,nil
    }

    if trade == "FSN/BTC" {
	A = fusion
	 
	t := pool.currentState.GetBalance(from,"BTC")

	if t == nil {
	    t,_ = new(big.Int).SetString("0",10)
	}

	va = fmt.Sprintf("%v",t)
	B,err = decimal.NewFromString(va)
	if err != nil {
	    return decimal.Zero,decimal.Zero,decimal.Zero,err
	}

	return fusion,A,B,nil
    }

    if trade == "FSN/ETH" {
	A = fusion 
	 
	t := pool.currentState.GetBalance(from,"ETH")

	if t == nil {
	    t,_ = new(big.Int).SetString("0",10)
	}

	va = fmt.Sprintf("%v",t)
	B,err = decimal.NewFromString(va)
	if err != nil {
	    return decimal.Zero,decimal.Zero,decimal.Zero,err
	}

	return fusion,A,B,nil
    }*////tmp code

    return decimal.Zero,decimal.Zero,decimal.Zero,dcrm.GetRetErr(dcrm.ErrCalcOrderBalance)
}

func (pool *TxPool) checkorder(tx *types.Transaction) (bool,error) {

    log.Debug("============checkorder=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))

    if !types.IsXvcCreateOrderTx(tx) {
	return true,nil 
    }

    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    if len(inputs) < 9 {
	return false,dcrm.GetRetErr(dcrm.ErrTxDataError)
    }

    fusionaddr := inputs[1]
    trade := inputs[2]
    ordertype := inputs[3]
    side := inputs[4]
    price := inputs[5]
    quantity := inputs[6]
    rule := inputs[7]

    if fusionaddr == "" || trade == "" || ordertype == "" || side == "" || price == "" || quantity == "" || rule == "" {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    if !types.IsValideTrade(trade) || (strings.EqualFold(ordertype,"LimitOrder") == false && strings.EqualFold(ordertype,"MarketOrder") == false) || (strings.EqualFold(side,"Buy") == false && strings.EqualFold(side,"Sell") == false) || (strings.EqualFold(rule,"GTE") == false && strings.EqualFold(rule,"IOC") == false) {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    if !dcrm.IsValidDcrmAddr(fusionaddr,"ETH") {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    p,err := decimal.NewFromString(price)
    if err != nil {
	return false,err
    }
    q,err := decimal.NewFromString(quantity)
    if err != nil {
	return false,err
    }

    if p.LessThanOrEqual(decimal.Zero) == true {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }
    if q.LessThanOrEqual(decimal.Zero) == true {
	return false,dcrm.GetRetErr(dcrm.ErrParamError)
    }

    return true,nil //TODO

    //////////////////////////////////////

    small_q,small_p,small_amount,err := GetMarketTradeRuleParam(trade)
    if err != nil {
	return false,err
    }

    log.Debug("================checkorder================","smallest quantity",small_q,"smallest price",small_p,"smallest amount",small_amount)

    if q.LessThan(small_q) == true {
	return false, fmt.Errorf("quantity must great than %s",small_q.String())
    }

    if p.LessThan(small_p) == true {
	return false, fmt.Errorf("price must great than %s",small_p.String())
    }

    area := p.Mul(q)
    if area.LessThan(small_amount) == true {
	return false, fmt.Errorf("price*quantity must great than %s",small_amount.String())
    }

    //////bug
    //_,err = IsValideDealForOrder(side,p,q,trade)
    //if err != nil {
//	return false,err
  //  }
    //////

    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	log.Debug("==========checkorder,fail:from is nil.=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,ErrInvalidSender
    }

    ba_fusion := decimal.Zero
    ba_A := decimal.Zero
    ba_B := decimal.Zero
    ba_fusion,ba_A,ba_B,err = pool.CalcBalanceOf(trade,from)
    if err != nil {
	log.Debug("checkorder.CalcBalanceOf,","err",err.Error(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,err
    }
    
    _,err = dcrm.IsOrderBalanceOk(fusionaddr,trade,side,p,q,ba_fusion,ba_A,ba_B)
    if err != nil {
	log.Debug("checkorder.IsOrderBalanceOk","err",err.Error(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,err
    }

    return true,nil
}

func (pool *TxPool) validateOrder(tx *types.Transaction) (bool,error) {
    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
    inputs := strings.Split(realtxdata,":")
    //log.Debug("==========validateOrder===================","tx.Hash",tx.Hash(),"tx.Data()",string(tx.Data()),"inputs[0]",inputs[0])

    if len(inputs) != 0 && inputs[0] == "ORDER" {
	_,err := pool.checkorder(tx)
	if err != nil {
	    log.Debug("validateOrder,check order fail.","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}
	
	result,err := tx.MarshalJSON()
	if err != nil {
	    return false,err
	}

	//txhash:tx:fusionaddr:trade:ordertype:side:price:quanity:rule:time
	msg := tx.Hash().Hex() + common.Sep9 + string(result) + common.Sep9 + inputs[1] + common.Sep9 + inputs[2] + common.Sep9 + inputs[3] + common.Sep9 + inputs[4] + common.Sep9 + inputs[5] + common.Sep9 + inputs[6] + common.Sep9 + inputs[7] + common.Sep9 + inputs[8] 
	_,err = dcrm.SendReqToGroup(msg,"rpc_order_create")
	if err != nil {
	    log.Debug("==========validateOrder===================","tx.Hash",tx.Hash(),"err",err.Error(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,err
	}

	log.Debug("==========validateOrder,success===================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	return false,errors.New("success")
    }

    if len(inputs) != 0 && inputs[0] == "CANCELORDER" {
	if !types.IsXvcCancelOrderTx(tx) {
	    return true,nil
	}

	fusionaddr := inputs[1]
	id := inputs[2]
	if fusionaddr == "" || id == "" {
	    return false,nil
	}

	cancelordercb(id)
	return false,errors.New("success")
    }
    
    return true,nil
}

func (pool *TxPool) findlo(in *types.Transaction) bool {
    if in == nil {
	return false
    }

    nonce := in.Nonce()
    if nonce < 0 {
	return false
    }

    if pool.Los == nil || pool.Los.Len() == 0 {
	return false
    }

    iter := pool.Los.Front()
    for iter != nil {
	tx := iter.Value.(*types.Transaction)
	if tx == nil || !types.IsDcrmLockOut(tx) {
	    continue
	}

	//////if user send same tx to diffrent dcrm node
	if types.IsDcrmLockOut(in) && in.Hash().Hex() == tx.Hash().Hex() {
	    return true
	}
	///////

	if tx.Nonce() == nonce {
	    return true
	}

	iter = iter.Next()
    }

    return false
}

func (pool *TxPool) removelo(tx *types.Transaction) {
    if tx == nil {
	return
    }
    
    //log.Debug("=============txpool.removelo================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
    if pool.Los == nil || pool.Los.Len() == 0 {
	return
    }

    iter := pool.Los.Front()
    for iter != nil {
	tx2 := iter.Value.(*types.Transaction)
	if tx2 == nil || !types.IsDcrmLockOut(tx2) {
	    continue
	}

	if tx2 == tx {
	    pool.Los.Remove(iter)
	    return
	}

	iter = iter.Next()
    }
}

func (pool *TxPool) InsToOB(tx *types.Transaction) (bool,error) {

    if tx != nil && types.IsXvcCreateOrderTx(tx) {
	    _,err := pool.checkorder(tx)
	    if err != nil {
		log.Debug("pool.add,check order fail.","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return false,err
	    }
	    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
	    inputs := strings.Split(realtxdata,":")
	   
	    ////
	    e,_ := decimal.NewFromString(dcrm.UT)
	    p,_ := decimal.NewFromString(inputs[5]) 
	    q,_ := decimal.NewFromString(inputs[6])
	    p = p.Mul(e)
	    q = q.Mul(e)
	    pp := p.String()
	    qq := q.String()
	    ////

	    msg := tx.Hash().Hex() + common.Sep9 + "xxxx" + common.Sep9 + inputs[1] + common.Sep9 + inputs[2] + common.Sep9 + inputs[3] + common.Sep9 + inputs[4] + common.Sep9 + pp + common.Sep9 + qq + common.Sep9 + inputs[7] + common.Sep9 + inputs[8]
	    if dcrm.IsInXvcGroup() {
		dcrm.InsToOB(msg)
	    }
	    
	    return true,nil
	}

	return false,errors.New("insert fail.")
}

func (pool *TxPool) FeedTx(tx *types.Transaction) {
    if tx != nil {
	go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})
    }
}

func (pool *TxPool) AddNewTrade(tx *types.Transaction,local bool) {
    if tx == nil {
	return
    }

    if vm.AddNewTradeCb != nil {
	m := strings.Split(string(tx.Data()),":")
	vm.AddNewTradeCb(m[1],local)
	return 
    }

    return
}

// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *TxPool) add(tx *types.Transaction, local bool) (bool, error) {
	//for mangodb
	//if types.IsXvcCreateOrderTx(tx) {
	//	log.Debug("==== pool.add() ====", "callback", "mongodb.UpdateOrderAddCache")
	//	var txCopy types.Transaction = *tx
	//	go mongodb.UpdateOrderAddCache(&txCopy)
	//}

	log.Debug("=================================!!!!!!!!!!!!!!!!!! pool.add !!!!!!!!!!!!!!!!!=============================","tx hash",tx.Hash().Hex(),"tx nonce",tx.Nonce(),"tx  data",string(tx.Data()))

	// If the transaction is already known, discard it
	hash := tx.Hash()
	if pool.all.Get(hash) != nil {
	    log.Debug("===========pool.add,fail: already known transaction============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		log.Trace("Discarding already known transaction", "hash", hash)
		return false, fmt.Errorf("known transaction: %x", hash)
	}

	if pool.findlo(tx) {
	    return false,dcrm.GetRetErr(dcrm.ErrAlreadyKnownLOTx)
	}

	if ok,verr := pool.validateDcrm(tx);ok == false || verr != nil {
	    log.Debug("================pool.add,validate dcrm fail.==================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	    return false,verr
	}

	///////////////////////////////////////////////////////
	if types.IsXvcCreateOrderTx(tx) {
	    go pool.InsToOB(tx)
	    go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})
	    return true,nil
	}
	if types.IsXvcCancelOrderTx(tx) {
	    realtxdata,_ := types.GetRealTxData(string(tx.Data()))
	    inputs := strings.Split(realtxdata,":")
	    fusionaddr := inputs[1]
	    id := inputs[2]
	    if fusionaddr == "" || id == "" {
		return false,errors.New("param error.")
	    }

	    cancelordercb(id)
	    go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})
	}
	////////
	if types.IsAddNewTradeTx(tx) {
	    pool.AddNewTrade(tx,false)
	    go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})
	    return true,nil
	}
	////////
	////////////////////////////////////////////////////////

	//if ok,verr := pool.validateOrder(tx);ok == false || verr != nil {
	//    return false,verr
	//}

	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx, local); err != nil {
	    log.Debug("==============================!!!!!!!!!!!! pool.add,fail: invalid transaction !!!!!!!!!!!!===========================","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		log.Trace("Discarding invalid transaction", "hash", hash, "err", err)
		invalidTxCounter.Inc(1)
		
		if types.IsDcrmLockOut(tx) {
		    pool.removelo(tx)
		    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
		}

		return false, err
	}
	// If the transaction pool is full, discard underpriced transactions
	if uint64(pool.all.Count()) >= pool.config.GlobalSlots+pool.config.GlobalQueue {
		// If the new transaction is underpriced, don't accept it
		if !local && pool.priced.Underpriced(tx, pool.locals) {
			log.Debug("===========pool.add,fail: underpriced transaction============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
			log.Trace("Discarding underpriced transaction", "hash", hash, "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			if types.IsDcrmLockOut(tx) {
			    pool.removelo(tx)
			    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
			}

			return false, ErrUnderpriced
		}
		// New transaction is better than our worse ones, make room for it
		drop := pool.priced.Discard(pool.all.Count()-int(pool.config.GlobalSlots+pool.config.GlobalQueue-1), pool.locals)
		for _, tx := range drop {
		    //log.Debug("===========pool.add,fail: freshly underpriced transaction============")
			log.Trace("Discarding freshly underpriced transaction", "hash", tx.Hash(), "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			pool.removeTx(tx.Hash(), false)
		}
	}

	/////////////////TODO
	if types.IsXvcTx(tx) {
	    go updateobcb(string(tx.Data()))
	}
	//if types.IsXvcTx(tx) {
	//	log.Debug("==== pool.add() ====", "callback", "mongodb.UpdateOrderFromShare")
	//	go mongodb.UpdateOrderFromShare(tx)
	//}
	//////////////////

	// If the transaction is replacing an already pending one, do directly
	from, _ := types.Sender(pool.signer, tx) // already validated
	if list := pool.pending[from]; list != nil && list.Overlaps(tx) && !types.IsDcrmLockOut(tx) {
		// Nonce already pending, check if required price bump is met
		inserted, old := list.Add(tx, pool.config.PriceBump)
		if !inserted {
		    log.Debug("===========pool.add,fail: ErrReplaceUnderpriced============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
			pendingDiscardCounter.Inc(1)
			if types.IsDcrmLockOut(tx) {
			    pool.removelo(tx)
			    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
			}
			return false, ErrReplaceUnderpriced
		}
		// New transaction is better, replace old one
		if old != nil {
			//log.Debug("===========pool.add,New transaction is better, replace old one============")
			pool.all.Remove(old.Hash())
			pool.priced.Removed()
			pendingReplaceCounter.Inc(1)
			if types.IsDcrmLockOut(old) {
			    pool.removelo(old)
			    //types.DeleteDcrmLockoutFeeData(old.Hash().Hex())
			}
		}
		pool.all.Add(tx)
		pool.priced.Put(tx)
		pool.journalTx(from, tx)

		log.Trace("Pooled new executable transaction", "hash", hash, "from", from, "to", tx.To())

		log.Debug("===========pool.add,if list := pool.pending[from]; list != nil && list.Overlaps(tx)============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		// We've directly injected a replacement transaction, notify subsystems
		go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})

		return old != nil, nil
	}
	log.Debug("===========pool.add,insert into queue.============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	// New transaction isn't replacing a pending one, push into queue
	replace, err := pool.enqueueTx(hash, tx)
	if err != nil {
		if types.IsDcrmLockOut(tx) {
		    pool.removelo(tx)
		    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
		}
		log.Debug("===========pool.add fail.============","err",err,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		return false, err
	}
	// Mark local addresses and journal local transactions
	if local {
		if !pool.locals.contains(from) {
			log.Info("Setting new local account", "address", from)
			pool.locals.add(from)
		}
	}
	pool.journalTx(from, tx)

	log.Trace("Pooled new future transaction", "hash", hash, "from", from, "to", tx.To())
	log.Debug("===========pool.add success.============","tx hash",tx.Hash(),"tx data",string(tx.Data()))

	/////
	realtxdata,_ := types.GetRealTxData(string(tx.Data()))
	inputs := strings.Split(realtxdata,":")
	if len(inputs) != 0 && inputs[0] == "DCRMCONFIRMADDR" {
	    _,err = pool.validateConfirmAddr(tx)
	    if err != nil {
		log.Debug("validate confirm addr fail.","err",err.Error(),"tx hash",tx.Hash(),"tx data",string(tx.Data()))
		pool.all.Remove(tx.Hash())
		pool.priced.Removed()
		return false,err
	    }
	}
	/*if len(inputs) != 0 && inputs[0] == "LOCKOUT" {
	    _,err = pool.validateLockout(tx)
	    if err != nil {
		log.Debug("validateDcrm.validateLockout","err",err.Error(),"tx hash",tx.Hash(),"tx data",string(tx.Data())) //TODO for example: check utxo,but report "connect refused"
		pool.removeTx(tx.Hash(),true)
		return false,err
	    }
	}*/
	
	//log.Debug("func pool.add finish.","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	/////
	return replace, nil
}

func (pool *TxPool) GetDcrmTxRealNonce(from string) (uint64,error) {

    if common.HexToAddress(from) == (common.Address{}) {
	    return 0,errors.New("get nonce error.")
    }
  
    log.Debug("==============GetDcrmTxRealNonce,","txpool.currentState nonce",pool.currentState.GetNonce(common.HexToAddress(from)),"txpool.pendingState nonce",pool.pendingState.GetNonce(common.HexToAddress(from)),"","=================")

    var queue uint64
    if pool.queue[common.HexToAddress(from)] == nil {
	queue = 0
    } else {
	queue = uint64(pool.queue[common.HexToAddress(from)].Len())
	list := pool.queue[common.HexToAddress(from)] 
	if list != nil {
		txs := list.Flatten()
		for _,tx := range txs {
		    log.Info("===============GetDcrmTxRealNonce====================","tx in queue nonce",tx.Nonce(),"tx hash",tx.Hash())
		}
	}
    }
    log.Debug("==============GetDcrmTxRealNonce,","counts in queue",queue,"","=================")

    return queue,nil
}

// enqueueTx inserts a new transaction into the non-executable transaction queue.
//
// Note, this method assumes the pool lock is held!
func (pool *TxPool) enqueueTx(hash common.Hash, tx *types.Transaction) (bool, error) {
	// Try to insert the transaction into the future queue
	from, _ := types.Sender(pool.signer, tx) // already validated
	if pool.queue[from] == nil {
		pool.queue[from] = newTxList(false)
	}
	inserted, old := pool.queue[from].Add(tx, pool.config.PriceBump)
	if !inserted {
	log.Debug("==============enqueueTx,ErrReplaceUnderpriced=========","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		// An older transaction was better, discard this
		queuedDiscardCounter.Inc(1)
		return false, ErrReplaceUnderpriced
	}
	// Discard any previous transaction and mark this
	if old != nil {
		pool.all.Remove(old.Hash())
		pool.priced.Removed()
		queuedReplaceCounter.Inc(1)
		if types.IsDcrmLockOut(old) {
		    pool.removelo(old)
		    //types.DeleteDcrmLockoutFeeData(old.Hash().Hex())
		}
	}
	if pool.all.Get(hash) == nil {
		pool.all.Add(tx)
		pool.priced.Put(tx)
	}
	return old != nil, nil
}

// journalTx adds the specified transaction to the local disk journal if it is
// deemed to have been sent from a local account.
func (pool *TxPool) journalTx(from common.Address, tx *types.Transaction) {
	// Only journal if it's enabled and the transaction is local
	if pool.journal == nil || !pool.locals.contains(from) {
		return
	}
	if err := pool.journal.insert(tx); err != nil {
		log.Warn("Failed to journal local transaction", "err", err)
	}
}

// promoteTx adds a transaction to the pending (processable) list of transactions
// and returns whether it was inserted or an older was better.
//
// Note, this method assumes the pool lock is held!
func (pool *TxPool) promoteTx(addr common.Address, hash common.Hash, tx *types.Transaction) bool {
	//log.Debug("==================promoteTx=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
	// Try to insert the transaction into the pending queue
	if pool.pending[addr] == nil {
		//log.Debug("==================promoteTx,pool.pending[addr] == nil=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		pool.pending[addr] = newTxList(true)
	}
	list := pool.pending[addr]

	inserted, old := list.Add(tx, pool.config.PriceBump)
	if !inserted {
		//log.Debug("==================promoteTx,An older transaction was better, discard this=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		// An older transaction was better, discard this
		pool.all.Remove(hash)
		pool.priced.Removed()

		pendingDiscardCounter.Inc(1)
		if types.IsDcrmLockOut(tx) {
		    pool.removelo(tx)
		    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
		}
		return false
	}
	// Otherwise discard any previous transaction and mark this
	if old != nil {
		//log.Debug("==================promoteTx,discard any previous transaction and mark this=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		pool.all.Remove(old.Hash())
		pool.priced.Removed()

		pendingReplaceCounter.Inc(1)
		if types.IsDcrmLockOut(old) {
		    pool.removelo(old)
		    //types.DeleteDcrmLockoutFeeData(old.Hash().Hex())
		}
	}
	// Failsafe to work around direct pending inserts (tests)
	if pool.all.Get(hash) == nil {
		//log.Debug("==================promoteTx,Failsafe to work around direct pending inserts (tests)=================","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		pool.all.Add(tx)
		pool.priced.Put(tx)
	}
	// Set the potentially new pending nonce and notify any subsystems of the new tx
	//log.Debug("==================promoteTx,Set the potentially new pending nonce and notify any subsystems of the new tx=================","current nonce",tx.Nonce(),"addr",addr,"tx hash",tx.Hash(),"tx data",string(tx.Data()))
	pool.beats[addr] = time.Now()
	pool.pendingState.SetNonce(addr, tx.Nonce()+1)

	return true
}

// AddLocal enqueues a single transaction into the pool if it is valid, marking
// the sender as a local one in the mean time, ensuring it goes around the local
// pricing constraints.
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	return pool.addTx(tx, !pool.config.NoLocals)
}

// AddRemote enqueues a single transaction into the pool if it is valid. If the
// sender is not among the locally tracked ones, full pricing constraints will
// apply.
func (pool *TxPool) AddRemote(tx *types.Transaction) error {
	return pool.addTx(tx, false)
}

// AddLocals enqueues a batch of transactions into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
func (pool *TxPool) AddLocals(txs []*types.Transaction) []error {
	return pool.addTxs(txs, !pool.config.NoLocals)
}

// AddRemotes enqueues a batch of transactions into the pool if they are valid.
// If the senders are not among the locally tracked ones, full pricing constraints
// will apply.
func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	return pool.addTxs(txs, false)
}

// addTx enqueues a single transaction into the pool if it is valid.
func (pool *TxPool) addTx(tx *types.Transaction, local bool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Try to inject the transaction and update any state
	replace, err := pool.add(tx, local)
	if err != nil {
		return err
	}
	// If we added a new transaction, run promotion checks and return
	if !replace {
		from, _ := types.Sender(pool.signer, tx) // already validated
		pool.promoteExecutables([]common.Address{from})
	}
	return nil
}

// addTxs attempts to queue a batch of transactions if they are valid.
func (pool *TxPool) addTxs(txs []*types.Transaction, local bool) []error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	return pool.addTxsLocked(txs, local)
}

// addTxsLocked attempts to queue a batch of transactions if they are valid,
// whilst assuming the transaction pool lock is already held.
func (pool *TxPool) addTxsLocked(txs []*types.Transaction, local bool) []error {
	// Add the batch of transaction, tracking the accepted ones
	dirty := make(map[common.Address]struct{})
	errs := make([]error, len(txs))

	for i, tx := range txs {
		var replace bool
		//if replace, errs[i] = pool.add(tx, local); errs[i] == nil && !replace {
		replace, errs[i] = pool.add(tx, local)
		//log.Info("===========pool.addTxsLocked=================","replace",replace,"err",errs[i],"tx nonce",tx.Nonce(),"tx hash",tx.Hash())
		if errs[i] == nil && !replace {
			from, _ := types.Sender(pool.signer, tx) // already validated
			dirty[from] = struct{}{}
		}
	}
	// Only reprocess the internal state if something was actually added
	if len(dirty) > 0 {
		addrs := make([]common.Address, 0, len(dirty))
		for addr := range dirty {
			addrs = append(addrs, addr)
		}
		pool.promoteExecutables(addrs)
	}
	return errs
}

// Status returns the status (unknown/pending/queued) of a batch of transactions
// identified by their hashes.
func (pool *TxPool) Status(hashes []common.Hash) []TxStatus {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	status := make([]TxStatus, len(hashes))
	for i, hash := range hashes {
		if tx := pool.all.Get(hash); tx != nil {
			from, _ := types.Sender(pool.signer, tx) // already validated
			if pool.pending[from] != nil && pool.pending[from].txs.items[tx.Nonce()] != nil {
				status[i] = TxStatusPending
			} else {
				status[i] = TxStatusQueued
			}
		}
	}
	return status
}

// Get returns a transaction if it is contained in the pool
// and nil otherwise.
func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	return pool.all.Get(hash)
}

// removeTx removes a single transaction from the queue, moving all subsequent
// transactions back to the future queue.
func (pool *TxPool) removeTx(hash common.Hash, outofbound bool) {
	// Fetch the transaction we wish to delete
	tx := pool.all.Get(hash)
	if tx == nil {
		return
	}
	addr, _ := types.Sender(pool.signer, tx) // already validated during insertion

	if types.IsDcrmLockOut(tx) {
	    pool.removelo(tx)
	    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
	}
	// Remove it from the list of known transactions
	pool.all.Remove(hash)
	if outofbound {
		pool.priced.Removed()
	}
	// Remove the transaction from the pending lists and reset the account nonce
	if pending := pool.pending[addr]; pending != nil {
		if removed, invalids := pending.Remove(pool,tx); removed { 
			// If no more pending transactions are left, remove the list
			if pending.Empty() {
				delete(pool.pending, addr)
				delete(pool.beats, addr)
			}
			// Postpone any invalidated transactions
			for _, tx := range invalids {
				pool.enqueueTx(tx.Hash(), tx)
			}
			// Update the account nonce if needed
			if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
				pool.pendingState.SetNonce(addr, nonce)
			}
			return
		}
	}
	// Transaction is in the future queue
	if future := pool.queue[addr]; future != nil {
	    //log.Debug("===========txpool.removeTx==============","tx hash",tx.Hash(),"tx data",string(tx.Data()))
		future.Remove(pool,tx)
		if future.Empty() {
			delete(pool.queue, addr)
		}
	}
}

// promoteExecutables moves transactions that have become processable from the
// future queue to the set of pending transactions. During this process, all
// invalidated transactions (low nonce, low balance) are deleted.
func (pool *TxPool) promoteExecutables(accounts []common.Address) {
	// Track the promoted transactions to broadcast them at once
	var promoted []*types.Transaction

	// Gather all the accounts potentially needing updates
	if accounts == nil {
		accounts = make([]common.Address, 0, len(pool.queue))
		for addr := range pool.queue {
			accounts = append(accounts, addr)
		}
	}
	// Iterate over all accounts and promote any executable transactions
	for _, addr := range accounts {
		list := pool.queue[addr]
		if list == nil {
			continue // Just in case someone calls with a non existing account
		}
		// Drop all transactions that are deemed too old (low nonce)
		for _, tx := range list.Forward(pool.currentState.GetNonce(addr)) {
			hash := tx.Hash()
			log.Trace("Removed old queued transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			if types.IsDcrmLockOut(tx) {
			    pool.removelo(tx)
			    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
			}
		}
		// Drop all transactions that are too costly (low balance or out of gas)
		drops, _ := list.Filter(pool,pool.currentState.GetBalance(addr,"FSN"), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable queued transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			queuedNofundsCounter.Inc(1)
			if types.IsDcrmLockOut(tx) {
			    pool.removelo(tx)
			    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
			}
		}
		// Gather all executable transactions and promote them
		for _, tx := range list.Ready(pool,pool.pendingState.GetNonce(addr)) { 
			hash := tx.Hash()
			if pool.promoteTx(addr, hash, tx) {
				log.Trace("Promoting queued transaction", "hash", hash)
				promoted = append(promoted, tx)
			}
		}
		// Drop all transactions over the allowed limit
		if !pool.locals.contains(addr) {
			for _, tx := range list.Cap(int(pool.config.AccountQueue)) {
				hash := tx.Hash()
				pool.all.Remove(hash)
				pool.priced.Removed()
				queuedRateLimitCounter.Inc(1)
				log.Trace("Removed cap-exceeding queued transaction", "hash", hash)
				if types.IsDcrmLockOut(tx) {
				    pool.removelo(tx)
				    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
				}
			}
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.queue, addr)
		}
	}
	// Notify subsystem for new promoted transactions.
	if len(promoted) > 0 {
		go pool.txFeed.Send(NewTxsEvent{promoted})
	}
	// If the pending limit is overflown, start equalizing allowances
	pending := uint64(0)
	for _, list := range pool.pending {
		pending += uint64(list.Len())
	}
	if pending > pool.config.GlobalSlots {
		pendingBeforeCap := pending
		// Assemble a spam order to penalize large transactors first
		spammers := prque.New(nil)
		for addr, list := range pool.pending {
			// Only evict transactions from high rollers
			if !pool.locals.contains(addr) && uint64(list.Len()) > pool.config.AccountSlots {
				spammers.Push(addr, int64(list.Len()))
			}
		}
		// Gradually drop transactions from offenders
		offenders := []common.Address{}
		for pending > pool.config.GlobalSlots && !spammers.Empty() {
			// Retrieve the next offender if not local address
			offender, _ := spammers.Pop()
			offenders = append(offenders, offender.(common.Address))

			// Equalize balances until all the same or below threshold
			if len(offenders) > 1 {
				// Calculate the equalization threshold for all current offenders
				threshold := pool.pending[offender.(common.Address)].Len()

				// Iteratively reduce all offenders until below limit or threshold reached
				for pending > pool.config.GlobalSlots && pool.pending[offenders[len(offenders)-2]].Len() > threshold {
					for i := 0; i < len(offenders)-1; i++ {
						list := pool.pending[offenders[i]]
						for _, tx := range list.Cap(list.Len() - 1) {
							// Drop the transaction from the global pools too
							hash := tx.Hash()
							pool.all.Remove(hash)
							pool.priced.Removed()
							if types.IsDcrmLockOut(tx) {
							    pool.removelo(tx)
							    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
							}

							// Update the account nonce to the dropped transaction
							if nonce := tx.Nonce(); pool.pendingState.GetNonce(offenders[i]) > nonce {
								pool.pendingState.SetNonce(offenders[i], nonce)
							}
							log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
						}
						pending--
					}
				}
			}
		}
		// If still above threshold, reduce to limit or min allowance
		if pending > pool.config.GlobalSlots && len(offenders) > 0 {
			for pending > pool.config.GlobalSlots && uint64(pool.pending[offenders[len(offenders)-1]].Len()) > pool.config.AccountSlots {
				for _, addr := range offenders {
					list := pool.pending[addr]
					for _, tx := range list.Cap(list.Len() - 1) {
						//log.Debug("===========promoteExecutables,len(offenders) > 0 range list.Cap=============")
						// Drop the transaction from the global pools too
						hash := tx.Hash()
						pool.all.Remove(hash)
						pool.priced.Removed()
						if types.IsDcrmLockOut(tx) {
						    pool.removelo(tx)
						    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
						}

						    // Update the account nonce to the dropped transaction
						    if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
							    pool.pendingState.SetNonce(addr, nonce)
						    }
						    log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
					    }
					    pending--
				    }
			    }
		    }
		    pendingRateLimitCounter.Inc(int64(pendingBeforeCap - pending))
	    }
	    // If we've queued more transactions than the hard limit, drop oldest ones
	    queued := uint64(0)
	    for _, list := range pool.queue {
		    queued += uint64(list.Len())
	    }
	    if queued > pool.config.GlobalQueue {
		    //log.Debug("===========promoteExecutables,queued > pool.config.GlobalQueue=============")
		    // Sort all accounts with queued transactions by heartbeat
		    addresses := make(addressesByHeartbeat, 0, len(pool.queue))
		    for addr := range pool.queue {
			    if !pool.locals.contains(addr) { // don't drop locals
				    addresses = append(addresses, addressByHeartbeat{addr, pool.beats[addr]})
			    }
		    }
		    sort.Sort(addresses)

		    // Drop transactions until the total is below the limit or only locals remain
		for drop := queued - pool.config.GlobalQueue; drop > 0 && len(addresses) > 0; {
			addr := addresses[len(addresses)-1]
			list := pool.queue[addr.address]

			addresses = addresses[:len(addresses)-1]

			// Drop all transactions if they are less than the overflow
			if size := uint64(list.Len()); size <= drop {
				for _, tx := range list.Flatten() {
				    //log.Debug("===========promoteExecutables,range list.Flatten()=============")
					pool.removeTx(tx.Hash(), true)
				}
				drop -= size
				queuedRateLimitCounter.Inc(int64(size))
				continue
			}
			// Otherwise drop only last few transactions
			txs := list.Flatten()
			for i := len(txs) - 1; i >= 0 && drop > 0; i-- {
				//log.Debug("===========promoteExecutables,list.Flatten()=============")
				pool.removeTx(txs[i].Hash(), true)
				drop--
				queuedRateLimitCounter.Inc(1)
			}
		}
	}
}

// demoteUnexecutables removes invalid and processed transactions from the pools
// executable/pending queue and any subsequent transactions that become unexecutable
// are moved back into the future queue.
func (pool *TxPool) demoteUnexecutables() {
	//log.Debug("===========demoteUnexecutables=============")
	// Iterate over all accounts and demote any non-executable transactions
	for addr, list := range pool.pending {
		nonce := pool.currentState.GetNonce(addr)
		//log.Debug("=========demoteUnexecutables,","addr",addr,"nonce",nonce,"list",list,"","============")

		// Drop all transactions that are deemed too old (low nonce)
		for _, tx := range list.Forward(nonce) {
			hash := tx.Hash()
			log.Trace("Removed old pending transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			if types.IsDcrmLockOut(tx) {
			    pool.removelo(tx)
			    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
			}
		}
		
		// Drop all transactions that are too costly (low balance or out of gas), and queue any invalids back for later
		drops, invalids := list.Filter(pool,pool.currentState.GetBalance(addr,"FSN"), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			//log.Debug("===========promoteExecutables,range drops,","hash",hash,"","==============")
			log.Trace("Removed unpayable pending transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			pendingNofundsCounter.Inc(1)
			if types.IsDcrmLockOut(tx) {
			    pool.removelo(tx)
			    //types.DeleteDcrmLockoutFeeData(tx.Hash().Hex())
			}
		}
		for _, tx := range invalids {
			hash := tx.Hash()
			log.Trace("Demoting pending transaction", "hash", hash)
			pool.enqueueTx(hash, tx)
		}
		// If there's a gap in front, alert (should never happen) and postpone all transactions
		if list.Len() > 0 && list.txs.Get(nonce) == nil {
			for _, tx := range list.Cap(0) {
				hash := tx.Hash()
				log.Error("Demoting invalidated transaction", "hash", hash)
				pool.enqueueTx(hash, tx)
			}
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			//log.Debug("===========promoteExecutables,Delete the entire queue entry if it became empty=============")
			delete(pool.pending, addr)
			delete(pool.beats, addr)
		}
	}
}

func (pool *TxPool) ClearPending(from common.Address) {
    pending, err := pool.Pending()
    if err != nil || len(pending) == 0 {
	return
    }
    
    if txs2 := pending[from]; len(txs2) > 0 {
	for _,tx := range txs2 {
	    hash := tx.Hash()
	    pool.all.Remove(hash)
	    pool.priced.Removed()
	    if types.IsDcrmLockOut(tx) {
		pool.removelo(tx)
	    }
	}
    }
    
    delete(pool.pending, from)
    delete(pool.beats, from)
}

// addressByHeartbeat is an account address tagged with its last activity timestamp.
type addressByHeartbeat struct {
	address   common.Address
	heartbeat time.Time
}

type addressesByHeartbeat []addressByHeartbeat

func (a addressesByHeartbeat) Len() int           { return len(a) }
func (a addressesByHeartbeat) Less(i, j int) bool { return a[i].heartbeat.Before(a[j].heartbeat) }
func (a addressesByHeartbeat) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// accountSet is simply a set of addresses to check for existence, and a signer
// capable of deriving addresses from transactions.
type accountSet struct {
	accounts map[common.Address]struct{}
	signer   types.Signer
	cache    *[]common.Address
}

// newAccountSet creates a new address set with an associated signer for sender
// derivations.
func newAccountSet(signer types.Signer) *accountSet {
	return &accountSet{
		accounts: make(map[common.Address]struct{}),
		signer:   signer,
	}
}

// contains checks if a given address is contained within the set.
func (as *accountSet) contains(addr common.Address) bool {
	_, exist := as.accounts[addr]
	return exist
}

// containsTx checks if the sender of a given tx is within the set. If the sender
// cannot be derived, this method returns false.
func (as *accountSet) containsTx(tx *types.Transaction) bool {
	if addr, err := types.Sender(as.signer, tx); err == nil {
		return as.contains(addr)
	}
	return false
}

// add inserts a new address into the set to track.
func (as *accountSet) add(addr common.Address) {
	as.accounts[addr] = struct{}{}
	as.cache = nil
}

// flatten returns the list of addresses within this set, also caching it for later
// reuse. The returned slice should not be changed!
func (as *accountSet) flatten() []common.Address {
	if as.cache == nil {
		accounts := make([]common.Address, 0, len(as.accounts))
		for account := range as.accounts {
			accounts = append(accounts, account)
		}
		as.cache = &accounts
	}
	return *as.cache
}

// txLookup is used internally by TxPool to track transactions while allowing lookup without
// mutex contention.
//
// Note, although this type is properly protected against concurrent access, it
// is **not** a type that should ever be mutated or even exposed outside of the
// transaction pool, since its internal state is tightly coupled with the pools
// internal mechanisms. The sole purpose of the type is to permit out-of-bound
// peeking into the pool in TxPool.Get without having to acquire the widely scoped
// TxPool.mu mutex.
type txLookup struct {
	all  map[common.Hash]*types.Transaction
	lock sync.RWMutex
}

// newTxLookup returns a new txLookup structure.
func newTxLookup() *txLookup {
	return &txLookup{
		all: make(map[common.Hash]*types.Transaction),
	}
}

// Range calls f on each key and value present in the map.
func (t *txLookup) Range(f func(hash common.Hash, tx *types.Transaction) bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for key, value := range t.all {
		if !f(key, value) {
			break
		}
	}
}

// Get returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) Get(hash common.Hash) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.all[hash]
}

// Count returns the current number of items in the lookup.
func (t *txLookup) Count() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.all)
}

// Add adds a transaction to the lookup.
func (t *txLookup) Add(tx *types.Transaction) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.all[tx.Hash()] = tx
}

// Remove removes a transaction from the lookup.
func (t *txLookup) Remove(hash common.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.all, hash)
}

func GetTrade(s string) string {
    if s == ""  {
	return ""
    }

    return gettrade(s)
}

func (pool *TxPool) GetAccountNonce(account common.Address) (uint64,error) {
    if pool == nil {
	return 0,errors.New("get nonce fail.")
    }
	
    return pool.currentState.GetNonce(account),nil
}

func (pool *TxPool) GetMatchNonce(account common.Address) string {
    return pool.currentState.GetMatchNonce(account)
}

func (pool *TxPool) GetFrom(tx *types.Transaction) (common.Address,error) {
    if tx == nil {
	return common.Address{},errors.New("get from fail.")
    }

    from, err := types.Sender(pool.signer, tx)
    if err != nil {
	return common.Address{},err
    }

    return from,nil
}

func (pool *TxPool) RmTx(hash common.Hash, outofbound bool) {
    pool.mu.Lock()
    defer pool.mu.Unlock()
    pool.removeTx(hash,outofbound)
}

