

组内节点默认3个，均为挖矿节点。 即启动命令中得加--miner，并且得用比如 clique.propose("0x998fc96bfb1ed0f990c69004224a6b005d8eef29",true) 命令进行授权。

新增api：

1.  func (s *PublicFsnAPI) XproOrderCreate(ctx context.Context,trade string,ordertype string,side string,price string,quantity string,rule string) (common.Hash, error)

该函数实现了下单功能。

trade为交易对 目前仅支持“ETH/BTC”
side为“Buy” 或者 “Sell”

ordertype为订单类型 目前仅仅支持"LimitOrder"

price为订单价格，目前默认支持16位小数精度。

quantity为购买数量，目前默认支持16位小数精度。
rule为订单时效性。目前仅仅支持“GTE”

使用举例：  
终端1
lilo.xproOrderCreate("ETH/BTC","LimitOrder","Buy","10.01","0.01","GTE")
lilo.xproOrderCreate("ETH/BTC","LimitOrder","Buy","10.00","0.02","GTE")
终端2
lilo.xproOrderCreate("ETH/BTC","LimitOrder","Sell","9.98","0.01","GTE")
lilo.xproOrderCreate("ETH/BTC","LimitOrder","Sell","9.97","0.01","GTE")


2. func (s *PublicFsnAPI) XproOrderStatus(ctx context.Context,trade string) (string, error)
该函数获取当前某个交易对的未成交订单集合，以json格式返回。

例如： lilo.xproOrderStatus("ETH/BTC")

3. func (s *PublicFsnAPI) XproOrderDealedInfo(ctx context.Context,blockNr rpc.BlockNumber,trade string) (string, error)
该函数获取当前最新区块中某个交易对的已经成交的订单集合，以json格式返回。

blockNr为要查询的块高度,该值要么为"latest"或者"pending",要么为0x开头的16进制数.

例如： lilo.xproOrderDealedInfo("ETH/BTC")

4. func (s *PublicFsnAPI) GetPairList(ctx context.Context) (string, error)
该函数获取目前支持的所有交易对。

例如：lilo.getPairList()





