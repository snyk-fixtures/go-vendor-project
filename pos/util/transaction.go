package	util 

import (
	"errors"
	"context"
	"github.com/wanchain/go-wanchain/rpc"
	"github.com/wanchain/go-wanchain/common"
	"github.com/wanchain/go-wanchain/log"
)

//type SendTxArgs struct {
//  From     common.Address  `json:"from"`
//  To       *common.Address `json:"to"`
//  Gas      *hexutil.Big    `json:"gas"`
//  GasPrice *hexutil.Big    `json:"gasPrice"`
//  Value    *hexutil.Big    `json:"value"`
//  Data     hexutil.Bytes   `json:"data"`
//  Nonce    *hexutil.Uint64 `json:"nonce"`
//}
func SendTx(rc *rpc.Client, tx map[string]interface{}) (common.Hash, error) {
	log.Info("begin send pos tx")
	if rc == nil {
		log.Error("connect rpc fail, rc is nil")
		return common.Hash{}, errors.New("rc is not ready")
	}

	ctx := context.Background()
	var txHash common.Hash
	err := rc.CallContext(ctx, &txHash, "eth_sendPosTransaction", tx)
	if nil != err {
		log.Error("send pos tx fail", "err", err)
		return common.Hash{}, err
	}

	log.Info("send pos tx success", "txHash", txHash)
	return txHash, nil
}

