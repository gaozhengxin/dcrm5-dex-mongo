package main

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

func main() {
    u1, _ := new(big.Int).SetString("42254905206629322958607187159043159719271763184953301425368004071884105975490",10) //从日志文件中获取  path: datadir+"/xxx-"+cur_enode,log.JSONFormat()
	u2, _ := new(big.Int).SetString("43491503642523465191224458729373190832208493117655010120162637749084004320736",10) //从日志文件中获取  path: datadir+"/xxx-"+cur_enode,log.JSONFormat()
	u3, _ := new(big.Int).SetString("106751321815559071640884309746757660325063981055444147463891709324109376621461",10) //从日志文件中获取  path: datadir+"/xxx-"+cur_enode,log.JSONFormat()
	u := new(big.Int).Add(u1, u2)
	u = new(big.Int).Add(u, u3)
	u = new(big.Int).Mod(u,secp256k1.S256().N)
	sk := hex.EncodeToString(u.Bytes())
	fmt.Printf("sk:\n%+v\n", sk)
}
