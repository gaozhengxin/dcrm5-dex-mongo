package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"github.com/fusion/go-fusion/common"
	"github.com/fusion/go-fusion/mongodb"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

func init() {
	mongodb.MongoInit()
	initCmd()
}

func initCmd() {
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 5050, "listening port")
}

var port int

var rootCmd = &cobra.Command{
	Run: func(cmd *cobra.Command, args []string) {
		p := strconv.Itoa(port)
		a := "localhost:"+p
		Serve(a)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func Serve(a string) {
	router := gin.Default()
	router.GET("txs/to/:address", HandleGetTxsToAddr)
	router.Run(a)
}

func HandleGetTxsToAddr(c *gin.Context) {
	address := c.Param("address")
	if address == "" {
		c.String(400, "param address not set")
		return
	}
	address = strings.ToLower(address)
	txs := mongodb.FindTransactionToAccount("Transactions", common.HexToAddress(address))
	res, err := json.Marshal(txs)
	if err == nil && res != nil {
		c.Data(200, "application/json", res)
		return
	} else if err != nil {
		c.String(500, err.Error())
		return
	} else {
		c.String(500, "unknown error")
		return
	}
}
