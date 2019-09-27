curl --request POST --url https://api-kylin.eoslaomao.com/v1/history/get_actions --header 'content-type: application/json; charset=UTF-8' -d '{"pos":10,"offset":9,"account_name":"gzx123454321"}'

# pos: 开始的account_action_seq，-1表示从最新trace向前找
# offset: 向后查找的trace数-1

# 遍历方式: {"pos":0,"offset":19}, {"pos":20,"offset":19}, {"pos":40,"offset":19}, ...
