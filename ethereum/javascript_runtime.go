package main

import (
	"fmt"
	"github.com/ethereum/eth-go"
	"github.com/ethereum/eth-go/ethchain"
	"github.com/ethereum/eth-go/ethpub"
	"github.com/ethereum/eth-go/ethutil"
	"github.com/robertkrimen/otto"
)

type JSRE struct {
	ethereum *eth.Ethereum
	vm       *otto.Otto
	lib      *ethpub.PEthereum

	blockChan  chan ethutil.React
	changeChan chan ethutil.React
	quitChan   chan bool

	objectCb map[string][]otto.Value
}

func NewJSRE(ethereum *eth.Ethereum) *JSRE {
	re := &JSRE{
		ethereum,
		otto.New(),
		ethpub.NewPEthereum(ethereum),
		make(chan ethutil.React, 1),
		make(chan ethutil.React, 1),
		make(chan bool),
		make(map[string][]otto.Value),
	}

	// Init the JS lib
	re.vm.Run(jsLib)

	// We have to make sure that, whoever calls this, calls "Stop"
	go re.mainLoop()

	re.Bind("eth", &JSEthereum{re.lib, re.vm})

	re.initStdFuncs()

	return re
}

func (self *JSRE) Bind(name string, v interface{}) {
	self.vm.Set(name, v)
}

func (self *JSRE) Run(code string) (otto.Value, error) {
	return self.vm.Run(code)
}

func (self *JSRE) Stop() {
	// Kill the main loop
	self.quitChan <- true

	close(self.blockChan)
	close(self.quitChan)
	close(self.changeChan)
}

func (self *JSRE) mainLoop() {
	// Subscribe to events
	reactor := self.ethereum.Reactor()
	reactor.Subscribe("newBlock", self.blockChan)

out:
	for {
		select {
		case <-self.quitChan:
			break out
		case block := <-self.blockChan:
			if _, ok := block.Resource.(*ethchain.Block); ok {
			}
		case object := <-self.changeChan:
			if stateObject, ok := object.Resource.(*ethchain.StateObject); ok {
				for _, cb := range self.objectCb[ethutil.Hex(stateObject.Address())] {
					val, _ := self.vm.ToValue(ethpub.NewPStateObject(stateObject))
					cb.Call(cb, val)
				}
			} else if storageObject, ok := object.Resource.(*ethchain.StorageState); ok {
				fmt.Println(storageObject)
			}
		}
	}
}

func (self *JSRE) initStdFuncs() {
	t, _ := self.vm.Get("eth")
	eth := t.Object()
	eth.Set("watch", func(call otto.FunctionCall) otto.Value {
		addr, _ := call.Argument(0).ToString()
		cb := call.Argument(1)

		self.objectCb[addr] = append(self.objectCb[addr], cb)

		event := "object:" + string(ethutil.FromHex(addr))
		self.ethereum.Reactor().Subscribe(event, self.changeChan)

		return otto.UndefinedValue()
	})
	eth.Set("addPeer", func(call otto.FunctionCall) otto.Value {
		host, err := call.Argument(0).ToString()
		if err != nil {
			return otto.FalseValue()
		}
		self.ethereum.ConnectToPeer(host)

		return otto.TrueValue()
	})
}