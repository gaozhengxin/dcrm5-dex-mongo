package dcrm

import (
    //"errors"
    "encoding/json"
)

// dcrm erros
var (
        //err code 1
	ErrEncodeSendMsgFail   = `{Code:1,Error:"encode send msg fail."}`
	ErrParamError   = `{Code:2,Error:"parameters error."}`
	ErrGetOtherNodesDataFail   = `{Code:3,Error:"NetWork Error,Get Data From Other Node Fail."}`
	ErrUnknownChType   = `{Code:4,Error:"unknown channel type."}`
	ErrGetChValueFail   = `{Code:5,Error:"get channel value fail."}`
	ErrNoGetLOAccout   = `{Code:6,Error:"There's no proper account to do lockout."}`
	ErrNoFindWorker   = `{Code:7,Error:"can not find worker."}`
	ErrOutsideTxFail   = `{Code:8,Error:"outside tx fail."}`
	ErrReqAddrTimeout   = `{Code:9,Error:"request dcrm address timeout."}`
	ErrGetWorkerIdError   = `{Code:10,Error:"get worker id error."}`
	ErrGetPrexDataError   = `{Code:11,Error:"get msg prefix data error."}`
	ErrValidateRealFusionAddrFail   = `{Code:12,Error:"validate real fusion from fail."}`
	ErrValidateRealDcrmFromFail   = `{Code:13,Error:"validate real dcrm from fail."}`
	ErrSendTxToNetFail   = `{Code:14,Error:"send tx to outside net fail."}`
	ErrSendDataToGroupFail   = `{Code:15,Error:"send data to group fail."}`
	ErrInternalMsgFormatError   = `{Code:16,Error:"msg data format error."}`
	ErrGetNoResFromGroupMem   = `{Code:17,Error:"no get any result from other group node."}`
	ErrCoinTypeNotSupported   = `{Code:18,Error:"coin type is not supported."}`
	ErrTokenTypeError   = `{Code:19,Error:"token type error."}`
	ErrValidateLIFromAddrFail   = `{Code:20,Error:"lockin validate from address fail."}`
	ErrValidateLIValueFail   = `{Code:21,Error:"lockin validate value fail."}`
	ErrConfirmAddrFail   = `{Code:22,Error:"the dcrm address confirm validate fail."}`
	ErrGroupNotReady   = `{Code:23,Error:"the group is not ready.please try again."}`
	ErrGetGenPubkeyFail   = `{Code:24,Error:"get generate pubkey fail."}`
	ErrGetGenSaveDataFail   = `{Code:25,Error:"get generate save data fail."}`
	ErrCreateDbFail   = `{Code:26,Error:"create db fail."}`
	ErrGetRealEosUserFail   = `{Code:27,Error:"cannot get real eos account."}`
	ErrDcrmSigWrongSize   = `{Code:28,Error:"wrong size for dcrm sig."}`
	ErrDcrmSigFail   = `{Code:29,Error:"dcrm sign fail."}`
	ErrInvalidDcrmAddr   = `{Code:30,Error:"invalid dcrm address."}`
	ErrGetC1Timeout   = `{Code:31,Error:"get C1 timeout."}`
	ErrGetEnodeByUIdFail   = `{Code:32,Error:"can not find proper enodes by uid."}`
	ErrGetD1Timeout   = `{Code:33,Error:"get D1 timeout."}`
	ErrGetSHARE1Timeout   = `{Code:34,Error:"get SHARE1 timeout."}`
	ErrGetAllSHARE1Fail   = `{Code:35,Error:"get all SHARE1 msg fail."}`
	ErrGetAllD1Fail   = `{Code:36,Error:"get all D1 msg fail."}`
	ErrVerifySHARE1Fail   = `{Code:37,Error:"verify SHARE1 fail."}`
	ErrGetAllC1Fail   = `{Code:38,Error:"get all C1 msg fail."}`
	ErrKeyGenVerifyCommitFail   = `{Code:39,Error:"verify commit in keygenerate fail."}`
	ErrGetZKFACTPROOFTimeout   = `{Code:40,Error:""get ZKFACTPROOF timeout."}`
	ErrGetZKUPROOFTimeout   = `{Code:41,Error:""get ZKUPROOF timeout."}`
	ErrGetAllZKFACTPROOFFail   = `{Code:42,Error:"get all ZKFACTPROOF msg fail."}`
	ErrVerifyZKFACTPROOFFail   = `{Code:43,Error:"verify ZKFACTPROOF fail."}`
	ErrGetAllZKUPROOFFail   = `{Code:44,Error:"get all ZKUPROOF msg fail."}`
	ErrVerifyZKUPROOFFail   = `{Code:45,Error:"verify ZKUPROOF fail."}`
	ErrGetC11Timeout   = `{Code:46,Error:"get C11 timeout."}`
	ErrGetMTAZK1PROOFTimeout   = `{Code:47,Error:"get MTAZK1PROOF timeout."}`
	ErrGetKCTimeout   = `{Code:48,Error:"get KC timeout."}`
	ErrGetAllKCFail   = `{Code:49,Error:"get all KC msg fail."}`
	ErrGetAllMTAZK1PROOFFail   = `{Code:50,Error:"get all MTAZK1PROOF msg fail."}`
	ErrVerifyMTAZK1PROOFFail   = `{Code:51,Error:"verify MTAZK1PROOF fail.""}`
	ErrGetMKGTimeout   = `{Code:52,Error:"get MKG timeout."}`
	ErrGetAllMKGFail   = `{Code:53,Error:"get all MKG msg fail."}`
	ErrGetMKWTimeout   = `{Code:54,Error:"get MKW timeout."}`
	ErrGetAllMKWFail   = `{Code:55,Error:"get all MKW msg fail."}`
	ErrVerifyMKGFail   = `{Code:56,Error:"verify MKG fail.""}`
	ErrVerifyMKWFail   = `{Code:57,Error:"verify MKW fail.""}`
	ErrGetPaillierPrivKeyFail   = `{Code:58,Error:"get paillier privkey fail.""}`
	ErrGetDELTA1Timeout   = `{Code:59,Error:"get DELTA1 timeout."}`
	ErrGetAllDELTA1Fail   = `{Code:60,Error:"get all DELTA1 msg fail."}`
	ErrGetD11Timeout   = `{Code:61,Error:"get D11 timeout."}`
	ErrGetAllD11Fail   = `{Code:62,Error:"get all D11 msg fail."}`
	ErrGetAllC11Fail   = `{Code:63,Error:"get all C11 msg fail."}`
	ErrSignVerifyCommitFail   = `{Code:64,Error:"verify commit in dcrm sign fail."}`
	ErrREqualZero   = `{Code:65,Error:"sign error: r equal zero."}`
	ErrGetS1Timeout   = `{Code:66,Error:"get S1 timeout."}`
	ErrGetAllS1Fail   = `{Code:67,Error:"get all S1 msg fail."}`
	ErrVerifySAllFail   = `{Code:68,Error:"verify SAll != m*G + r*PK in dcrm sign ec2."}`
	ErrGetSS1Timeout   = `{Code:69,Error:"get SS1 timeout."}`
	ErrGetAllSS1Fail   = `{Code:70,Error:"get all SS1 msg fail."}`
	ErrSEqualZero   = `{Code:71,Error:"sign error: s equal zero."}`
	ErrDcrmSignVerifyFail   = `{Code:72,Error:"dcrm sign verify fail."}`
	ErrInvalidCoinbase   = `{Code:73,Error:"Invalid Coinbase."}`
	ErrStateDBError   = `{Code:74,Error:"StateDB Error."}`
	ErrEosAccountNameError   = `{Code:75,Error:"eos account name must be 12 character long, lowercase letters or 1-5."}`
	ErrReqEosPubkeyError   = `{Code:76,Error:"Request eos pubkey error."}`
	
	ErrAlreadyKnownLOTx   = `{Code:101,Error:"already known lockout transaction with same nonce."}`
	ErrOrderAlreadySend   = `{Code:102,Error:"the miner has send order already."}`
	ErrTxDataError   = `{Code:103,Error:"tx input data error."}`
	ErrInvalidDcrmPubkey   = `{Code:104,Error:"invalid dcrm pubkey."}`
	ErrDcrmAddrAlreadyConfirmed  = `{Code:105,Error:"the account has confirmed dcrm address."}`
	ErrDcrmAddrNotConfirmed  = `{Code:106,Error:"the account has not confirmed dcrm address before."}`
	ErrDcrmAddrAlreadyLockIn  = `{Code:107,Error:"the dcrmaddr has lockin alread."}`
	ErrNotRealLockIn  = `{Code:108,Error:"it is not the real lockin,it is BTC change."}`
	ErrInsufficientDcrmFunds  = `{Code:109,Error:"Insufficient Dcrm Funds For Value + Fee."}`
	ErrInvalidAddrToLO  = `{Code:110,Error:"Lock Out To Invalid Address."}`
	ErrLOToSelf  = `{Code:111,Error:"can not lockout to yourself."}`
	ErrInvalidTx  = `{Code:112,Error:"tx data invalid."}`
	ErrHashKeyMiss  = `{Code:113,Error:"hash key and real dcrm from is miss."}`
	
	//TODO
	ErrGetTradeUnitFail  = `{Code:114,Error:"get trade unit fail."}`
	ErrCalcOrderBalance  = `{Code:115,Error:"calc balance error."}`
	
	ErrFromNotFusionAccount  = `{Code:116,Error:"From Must Be Fusion Account In LockOut Tx."}`
	ErrReqAddrInsufficient  = `{Code:117,Error:"Insufficient for req addr,need least 5 fsn."}`
	ErrAddNewTradeInsufficient  = `{Code:118,Error:"Insufficient for add new trade,need least 5 fsn."}`
)

type ErrorRet struct {
    Code int
    Error string
}

func GetRetErrJsonStr(code int,err string) string {
    m := &ErrorRet{Code:code,Error:err}
    ret,_ := json.Marshal(m)
    return string(ret)
}

func GetRetErr(err string) error {
    var ret2 Err
    ret2.Info = err
    return ret2
}

