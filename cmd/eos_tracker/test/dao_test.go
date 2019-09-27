package test

import (
	"github.com/fusion/go-fusion/cmd/eos_tracker/dao"
	"fmt"
	"testing"
)

func TestDao(t *testing.T) {
	balance := dao.GetBalance("wangxiaoming")
	fmt.Printf("wangxiaoming's balance is %v\n\n", balance)
	balanceChange := "10086"
	err := dao.UpdateBalance("wangxiaoming", balanceChange)
	if err != nil {
		panic(err)
	}

	newBalance := dao.GetBalance("wangxiaoming")
	fmt.Printf("wangxiaoming's new balance is %v\n\n", newBalance)

	balanceChange = "-173000"
	err = dao.UpdateBalance("wangxiaoming", balanceChange)
	if err != nil {
		panic(err)
	}

	newBalance = dao.GetBalance("wangxiaoming")
	fmt.Printf("wangxiaoming's new balance is %v\n\n", newBalance)
}
