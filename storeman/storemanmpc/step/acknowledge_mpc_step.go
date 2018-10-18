package step

import (
	"github.com/wanchain/go-wanchain/log"
	mpcprotocol "github.com/wanchain/go-wanchain/storeman/storemanmpc/protocol"
	mpcsyslog "github.com/wanchain/go-wanchain/storeman/syslog"
	"math/big"
)

type AcknowledgeMpcStep struct {
	BaseStep
	messageType int64
}

func (ack *AcknowledgeMpcStep) InitStep(mpcprotocol.MpcResultInterface) error {
	return nil
}

func CreateAcknowledgeMpcStep(peers *[]mpcprotocol.PeerInfo, messageType int64) *AcknowledgeMpcStep {
	log.Info("CreateAcknowledgeMpcStep begin")
	mpcsyslog.Info("CreateAcknowledgeMpcStep begin")

	return &AcknowledgeMpcStep{*CreateBaseStep(peers, 0), messageType}
}

func (ack *AcknowledgeMpcStep) CreateMessage() []mpcprotocol.StepMessage {
	log.Info("AcknowledgeMpcStep.CreateMessage begin")
	mpcsyslog.Info("AcknowledgeMpcStep.CreateMessage begin")

	data := make([]big.Int, 1)
	data[0].SetInt64(ack.messageType)
	return []mpcprotocol.StepMessage{mpcprotocol.StepMessage{
		Msgcode:mpcprotocol.MPCMessage,
		PeerID:nil,
		Peers:nil,
		Data:data,
		BytesData:nil}}
}

func (ack *AcknowledgeMpcStep) FinishStep(result mpcprotocol.MpcResultInterface, mpc mpcprotocol.StoremanManager) error {
	log.Info("AcknowledgeMpcStep.FinishStep begin")
	mpcsyslog.Info("AcknowledgeMpcStep.FinishStep begin")

	err := ack.BaseStep.FinishStep()
	if err != nil {
		return err
	}

	data := make([]big.Int, 1)
	data[0].SetInt64(ack.messageType)
	result.SetValue(mpcprotocol.MPCActoin, data)

	log.Info("AcknowledgeMpcStep.FinishStep succeed")
	mpcsyslog.Info("AcknowledgeMpcStep.FinishStep succeed")
	return nil
}

func (ack *AcknowledgeMpcStep) HandleMessage(msg *mpcprotocol.StepMessage) bool {
	return true
}
