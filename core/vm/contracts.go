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

package vm

import (
	"crypto/sha256"
	"errors"
	"math/big"
	"strings"
	"fmt"
	"sync"
	"github.com/fusion/go-fusion/core/types"
	"os"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/common/math"
	"github.com/fusion/go-fusion/crypto"
	"github.com/fusion/go-fusion/crypto/bn256"
	"github.com/fusion/go-fusion/params"
	"golang.org/x/crypto/ripemd160"
	"github.com/fusion/go-fusion/ethdb"
	"github.com/fusion/go-fusion/log"
	"github.com/fusion/go-fusion/crypto/dcrm"
	"github.com/shopspring/decimal"
	"github.com/fusion/go-fusion/crypto/dcrm/cryptocoins"
)

func init() {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	log.Root().SetHandler(glogger)
}

var (
    settleordercb   func(*EVM,common.Address,string)
    getunitcb func(string) (decimal.Decimal,decimal.Decimal,error)
    getabbytradecb func(string) (string,string)
    getexchangedefaultfeecb func() decimal.Decimal
    AddNewTradeCb   func(string,bool) error

)

//for orderbook
func RegisterSettleOdcb(recvObFunc func(*EVM,common.Address,string)) {
	settleordercb = recvObFunc
}

func RegisterGetUnitCB(recvObFunc func(string) (decimal.Decimal,decimal.Decimal,error)) {
	getunitcb = recvObFunc
}

func RegisterGetABByTradeCB(recvObFunc func(string) (string,string)) {
	getabbytradecb = recvObFunc
}

func RegisterGetExChangeDefaultFeeCB(recvObFunc func() decimal.Decimal) {
	getexchangedefaultfeecb = recvObFunc
}

func RegisterAddNewTradeCB(recvObFunc func(string,bool) error) {
	AddNewTradeCb = recvObFunc
}

// PrecompiledContract is the basic interface for native Go contracts. The implementation
// requires a deterministic gas count based on the input size of the Run method of the
// contract.
type PrecompiledContract interface {
	RequiredGas(input []byte) uint64  // RequiredPrice calculates the contract gas use
	Run(input []byte, contract *Contract, evm *EVM) ([]byte, error)
	ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error
}

// PrecompiledContractsHomestead contains the default set of pre-compiled Ethereum
// contracts used in the Frontier and Homestead releases.
var PrecompiledContractsHomestead = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	types.DcrmPrecompileAddr: &dcrmTransaction{Tx:""},
	types.XvcPrecompileAddr: &xvcTransaction{Tx:""},
	types.EosPrecompileAddr: &eosInitTransaction{Tx:""},
}

// PrecompiledContractsByzantium contains the default set of pre-compiled Ethereum
// contracts used in the Byzantium release.
var PrecompiledContractsByzantium = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	common.BytesToAddress([]byte{5}): &bigModExp{},
	common.BytesToAddress([]byte{6}): &bn256Add{},
	common.BytesToAddress([]byte{7}): &bn256ScalarMul{},
	common.BytesToAddress([]byte{8}): &bn256Pairing{},
	types.DcrmPrecompileAddr: &dcrmTransaction{Tx:""},
	types.XvcPrecompileAddr: &xvcTransaction{Tx:""},
	types.EosPrecompileAddr: &eosInitTransaction{Tx:""},
}

// RunPrecompiledContract runs and evaluates the output of a precompiled contract.
func RunPrecompiledContract(p PrecompiledContract, input []byte, contract *Contract, evm *EVM) (ret []byte, err error) {
	gas := p.RequiredGas(input)
	if contract.UseGas(gas) {
		return p.Run(input, contract, evm)
	}
	return nil, ErrOutOfGas
}

// ECRECOVER implemented as a native contract.
type ecrecover struct{}

func (c *ecrecover) RequiredGas(input []byte) uint64 {
	return params.EcrecoverGas
}

func (c *ecrecover) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *ecrecover) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {

	const ecRecoverInputLength = 128

	input = common.RightPadBytes(input, ecRecoverInputLength)
	// "input" is (hash, v, r, s), each 32 bytes
	// but for ecrecover we want (r, s, v)

	r := new(big.Int).SetBytes(input[64:96])
	s := new(big.Int).SetBytes(input[96:128])
	v := input[63] - 27

	// tighter sig s values input homestead only apply to tx sigs
	if !allZero(input[32:63]) || !crypto.ValidateSignatureValues(v, r, s, false) {
		return nil, nil
	}
	// v needs to be at the end for libsecp256k1
	pubKey, err := crypto.Ecrecover(input[:32], append(input[64:128], v))
	// make sure the public key is a valid one
	if err != nil {
		return nil, nil
	}

	// the first byte of pubkey is bitcoin heritage
	return common.LeftPadBytes(crypto.Keccak256(pubKey[1:])[12:], 32), nil
}

// SHA256 implemented as a native contract.
type sha256hash struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
//
// This method does not require any overflow checking as the input size gas costs
// required for anything significant is so high it's impossible to pay for.
func (c *sha256hash) RequiredGas(input []byte) uint64 {
	return uint64(len(input)+31)/32*params.Sha256PerWordGas + params.Sha256BaseGas
}
func (c *sha256hash) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *sha256hash) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	h := sha256.Sum256(input)
	return h[:], nil
}

// RIPEMD160 implemented as a native contract.
type ripemd160hash struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
//
// This method does not require any overflow checking as the input size gas costs
// required for anything significant is so high it's impossible to pay for.
func (c *ripemd160hash) RequiredGas(input []byte) uint64 {
	return uint64(len(input)+31)/32*params.Ripemd160PerWordGas + params.Ripemd160BaseGas
}
func (c *ripemd160hash) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *ripemd160hash) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	ripemd := ripemd160.New()
	ripemd.Write(input)
	return common.LeftPadBytes(ripemd.Sum(nil), 32), nil
}

// data copy implemented as a native contract.
type dataCopy struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
//
// This method does not require any overflow checking as the input size gas costs
// required for anything significant is so high it's impossible to pay for.
func (c *dataCopy) RequiredGas(input []byte) uint64 {
	return uint64(len(input)+31)/32*params.IdentityPerWordGas + params.IdentityBaseGas
}
func (c *dataCopy) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *dataCopy) Run(in []byte, contract *Contract, evm *EVM) ([]byte, error) {
	return in, nil
}

// bigModExp implements a native big integer exponential modular operation.
type bigModExp struct{}

var (
	big1      = big.NewInt(1)
	big4      = big.NewInt(4)
	big8      = big.NewInt(8)
	big16     = big.NewInt(16)
	big32     = big.NewInt(32)
	big64     = big.NewInt(64)
	big96     = big.NewInt(96)
	big480    = big.NewInt(480)
	big1024   = big.NewInt(1024)
	big3072   = big.NewInt(3072)
	big199680 = big.NewInt(199680)
)

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bigModExp) RequiredGas(input []byte) uint64 {
	var (
		baseLen = new(big.Int).SetBytes(getData(input, 0, 32))
		expLen  = new(big.Int).SetBytes(getData(input, 32, 32))
		modLen  = new(big.Int).SetBytes(getData(input, 64, 32))
	)
	if len(input) > 96 {
		input = input[96:]
	} else {
		input = input[:0]
	}
	// Retrieve the head 32 bytes of exp for the adjusted exponent length
	var expHead *big.Int
	if big.NewInt(int64(len(input))).Cmp(baseLen) <= 0 {
		expHead = new(big.Int)
	} else {
		if expLen.Cmp(big32) > 0 {
			expHead = new(big.Int).SetBytes(getData(input, baseLen.Uint64(), 32))
		} else {
			expHead = new(big.Int).SetBytes(getData(input, baseLen.Uint64(), expLen.Uint64()))
		}
	}
	// Calculate the adjusted exponent length
	var msb int
	if bitlen := expHead.BitLen(); bitlen > 0 {
		msb = bitlen - 1
	}
	adjExpLen := new(big.Int)
	if expLen.Cmp(big32) > 0 {
		adjExpLen.Sub(expLen, big32)
		adjExpLen.Mul(big8, adjExpLen)
	}
	adjExpLen.Add(adjExpLen, big.NewInt(int64(msb)))

	// Calculate the gas cost of the operation
	gas := new(big.Int).Set(math.BigMax(modLen, baseLen))
	switch {
	case gas.Cmp(big64) <= 0:
		gas.Mul(gas, gas)
	case gas.Cmp(big1024) <= 0:
		gas = new(big.Int).Add(
			new(big.Int).Div(new(big.Int).Mul(gas, gas), big4),
			new(big.Int).Sub(new(big.Int).Mul(big96, gas), big3072),
		)
	default:
		gas = new(big.Int).Add(
			new(big.Int).Div(new(big.Int).Mul(gas, gas), big16),
			new(big.Int).Sub(new(big.Int).Mul(big480, gas), big199680),
		)
	}
	gas.Mul(gas, math.BigMax(adjExpLen, big1))
	gas.Div(gas, new(big.Int).SetUint64(params.ModExpQuadCoeffDiv))

	if gas.BitLen() > 64 {
		return math.MaxUint64
	}
	return gas.Uint64()
}

func (c *bigModExp) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *bigModExp) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	var (
		baseLen = new(big.Int).SetBytes(getData(input, 0, 32)).Uint64()
		expLen  = new(big.Int).SetBytes(getData(input, 32, 32)).Uint64()
		modLen  = new(big.Int).SetBytes(getData(input, 64, 32)).Uint64()
	)
	if len(input) > 96 {
		input = input[96:]
	} else {
		input = input[:0]
	}
	// Handle a special case when both the base and mod length is zero
	if baseLen == 0 && modLen == 0 {
		return []byte{}, nil
	}
	// Retrieve the operands and execute the exponentiation
	var (
		base = new(big.Int).SetBytes(getData(input, 0, baseLen))
		exp  = new(big.Int).SetBytes(getData(input, baseLen, expLen))
		mod  = new(big.Int).SetBytes(getData(input, baseLen+expLen, modLen))
	)
	if mod.BitLen() == 0 {
		// Modulo 0 is undefined, return zero
		return common.LeftPadBytes([]byte{}, int(modLen)), nil
	}
	return common.LeftPadBytes(base.Exp(base, exp, mod).Bytes(), int(modLen)), nil
}

// newCurvePoint unmarshals a binary blob into a bn256 elliptic curve point,
// returning it, or an error if the point is invalid.
func newCurvePoint(blob []byte) (*bn256.G1, error) {
	p := new(bn256.G1)
	if _, err := p.Unmarshal(blob); err != nil {
		return nil, err
	}
	return p, nil
}

// newTwistPoint unmarshals a binary blob into a bn256 elliptic curve point,
// returning it, or an error if the point is invalid.
func newTwistPoint(blob []byte) (*bn256.G2, error) {
	p := new(bn256.G2)
	if _, err := p.Unmarshal(blob); err != nil {
		return nil, err
	}
	return p, nil
}

// bn256Add implements a native elliptic curve point addition.
type bn256Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256Add) RequiredGas(input []byte) uint64 {
	return params.Bn256AddGas
}

func (c *bn256Add) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *bn256Add) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	x, err := newCurvePoint(getData(input, 0, 64))
	if err != nil {
		return nil, err
	}
	y, err := newCurvePoint(getData(input, 64, 64))
	if err != nil {
		return nil, err
	}
	res := new(bn256.G1)
	res.Add(x, y)
	return res.Marshal(), nil
}

// bn256ScalarMul implements a native elliptic curve scalar multiplication.
type bn256ScalarMul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256ScalarMul) RequiredGas(input []byte) uint64 {
	return params.Bn256ScalarMulGas
}

func (c *bn256ScalarMul) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *bn256ScalarMul) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	p, err := newCurvePoint(getData(input, 0, 64))
	if err != nil {
		return nil, err
	}
	res := new(bn256.G1)
	res.ScalarMult(p, new(big.Int).SetBytes(getData(input, 64, 32)))
	return res.Marshal(), nil
}

var (
	// true32Byte is returned if the bn256 pairing check succeeds.
	true32Byte = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	// false32Byte is returned if the bn256 pairing check fails.
	false32Byte = make([]byte, 32)

	// errBadPairingInput is returned if the bn256 pairing input is invalid.
	errBadPairingInput = errors.New("bad elliptic curve pairing size")
)

// bn256Pairing implements a pairing pre-compile for the bn256 curve
type bn256Pairing struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256Pairing) RequiredGas(input []byte) uint64 {
	return params.Bn256PairingBaseGas + uint64(len(input)/192)*params.Bn256PairingPerPointGas
}

func (c *bn256Pairing) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

func (c *bn256Pairing) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	// Handle some corner cases cheaply
	if len(input)%192 > 0 {
		return nil, errBadPairingInput
	}
	// Convert the input into a set of coordinates
	var (
		cs []*bn256.G1
		ts []*bn256.G2
	)
	for i := 0; i < len(input); i += 192 {
		c, err := newCurvePoint(input[i : i+64])
		if err != nil {
			return nil, err
		}
		t, err := newTwistPoint(input[i+64 : i+192])
		if err != nil {
			return nil, err
		}
		cs = append(cs, c)
		ts = append(ts, t)
	}
	// Execute the pairing checks and return the results
	if bn256.PairingCheck(cs, ts) {
		return true32Byte, nil
	}
	return false32Byte, nil
}

type eosInitTransaction struct {
	Tx string
}

func (c *eosInitTransaction) RequiredGas(input []byte) uint64 {
	str := string(input)
	if len(str) == 0 {
		return params.SstoreSetGas * 2
	}
	m := strings.Split(str,":")
	if m[0] == "EOS_INITIATE" {
		return 1
	}
	return params.SstoreSetGas * 2
}

func (c *eosInitTransaction) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
	log.Debug("============Run EosContract, Start============","tx hash",evm.GetTxhash(),"tx data",string(input))
	str := string(input)

	m := strings.Split(str,":")
	if m[0] == "EOS_INITIATE" {
		log.Debug("============Run EosInitiate============","tx hash",evm.GetTxhash(),"tx data",string(input))
		if len(m) == 5 {
			from := common.BytesToAddress([]byte("eos"))
			h := crypto.Keccak256Hash([]byte("eossettings"))
			log.Debug("============Run EosInitiate","from",from,"h",h,"","===============","tx hash",evm.GetTxhash(),"tx data",string(input))
			ok1 := dcrm.CheckRealEosAccount(m[1],m[2],m[3])
			if ok1 == false {
				return nil, errors.New("cannot verify account and pubkeys on EOS.")
			}
			exist := evm.StateDB.Exist(from)
			log.Debug("========Run EosInitiate, AAAAAAAA","evm.StateDB.Exist(from)",exist,"tx hash",evm.GetTxhash(),"tx data",string(input))
			if !exist {
				evm.StateDB.CreateAccount(from)
				log.Debug("========Run EosInitiate, BBBBBBBB","evm.StateDB.Exist(from)",evm.StateDB.Exist(from),"tx hash",evm.GetTxhash(),"tx data",string(input),"","========")
			}
			ret := evm.StateDB.GetStateDcrmAccountData(from,h)
			if ret != nil {
				log.Debug("===========Run EosInitiate: eos account already set.==========","tx hash",evm.GetTxhash(),"tx data",string(input))
				return nil, errors.New("eos account already set.")
			}
			nonce := evm.StateDB.GetNonce(from)
			if nonce == 0 {
				evm.StateDB.SetNonce(from,nonce+1)
			}
			evm.StateDB.SetStateDcrmAccountData(from,h,[]byte(str))
		}
		
		var lock sync.Mutex
		lock.Lock()
		dir := dcrm.GetEosDbDir()
		db,_ := ethdb.NewLDBDatabase(dir, 0, 0)
		if db == nil {
			log.Debug("==============Run EosInitiate,open db fail.============","tx hash",evm.GetTxhash(),"tx data",string(input))
			lock.Unlock()
			return nil, nil
		}
		db.Put([]byte("eossettings"),input)
		db.Close()
		lock.Unlock()
	}
	return nil, nil
}

func (c *eosInitTransaction) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {
	return nil
}

type dcrmTransaction struct {
    Tx string
}

func (c *dcrmTransaction) RequiredGas(input []byte) uint64 {
    str := string(input)
    if len(str) == 0 {
	return params.SstoreSetGas * 2
    }

    realtxdata,_ := types.GetRealTxData(str)
    m := strings.Split(realtxdata,":")
    if m[0] == "LOCKIN" {
	return 0 
    }
	
    if m[0] == "DCRMCONFIRMADDR" {
	return 0 
    }
	
    return params.SstoreSetGas * 2
}

func GetOutSideRealFee(chandler cryptocoins.CryptocoinHandler,txhash string) *big.Int {
    //log.Debug("=========GetOutSideRealFee=============","chandler",chandler,"txhash",txhash)
    return chandler.GetDefaultFee().Val

    if chandler == nil || txhash == "" {
	log.Debug("=========GetOutSideRealFee,param error,return nil.=============","chandler",chandler,"txhash",txhash)
	return nil
    }

    realfee,ok := types.GetDcrmLockoutFeeDataKReady(txhash)
    if !ok {
	realfee,e := dcrm.GetLockoutRealFeeFromLocalDB(txhash)
	if e == nil {
	    log.Debug("=========GetOutSideRealFee,not in map,get from local db success.=============","realfee",realfee,"chandler",chandler,"txhash",txhash)
	    realFee,_ := new(big.Int).SetString(realfee,10)
	    if realFee == nil {
		return chandler.GetDefaultFee().Val
	    }
	    return realFee
	} else {
	    log.Debug("=========GetOutSideRealFee,not in map,get from local db fail.=============","default fee",chandler.GetDefaultFee(),"chandler",chandler,"txhash",txhash)
	    return chandler.GetDefaultFee().Val
	}
    } else {
	log.Debug("=========GetOutSideRealFee,in map.=============","realfee",realfee,"chandler",chandler,"txhash",txhash)
	realFee,_ := new(big.Int).SetString(realfee,10)
	if realFee == nil {
	    return chandler.GetDefaultFee().Val
	}
	return realFee
    }

    return chandler.GetDefaultFee().Val
}

func (c *dcrmTransaction) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
    str := string(input)
    if len(str) == 0 {
	return nil,nil
    }

    realtxdata,_ := types.GetRealTxData(str)
    m := strings.Split(realtxdata,":")
    
    //log.Debug("==========dcrmTransaction.Run=============","tx hash",evm.GetTxhash(),"tx data",string(input))

    if m[0] == "DCRMCONFIRMADDR" {
	
	//log.Debug("===============dcrmTransaction.Run,DCRMCONFIRMADDR","from",contract.Caller().Hex(),"dcrm addr",m[1],"","=================")

	from := contract.Caller()
	num,_ := new(big.Int).SetString(dcrm.BLOCK_FORK_1,10)
	if evm.BlockNumber.Cmp(num) > 0 {
	    if strings.EqualFold(m[2], "ALL") {
		pubkeyHex := m[1]
		//log.Debug("===========dcrmTransaction.Run,confirm all==========","pubkeyHex",pubkeyHex,"tx hash",evm.GetTxhash(),"tx data",string(input))
		for _, cointype := range cryptocoins.Cointypes {
		    m[2] = cointype
		    if strings.EqualFold(cointype, "ALL") == false {
			    h := cryptocoins.NewCryptocoinHandler(cointype)
			    if h == nil {
				    //log.Debug("==========dcrmTransaction.Run,coin type not supported==========","cointype",cointype,"tx hash",evm.GetTxhash(),"tx data",string(input))
				    continue
			    }
			    dcrmAddr, err := h.PublicKeyToAddress(pubkeyHex)
			    if err != nil || dcrmAddr == "" {
				    //log.Debug("===========dcrmTransaction.Run,fail to calculate address============","error",err,"tx hash",evm.GetTxhash(),"tx data",string(input))
				    continue
			    }
			    m[1] = dcrmAddr
			    /////
			    evm.StateDB.SetAccountDcrmAddr(from,m[1],m[2])
			    /////
		    } else {
			    evm.StateDB.SetAccountDcrmAddr(from,pubkeyHex,"ALL")
		    }
		}
	    } else {
		    evm.StateDB.SetAccountDcrmAddr(from,m[1],m[2])
	    }

	    return nil,nil
	}
    }

    if m[0] == "LOCKIN" {
	//log.Debug("dcrmTransaction.Run,LOCKIN")
	from := contract.Caller()

	num,_ := new(big.Int).SetString(dcrm.BLOCK_FORK_1,10)
	if evm.BlockNumber.Cmp(num) > 0 {
	    ba := evm.StateDB.GetBalance(from,m[3])
	    outba := evm.StateDB.GetOutSideBalance(from,m[3])
	    ba2,_ := new(big.Int).SetString(m[2],10)
	    //log.Debug("=============dcrmTransaction.Run.lockin===============","from",from,"cointype",m[3],"balance",ba,"outside balance",outba,"lockin value",ba2,"tx hash",evm.GetTxhash(),"tx data",string(input))
	    
	    //bug
	    if ba2 == nil {
		//log.Debug("=============dcrmTransaction.Run.lockin,get from addr balance from block is nil.===============","from",from,"tx hash",evm.GetTxhash(),"tx data",string(input))
		return nil,nil
	    }
	    
	    b := new(big.Int).Add(ba,ba2)
	    outb := new(big.Int).Add(outba,ba2)
	    
	    log.Debug("=============dcrmTransaction.Run.lockin===============","from",from,"cointype",m[3],"update balance",b,"update outside balance",outb,"lockin value",ba2,"tx hash",evm.GetTxhash(),"tx data",string(input))
 	    
	    d := evm.StateDB.GetDcrmAddr(from, m[3])
	    if d == "" {
		    // lockin new erc20 omni evt bep2 tokens
		    log.Debug("========dcrmTransaction.Run.lockin, dcrm addr not confirmed========")
		    chandler := cryptocoins.NewCryptocoinHandler(m[3])
		    pubkeyHex := evm.StateDB.GetDcrmAddr(from,"ALL")
		    dcrmaddr, err := chandler.PublicKeyToAddress(pubkeyHex)
		    if err != nil {
			    log.Debug("========dcrmTransaction.Run.lockin, generate dcrm address failed.========", "err", err)
			    return nil, nil
		    }
		    evm.StateDB.AddAccountDcrmAddr(from,dcrmaddr,m[3])
	    }
	    
	    evm.StateDB.SetBalance(from,b,m[3])
	    evm.StateDB.SetOutSideBalance(from,outb,m[3])
	    evm.StateDB.SetAccountHashkey(from,m[1],m[3])

	    return nil,nil
	}
    }

    if m[0] == "LOCKOUT" {
	hash := crypto.Keccak256Hash([]byte(evm.GetTxhash() + ":" + strings.ToLower(m[3]))).Hex()
	val,err := dcrm.GetLockoutInfoFromLocalDB(hash)
	if err != nil {
	    log.Debug("===============dcrmTransaction.Run,LOCKOUT.get real account fail.ok is false.===================","tx hash",evm.GetTxhash())
	    return nil,nil
	}
	//log.Debug("=============dcrmTransaction.Run,LOCKOUT.===========","get real account info",val,"tx hash",evm.GetTxhash())

	retvas := strings.Split(val,common.Sep10)
	if len(retvas) < 2 {
	    log.Debug("===============dcrmTransaction.Run,LOCKOUT.get real account fail.len < 2.===================","tx hash",evm.GetTxhash())
	    return nil,nil
	}
	//hashkey := retvas[0]
	realfusionfrom := retvas[0]
	//realdcrmfrom := retvas[2]
	
	realchoosefrom := common.HexToAddress(realfusionfrom)//contract.Caller()

	from := contract.Caller()
	if from == (common.Address{}) {
	    log.Debug("===============dcrmTransaction.Run,LOCKOUT.get real account fail.from is nil.===================","tx hash",evm.GetTxhash())
	    return nil,nil
	}

	num,_ := new(big.Int).SetString(dcrm.BLOCK_FORK_1,10)
	if evm.BlockNumber.Cmp(num) > 0 {
	    chandler := cryptocoins.NewCryptocoinHandler(m[3])
	    if chandler != nil {
		ba := evm.StateDB.GetBalance(from,m[3])
		outba := evm.StateDB.GetOutSideBalance(realchoosefrom,m[3])
		ba2,_ := new(big.Int).SetString(m[2],10)
		b := new(big.Int).Sub(ba,ba2)
		outb := new(big.Int).Sub(outba,ba2)

		log.Debug("========dcrmTransaction.Run=========","from",from,"cointype",m[3],"balance",ba,"outside balance",outba,"amount",m[2],"amount value",ba2,"b",b,"outb",outb)

		if chandler.IsToken() == false {
		    /*realfee,ok := types.GetDcrmLockoutFeeDataKReady(evm.GetTxhash())
		    if !ok {
			realfee,e := dcrm.GetLockoutRealFeeFromLocalDB(evm.GetTxhash())
			if e == nil {
			    realFee,_ := new(big.Int).SetString(realfee,10)
			    b = new(big.Int).Sub(b,realFee)
			} else {
			    b = new(big.Int).Sub(b,chandler.GetDefaultFee().Val)
			}
		    } else {
			realFee,_ := new(big.Int).SetString(realfee,10)
			b = new(big.Int).Sub(b,realFee)
		    }*/
		    
		    realfee := GetOutSideRealFee(chandler,evm.GetTxhash())
		    if realfee == nil {
			log.Debug("===============dcrmTransaction.Run,LOCKOUT,it is coin.get real fee fail.===================","tx hash",evm.GetTxhash())
			return nil,nil
		    }

		    b = new(big.Int).Sub(b,chandler.GetDefaultFee().Val)
		    outb = new(big.Int).Sub(outb,realfee)
		    zero,_ := new(big.Int).SetString("0",10)
		    ///
		    if b.Cmp(zero) < 0 || outb.Cmp(zero) < 0 {
			log.Debug("=========dcrmTransaction.Run,LOCKOUT,it is coin.balance < 0.=========","real fee",realfee,"update balance",b,"update outside balance",outb,"from",from,"tx hash",evm.GetTxhash(),"tx data",string(input))
			return nil,errors.New("balance < 0")
		    }

		    log.Debug("=========dcrmTransaction.Run,LOCKOUT,it is coin.=========","real fee",realfee,"update balance",b,"update outside balance",outb,"from",from,"tx hash",evm.GetTxhash(),"tx data",string(input))
		    evm.StateDB.SetBalance(from,b,m[3])
		    evm.StateDB.SetOutSideBalance(realchoosefrom,outb,m[3])
		} else {
		    // token
		    // 1. 扣除fee
		    feeCointype := chandler.GetDefaultFee().Cointype
		    b0 := evm.StateDB.GetBalance(from,feeCointype)
		    outb0 := evm.StateDB.GetOutSideBalance(realchoosefrom,feeCointype)

		    /*realfee,ok := types.GetDcrmLockoutFeeDataKReady(evm.GetTxhash())
		    if !ok {
			    realfee,e := dcrm.GetLockoutRealFeeFromLocalDB(evm.GetTxhash())
			    if e == nil {
				realFee,_ := new(big.Int).SetString(realfee,10)
				b0 = new(big.Int).Sub(b0,realFee)
			    } else {
				b0 = new(big.Int).Sub(b0, chandler.GetDefaultFee().Val)
			    }
		    } else {
			    realFee,_ := new(big.Int).SetString(realfee,10)
			    b0 = new(big.Int).Sub(b0, realFee)
		    }*/

		    realfee := GetOutSideRealFee(chandler,evm.GetTxhash())
		    if realfee == nil {
			log.Debug("===============dcrmTransaction.Run,LOCKOUT,it is token.get real fee fail.===================","tx hash",evm.GetTxhash())
			return nil,nil
		    }
		    b0 = new(big.Int).Sub(b0, chandler.GetDefaultFee().Val)
		    outb0 = new(big.Int).Sub(outb0,realfee)

		    log.Debug("=========dcrmTransaction.Run,LOCKOUT,it is token.=========","real fee",realfee,"update fee-cointype balance",b0,"update fee-cointype outside balance",outb0,"update balance",b,"update outside balance",outb,"from",from,"tx hash",evm.GetTxhash(),"tx data",string(input))
		    evm.StateDB.SetBalance(from,b0,feeCointype)
		    evm.StateDB.SetOutSideBalance(realchoosefrom,outb0,feeCointype)

		    // 2. update token balance
		    evm.StateDB.SetBalance(from,b,m[3])
		    evm.StateDB.SetOutSideBalance(realchoosefrom,outb,m[3])
		}
	    }

	    return nil,nil
	}
    }
 
    if m[0] == "TRANSACTION" {
	//log.Debug("dcrmTransaction.Run,TRANSACTION")
	from := contract.Caller()
	toaddr,_ := new(big.Int).SetString(m[1],0)
	to := common.BytesToAddress(toaddr.Bytes())

	num,_ := new(big.Int).SetString(dcrm.BLOCK_FORK_1,10)
	if evm.BlockNumber.Cmp(num) > 0 {
	    //bug
	    num2,_ := new(big.Int).SetString(dcrm.BLOCK_FORK_0,10)
	    if strings.EqualFold(from.Hex(),to.Hex()) && evm.BlockNumber.Cmp(num2) > 0 {
		return nil,nil
	    }
	    
	    ba := evm.StateDB.GetBalance(from,m[3])
	    ba2,_ := new(big.Int).SetString(m[2],10)
	    b := new(big.Int).Sub(ba,ba2)
	    evm.StateDB.SetBalance(from,b,m[3])
	   
	    //////
	    d := evm.StateDB.GetDcrmAddr(to, m[3])
	    if d == "" {
		// lockin new erc20 omni evt bep2 tokens
		log.Debug("========dcrmTransaction.Run.TRANSACTION, dcrm addr not confirmed========")
		chandler := cryptocoins.NewCryptocoinHandler(m[3])
		pubkeyHex := evm.StateDB.GetDcrmAddr(to,"ALL")
		dcrmaddr, err := chandler.PublicKeyToAddress(pubkeyHex)
		if err != nil {
			log.Debug("========dcrmTransaction.Run.TRANSACTION, generate dcrm address failed.========", "err", err)
			evm.StateDB.AddAccountDcrmAddr(to,"",m[3])
		} else {
			evm.StateDB.AddAccountDcrmAddr(to,dcrmaddr,m[3])
		}
	    } 
	    //////

	    ba = evm.StateDB.GetBalance(to,m[3])
	    b = new(big.Int).Add(ba,ba2)
	    evm.StateDB.SetBalance(to,b,m[3])
	    //
	    return nil,nil
	}
    }
 
    return nil,nil
}

func (c *dcrmTransaction) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {

    return nil
}

//xvc

type xvcTransaction struct {
    Tx string
}

func (c *xvcTransaction) RequiredGas(input []byte) uint64 {
    str := string(input)
    if len(str) == 0 {
	return params.SstoreSetGas * 2
    }

    realtxdata,_ := types.GetRealTxData(str)
    m := strings.Split(realtxdata,common.SepOB)
    if m[0] == "ODB" {
	return 0 
    }

    return params.SstoreSetGas * 2
}

func (c *xvcTransaction) Run(input []byte, contract *Contract, evm *EVM) ([]byte, error) {
    str := string(input)
    if len(str) == 0 {
	return nil,nil
    }

    m := strings.Split(str,common.SepOB)
    
    if m[0] == "ODB" {
	from := contract.Caller()
	//log.Debug("====================xvcTransaction.Run.ODB==================","match in block height",m[1],"match result data len",len(m[2]),"match result data",m[2],"tx hash",evm.GetTxhash())
	settleordercb(evm,from,str)
    }
    
    mm := strings.Split(str,":")
    if mm[0] == "ADDNEWTRADE" {
	//log.Info("====================xvcTransaction.Run.ADDNEWTRADE==================")
	AddNewTradeCb(mm[1],true)
    }

    return nil,nil
}

func (c *xvcTransaction) ValidTx(stateDB StateDB, signer types.Signer, tx *types.Transaction) error {

    return nil
}

func GetDecimalNumber(s string) string {
    if s == "" {
	return s
    }

    nums := []rune(s)
    for k,_ := range nums {
	if string(nums[k:k+1]) == "." {
	    return string(nums[0:k])
	}
    }

    return s
}

func SettleOrders(evmtmp *EVM,price decimal.Decimal,from common.Address,side string,trade string,quan decimal.Decimal) {

    if evmtmp == nil {
	return
    }

    for _,trade2 := range types.AllSupportedTrades {
	//log.Info("===========SettleOrders==============","trade",trade2,"settle trade",trade)

	if strings.EqualFold(trade,trade2) {
	    A,B := getabbytradecb(trade)
	    if A == "" || B == "" {
		break
	    }

	    Ua,Ub,err := getunitcb(trade)
	    if err != nil {
		log.Info("===========SettleOrders==============","err",err,"Ua",Ua,"Ub",Ub,"trade",trade2,"settle trade",trade)
		break
	    }

	    ///
	    if strings.EqualFold(side,"Buy") {
		b := price.Mul(quan)
		b = b.Mul(Ub)
		//ut,_ := decimal.NewFromString(dcrm.UT)
		//ut2 := ut.Mul(ut)
		//b = b.Div(ut2)

		a := quan.Mul(Ua)
		//a = a.Div(ut)
		
		if evmtmp != nil {
		    ba := evmtmp.StateDB.GetBalance(from,B)
		    if ba == nil {
			ba,_ = new(big.Int).SetString("0",10)
		    }
		    va := fmt.Sprintf("%v",ba)
		    ba2,_ := decimal.NewFromString(va)
		    ba3 := ba2.Sub(b)
		    if ba3.Sign() < 0 {
			return  //////TODO
		    }
		    bb := GetDecimalNumber(ba3.String())
		    ba4,_ := new(big.Int).SetString(bb,10)
		    evmtmp.StateDB.SetBalance(from,ba4,B)
		    
		    ba = evmtmp.StateDB.GetBalance(from,A)
		    if ba == nil {
			ba,_ = new(big.Int).SetString("0",10)
		    }
		    va = fmt.Sprintf("%v",ba)
		    ba2,_ = decimal.NewFromString(va)
		    ba2 = ba2.Add(a)
		    fee := a.Mul(getexchangedefaultfeecb())
		    ba2 = ba2.Sub(fee)
		    bb = GetDecimalNumber(ba2.String())
		    ba4,_ = new(big.Int).SetString(bb,10)
		    evmtmp.StateDB.SetBalance(from,ba4,A)
		}
	    }
	    if strings.EqualFold(side,"Sell") {
		b := price.Mul(quan)
		b = b.Mul(Ub)
		//ut,_ := decimal.NewFromString(dcrm.UT)
		//ut2 := ut.Mul(ut)
		//b = b.Div(ut2)

		a := quan.Mul(Ua)
		//a = a.Div(ut)

		tmp := evmtmp.StateDB.GetBalance(from,A)
		if tmp == nil {
		    tmp,_ = new(big.Int).SetString("0",10)
		}
		amount := fmt.Sprintf("%v",tmp)
		ba,_ := decimal.NewFromString(amount)
		ba = ba.Sub(a)
		if ba.Sign() < 0 {
		    return  //////TODO
		}
		bb := GetDecimalNumber(ba.String())
		ba4,_ := new(big.Int).SetString(bb,10)
		evmtmp.StateDB.SetBalance(from,ba4,A)

		tmp = evmtmp.StateDB.GetBalance(from,B)
		if tmp == nil {
		    tmp,_ = new(big.Int).SetString("0",10)
		}
		amount = fmt.Sprintf("%v",tmp)
		ba,_ = decimal.NewFromString(amount)
		ba = ba.Add(b)
		fee := b.Mul(getexchangedefaultfeecb())  //TODO
		ba = ba.Sub(fee)
		bb = GetDecimalNumber(ba.String())
		ba4,_ = new(big.Int).SetString(bb,10)
		evmtmp.StateDB.SetBalance(from,ba4,B)
	    }
	    ///
	    break
	}
    }
}

