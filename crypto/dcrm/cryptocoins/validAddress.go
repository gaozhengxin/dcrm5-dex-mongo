package cryptocoins

import (
	"regexp"
	"strings"
	"github.com/fusion/go-fusion/log"
)

var RegExpmap map[string]string = map[string]string {
	"BTC":"^(1|3|m|n|2)[a-zA-Z\\d]{25,33}$",
	"USDT":"^(1|3|m|n|2)[a-zA-Z\\d]{25,33}$",
	"BCH":"^(bchtest:)?(p|q)[0-9a-z]{41}$",
	"TRX":"",
	"ETH":"^(0x)?[0-9a-fA-F]{40}$",
	"XRP":"^r[1-9a-km-zA-HJ-NP-Z]{32,33}$",
	"EOSDCRM":"^d[1-5a-z]{32,33}$",
	//"EOS":"^(d[1-5a-z]{32,33})|([1-5a-z]{12})$",
	"EOS":"^([a-z\\d\\.]+)$",
	"EOS_NORMAL":"^([1-5a-z]{12})$",
	"ATOM":"^cosmos1[qpzry9x8gf2tvdw0s3jn54khce6mua7l]{38}$",
	"EVT":"^EVT[a-zA-Z\\d]{50}$",
	"BNB":"^(t)?(bnb1)[qpzry9x8gf2tvdw0s3jn54khce6mua7l]{38}$",
}

type AddressValidator struct {
	Exp string
}

func NewAddressValidator (cointype string) *AddressValidator {
	if strings.HasPrefix(cointype,"ERC20") {
		cointype = "ETH"
	}
	if strings.HasPrefix(cointype,"OMNI") {
		cointype = "BTC"
	}
	if strings.HasPrefix(cointype,"EVT") {
		cointype = "EVT"
	}
	if strings.HasPrefix(cointype,"BEP2") {
		cointype = "BNB"
	}
	return &AddressValidator{
		Exp: RegExpmap[cointype],
	}
}

func NewDcrmAddressValidator (cointype string) *AddressValidator {
	if cointype == "EOS" {
		cointype = "EOSDCRM"
	}
	return NewAddressValidator(cointype)
}

func (v *AddressValidator) IsValidAddress (address string) bool {
	match, _ := regexp.MatchString(v.Exp, address)
	log.Debug("AddressValidator.IsValidAddress","v.Exp",v.Exp,"address",address,"match",match)
	return match
}
