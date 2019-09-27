package dao

import (
	"github.com/fusion/go-fusion/cmd/eos_tracker/utils"
	"fmt"
	"log"
	"math/big"
)

func GetBalance(userKey string) string {
	balance := Get(userKey)
	if balance == "" {
		return "0"
	}
	return balance
}

func UpdateBalance(userKey string, balanceChange string) error {
	if !utils.IsUserKey(userKey) {
		return fmt.Errorf("invalid user key")
	}
	balance := GetBalance(userKey)
	bal, _ := new(big.Int).SetString(balance, 10)
	balChange, _ := new(big.Int).SetString(balanceChange, 10)
	newBal := new(big.Int).Add(bal, balChange)
log.Printf("bal is %v, balanceChange is %v, newBal is %v\n", bal, balChange, newBal)
	if newBal.Cmp(big.NewInt(0)) < 0 {
		log.Printf("insufficient balance: %v\n", userKey)
	}
	Put(userKey, newBal.String())
	return nil
}

func Deposit(userKey string, amount *big.Int) error {
	return UpdateBalance(userKey, amount.String())
}

func Withdraw(userKey string, amount *big.Int) error {
	return UpdateBalance(userKey, "-"+amount.String())
}
