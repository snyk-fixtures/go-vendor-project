// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"encoding/hex"
	"fmt"
	"github.com/wanchain/go-wanchain/common/hexutil"
	"sync/atomic"

	"github.com/wanchain/go-wanchain/pos/util"

	"github.com/wanchain/go-wanchain/crypto"

	"github.com/wanchain/go-wanchain/pos/incentive"
	"github.com/wanchain/go-wanchain/pos/posconfig"
	"github.com/wanchain/go-wanchain/pos/randombeacon"

	"time"

	"github.com/wanchain/go-wanchain/accounts"
	"github.com/wanchain/go-wanchain/accounts/keystore"
	"github.com/wanchain/go-wanchain/common"
	"github.com/wanchain/go-wanchain/consensus"
	"github.com/wanchain/go-wanchain/core"
	"github.com/wanchain/go-wanchain/core/state"
	"github.com/wanchain/go-wanchain/core/types"
	"github.com/wanchain/go-wanchain/eth/downloader"
	"github.com/wanchain/go-wanchain/ethdb"
	"github.com/wanchain/go-wanchain/event"
	"github.com/wanchain/go-wanchain/log"
	"github.com/wanchain/go-wanchain/params"
	"github.com/wanchain/go-wanchain/pos/epochLeader"
	"github.com/wanchain/go-wanchain/pos/slotleader"
	"github.com/wanchain/go-wanchain/rpc"
)

// Backend wraps all methods required for mining.
type Backend interface {
	AccountManager() *accounts.Manager
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
	ChainDb() ethdb.Database
	Etherbase() (common.Address, error)
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux *event.TypeMux

	worker *worker

	coinbase common.Address
	mining   int32
	eth      Backend
	engine   consensus.Engine

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync
	timerStop   chan interface{}
}

func New(eth Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine) *Miner {
	miner := &Miner{
		eth:       eth,
		mux:       mux,
		engine:    engine,
		worker:    newWorker(config, engine, common.Address{}, eth, mux),
		canStart:  1,
		timerStop: make(chan interface{}),
	}
	miner.Register(NewCpuAgent(eth.BlockChain(), engine))
	//posInit(eth, nil)
	go miner.update()
	return miner
}

func PosInit(s Backend) *epochLeader.Epocher {
	log.Info("backendTimerLoop is running!!!!!!")
	g := s.BlockChain().GetHeaderByNumber(0)
	posconfig.GenesisPK = hexutil.Encode(g.Extra)[2:]
	slotleader.SlsInit()

	if posconfig.EpochBaseTime == 0 {
		h := s.BlockChain().GetHeaderByNumber(1)
		if nil != h {
			posconfig.EpochBaseTime = h.Time.Uint64()
		}
	}

	epochSelector := epochLeader.NewEpocher(s.BlockChain())


	eerr := epochSelector.SelectLeadersLoop(0)

	sls := slotleader.GetSlotLeaderSelection()
	sls.Init(s.BlockChain(), nil, nil)

	incentive.Init(epochSelector.GetEpochProbability, epochSelector.SetEpochIncentive, epochSelector.GetRBProposerGroup)
	fmt.Println("posInit: ", eerr)

	s.BlockChain().SetSlSelector(sls)
	s.BlockChain().SetRbSelector(epochSelector)

	s.BlockChain().SetSlotValidator(sls)

	return epochSelector
}
func posInitMiner(s Backend, key *keystore.Key) {
	log.Info("timer backendTimerLoop is running!!!!!!")

	// config
	if key != nil {
		posconfig.Cfg().MinerKey = key
	}
	epochSelector := epochLeader.NewEpocher(s.BlockChain())
	randombeacon.GetRandonBeaconInst().Init(epochSelector)
	if posconfig.EpochBaseTime == 0 {
		h := s.BlockChain().GetHeaderByNumber(1)
		if nil != h {
			posconfig.EpochBaseTime = h.Time.Uint64()
		}
	}
}

// backendTimerLoop is pos main time loop
func (self *Miner) backendTimerLoop(s Backend) {
	log.Info("backendTimerLoop is running!!!!!!")
	// get wallet
	eb, errb := s.Etherbase()
	if errb != nil {
		panic(errb)
	}
	wallet, errf := s.AccountManager().Find(accounts.Account{Address: eb})
	if wallet == nil || errf != nil {
		panic(errf)
	}
	type getKey interface {
		GetUnlockedKey(address common.Address) (*keystore.Key, error)
	}
	key, err := wallet.(getKey).GetUnlockedKey(eb)
	if key == nil || err != nil {
		panic(err)
	}
	log.Debug("Get unlocked key success address:" + eb.Hex())
	localPublicKey := hex.EncodeToString(crypto.FromECDSAPub(&key.PrivateKey.PublicKey))
	posInitMiner(s, key)
	// get rpcClient
	url := posconfig.Cfg().NodeCfg.IPCEndpoint()
	rc, err := rpc.Dial(url)
	if err != nil {
		fmt.Println("err:", err)
		panic(err)
	}

	for {
		// wait until block1
		h := s.BlockChain().GetHeaderByNumber(1)
		if nil == h {
			select {
			case <-self.timerStop:
				randombeacon.GetRandonBeaconInst().Stop()
				return
			case <-time.After(time.Duration(time.Second)):
				continue
			}

			continue
		} else {
			posconfig.EpochBaseTime = h.Time.Uint64()
			cur := uint64(time.Now().Unix())
			if cur < posconfig.EpochBaseTime+posconfig.SlotTime {
				time.Sleep(time.Duration((posconfig.EpochBaseTime + posconfig.SlotTime - cur)) * time.Second)
			}
		}

		util.CalEpochSlotIDByNow()
		epochid, slotid := util.GetEpochSlotID()
		log.Debug("get current period", "epochid", epochid, "slotid", slotid)

		slotleader.GetSlotLeaderSelection().Loop(rc, key, epochid, slotid)

		leaderPub, err := slotleader.GetSlotLeaderSelection().GetSlotLeader(epochid, slotid)
		if err == nil {
			leader := hex.EncodeToString(crypto.FromECDSAPub(leaderPub))
			if leader == localPublicKey {
				self.worker.chainSlotTimer <- struct{}{}
			}
		}

		// get state of k blocks ahead the last block
		lastBlockNum := s.BlockChain().CurrentBlock().NumberU64()
		root := s.BlockChain().GetBlockByNumber(lastBlockNum).Root()
		stateDb, err2 := s.BlockChain().StateAt(root)
		if err2 != nil {
			log.Error("Failed to get stateDb", "err", err2)
		}

		if stateDb != nil {
			randombeacon.GetRandonBeaconInst().Loop(stateDb, rc, epochid, slotid)
		}
		cur := uint64(time.Now().Unix())
		sleepTime := posconfig.SlotTime - (cur - posconfig.EpochBaseTime - (epochid*posconfig.SlotCount+slotid)*posconfig.SlotTime)
		log.Debug("timeloop sleep", "sleepTime", sleepTime)
		if sleepTime < 0 {
			sleepTime = 0
		}
		select {
		case <-self.timerStop:
			randombeacon.GetRandonBeaconInst().Stop()
			return
		case <-time.After(time.Duration(time.Second * time.Duration(sleepTime))):
			continue
		}
	}
	return
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (self *Miner) update() {
	events := self.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
out:
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case downloader.StartEvent:
			atomic.StoreInt32(&self.canStart, 0)
			if self.Mining() {
				self.Stop()
				atomic.StoreInt32(&self.shouldStart, 1)
				log.Info("Mining aborted due to sync")
			}
		case downloader.DoneEvent, downloader.FailedEvent:
			shouldStart := atomic.LoadInt32(&self.shouldStart) == 1

			atomic.StoreInt32(&self.canStart, 1)
			atomic.StoreInt32(&self.shouldStart, 0)
			if shouldStart {
				self.Start(self.coinbase)
			}
			// unsubscribe. we're only interested in this event once
			events.Unsubscribe()
			// stop immediately and ignore all further pending events
			break out
		}
	}
}

func (self *Miner) Start(coinbase common.Address) {
	atomic.StoreInt32(&self.shouldStart, 1)
	self.worker.setEtherbase(coinbase)
	self.coinbase = coinbase

	if atomic.LoadInt32(&self.canStart) == 0 {
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	atomic.StoreInt32(&self.mining, 1)

	log.Info("Starting mining operation")
	self.worker.start()
	self.worker.commitNewWork()
	if self.worker.config.Pluto != nil {
		go self.backendTimerLoop(self.eth)
	}
}

func (self *Miner) Stop() {
	self.worker.stop()
	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.shouldStart, 0)
	if self.worker.config.Pluto != nil {
		self.timerStop <- nil
	}
}

func (self *Miner) Register(agent Agent) {
	if self.Mining() {
		agent.Start()
	}
	self.worker.register(agent)
}

func (self *Miner) Unregister(agent Agent) {
	self.worker.unregister(agent)
}

func (self *Miner) Mining() bool {
	return atomic.LoadInt32(&self.mining) > 0
}

func (self *Miner) HashRate() (tot int64) {
	if pow, ok := self.engine.(consensus.PoW); ok {
		tot += int64(pow.Hashrate())
	}
	// do we care this might race? is it worth we're rewriting some
	// aspects of the worker/locking up agents so we can get an accurate
	// hashrate?
	for agent := range self.worker.agents {
		if _, ok := agent.(*CpuAgent); !ok {
			tot += agent.GetHashRate()
		}
	}
	return
}

func (self *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	self.worker.setExtra(extra)
	return nil
}

// Pending returns the currently pending block and associated state.
func (self *Miner) Pending() (*types.Block, *state.StateDB) {
	return self.worker.pending()
}

// PendingBlock returns the currently pending block.
//
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (self *Miner) PendingBlock() *types.Block {
	return self.worker.pendingBlock()
}

func (self *Miner) SetEtherbase(addr common.Address) {
	self.coinbase = addr
	self.worker.setEtherbase(addr)
}
