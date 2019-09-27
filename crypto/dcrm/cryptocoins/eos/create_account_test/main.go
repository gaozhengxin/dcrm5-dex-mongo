package main

import (
	"fmt"
	eos ".."
)

func main() {
	pub := "04c1a8dd2d6acd8891bddfc02bc4970a0569756ed19a2ed75515fa458e8cf979fdef6ebc5946e90a30c3ee2c1fadf4580edb1a57ad356efd7ce3f5c13c9bb4c78f"
	owner, _ := eos.HexToPubKey(pub)
	active, _ := eos.HexToPubKey(pub)
	//accountName := "wssbojbk2333"
	//accountName := "wssbojbk2334"
	//accountName := "wssbojbk2335"
	accountName := "wssbojbk2336"
	ojbk, err := eos.CreateNewAccount(eos.CREATOR_ACCOUNT,eos.CREATOR_PRIVKEY,accountName,owner.String(),active.String(),eos.InitialRam)
	if !ojbk {
		fmt.Printf("not ojbk, %v\n\n", err)
		return
	} else {
		fmt.Printf("ojbk\n")
	}

	ojbk2, err := eos.DelegateBW(eos.CREATOR_ACCOUNT,eos.CREATOR_PRIVKEY,accountName,eos.InitialCPU,eos.InitialStakeNet,true)
	if !ojbk2 {
		fmt.Printf("not ojbk, %v\n\n", err)
		return
	} else {
		fmt.Printf("ojbk\n")
	}
}
