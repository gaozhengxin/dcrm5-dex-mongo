// Copyright 2018 The fusion-dcrm 
//Author: caihaijun@fusion.org

package core

import (
	"container/list"
)

var (
    DCC *list.List
)

func init() {
    DCC = list.New()
}

