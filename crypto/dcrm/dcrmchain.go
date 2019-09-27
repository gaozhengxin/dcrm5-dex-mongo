// Copyright 2018 The fusion-dcrm 
//Author: caihaijun@fusion.org

package dcrm 

import (
	"container/list"
	"strings"
)

var (
    DC *list.List
)

func init() {
    DC = list.New()
}

type KVData struct {
    Key string
    Value string
}

type DcrmChain struct {
    //key: lockout tx hash
    //value: lockout_txhash + sep10 + realfusionfrom + sep10 + realdcrmfrom
    Lo_info *KVData
    
    //key:  kec256(fusionaddr:cointype)
    //value: dcrmaddr1:dcrmaddr2:dcrmaddr3............
    Da *KVData
    
    //key:  dcrmaddr or pubkey
    //value:  fusionaddr + sep + xxx + sep + ys + sep + save + sep + kec256(fusionaddr:cointype)
    Dc_info *KVData

    Status string

    Txhash string
}

func NewDcrmChain() (*DcrmChain,error) {
    return &DcrmChain{
	    Lo_info:    nil,
	    Da:    nil,
	    Dc_info:    nil,
	    Txhash:  "",
    },nil
}

func SetTxStatus(hash string,status string) {

    if finddc(hash) {
	return
    }

    dc := &DcrmChain{Lo_info:nil,Da:nil,Dc_info:nil,Status:status,Txhash:hash}
    DC.PushBack(dc)
}

func GetTxStatus(hash string) string {
    iter := DC.Front()
    for iter != nil {
	dc := iter.Value.(*DcrmChain)
	if dc == nil {
	    continue
	}

	if strings.EqualFold(dc.Txhash,hash) {
	    return dc.Status
	}

	iter = iter.Next()
    }

    return "Unknown Status."
}

