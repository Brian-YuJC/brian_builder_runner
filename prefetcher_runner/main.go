package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/blobpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/holiman/uint256"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type TxPoolData struct {
	Data map[string]map[common.Address]map[int]TransactionArgs
}

type ResData struct {
	Id      int                                                   `json:"id"`
	Jsonrpc string                                                `json:"jsonrpc"`
	Result  map[string]map[common.Address]map[int]TransactionArgs `json:"result"`
}

var TestBlockChainCacheConfig = &core.CacheConfig{
	TrieCleanLimit:  256,
	TrieDirtyLimit:  256,
	TrieTimeLimit:   5 * time.Minute,
	SnapshotLimit:   256,
	SnapshotWait:    true,
	StateScheme:     rawdb.HashScheme,
	SnapshotNoBuild: true,
}

// 读取disk中的数据库并建立区块链
func GetBlockChain() (ethdb.Database, *core.BlockChain) {
	//datadir := "/home/user/common/docker/volumes/eth-docker_geth-eth1-data/_data/geth/chaindata"
	datadir := "/home/user/common/docker/volumes/cp1_eth-docker_geth-eth1-data/_data/geth/chaindata"
	ancient := datadir + "/ancient"
	db, err := rawdb.Open(
		rawdb.OpenOptions{
			Directory:         datadir,
			AncientsDirectory: ancient,
			Ephemeral:         true,
			ReadOnly:          true,
		},
	)
	if err != nil {
		fmt.Println("rawdb.Open err!", err)
	} else {
		fmt.Println("Open Database Success!")
		// head_block_hash := common.Hash{}
		//default: 0x0000000000000000000000000000000000000000000000000000000000000000
		// head_block_hash.SetBytes([]byte("0xae6ddd150fb4278af16832bc91f34a81a2aafefde30fecf8da4cdcbcd8ec331f"))
		// rawdb.WriteHeadBlockHash(db, head_block_hash)
		// fmt.Println("Set Head Block Hash!")
		fmt.Println(rawdb.ReadStateScheme(db))
	}

	bc, err := core.NewBlockChain(db, TestBlockChainCacheConfig, nil, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	check(err)
	fmt.Println("New Block Chain Success!")

	return db, bc
}

// 测试txpool的功能
// bc所在区块链，update是否将txpool最新的交易加入池中
// 注意获取到的tx会保存在本地
func GetTxPool(bc *core.BlockChain, update bool) *txpool.TxPool {
	//新建txpool
	blobPool := blobpool.New(blobpool.DefaultConfig, bc)
	legacyPool := legacypool.New(legacypool.DefaultConfig, bc)
	//sbundlePool := txpool.NewSBundlePool(params.MainnetChainConfig)
	tx_pool, err := txpool.New(legacypool.DefaultConfig.PriceLimit, bc, []txpool.SubPool{legacyPool, blobPool})
	check(err)

	//需要添加最新pool中的tx
	if update {
		//获取最新的txpool交易，并添加进我们的实验交易池
		txpool_data := GetTxInPool().Data //调用接口
		var txs []*types.Transaction
		for /*type_*/ _, addr_map := range txpool_data { //pending, basefee, queued
			//fmt.Fprintln(os.Stderr, "Type", type_)
			for /*addr*/ _, tx_map := range addr_map {
				//fmt.Fprintln(os.Stderr, "\tSender address", addr)
				for /*nonce*/ _, tx := range tx_map {
					//fmt.Fprintln(os.Stderr, "\t\tNonce:", nonce)
					// fmt.Fprintln(os.Stderr, "\t\t\t",
					// 	"From:", tx.From,
					// 	"To:", tx.To,
					// 	"Gas:", tx.Gas,
					// 	"GasPrice:", tx.GasPrice,
					// 	"MaxFeePerGas:", tx.MaxFeePerGas,
					// 	"MaxPriorityFeePerGas:", tx.MaxPriorityFeePerGas,
					// 	"Value:", tx.Value)
					txs = append(txs, tx.toTransaction())
				}
			}
		}
		fmt.Fprintln(os.Stderr, "Total tx in txpool data:", len(txs))

		//添加进交易池
		err_list := tx_pool.Add(txs, true, false, false)
		fmt.Fprintln(os.Stderr, "Fail to add into pool:", len(err_list))
		for _, err := range err_list { //打印错误信息
			fmt.Println(err)
		}
	}

	//获取pending的tx
	pending := tx_pool.Pending(txpool.PendingFilter{
		MinTip: uint256.MustFromBig(big.NewInt(0)),
	})
	fmt.Println("Total Tx in Pool:", len(pending))

	return tx_pool
}

// 调用RPC的txpool_content接口获取最新的txpool中的tx信息
func GetTxInPool() TxPoolData {
	// 要发送的请求数据
	requestData := map[string]interface{}{
		"method":  "txpool_content",
		"id":      1,
		"jsonrpc": "2.0",
	}
	// 将数据编码为 JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}
	// 发送 POST 请求
	response, err := http.Post("https://rpc.ankr.com/eth/7e0d2f6412f9595e078416601c04d718fa9695af283f70759f199a21f80e19f8", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Error sending POST request: %v", err)
	}
	defer response.Body.Close()
	// 检查响应状态
	if response.StatusCode != http.StatusOK {
		log.Fatalf("Error: received status code %d", response.StatusCode)
	}
	// 处理响应
	var responseBody ResData //解码成ResData格式的结构体
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		log.Fatalf("Error decoding response: %v", err)
	}
	return TxPoolData{Data: responseBody.Result} //返回TxPoolData结构体数据
}

// // ！！！！！非常危险的操作！！！！！
// 用于rawdb在检测数据库类型时rawdb.ReadStateScheme(db)将原本的path检测为hash
// 该函数主要检测两个key的位置一个是key为“A”的位置表示AccountTrie的根，和“LastStateID”表示当前状态id
// func DeletePathDB(db ethdb.Database) {
//  //强制删除AccountTrie的根的数据
// 	SavePathDBParam(db)
// 	db.Delete([]byte("A"))
// 	//强制重置stateID为0
// 	var id uint64 = 0
// 	buf := make([]byte, 8)
// 	binary.BigEndian.PutUint64(buf, id)
// 	db.Put([]byte("LastStateID"), buf)
// 	db.Close()
// }

// // 保存PathDB重要的数据
// 用于DeletePathDB时先保存数据
// func SavePathDBParam(db ethdb.Database) {
// 	path := "./pathdb.log"
// 	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
// 	check(err)
// 	defer output.Close()
// 	state_id_byte, err := db.Get([]byte("LastStateID"))
// 	check(err)
// 	state_id := hex.EncodeToString(state_id_byte)
// 	account_trie_root_byte, err := db.Get(append([]byte("A"), nil...))
// 	account_trie_root := hex.EncodeToString(account_trie_root_byte)
// 	check(err)
// 	_, err = output.WriteString("LastStateID " + state_id)
// 	check(err)
// 	_, err = output.WriteString("AccountTrieRoot " + account_trie_root)
// 	check(err)
// }

// // 根据哈希获取block数据
// // 目的是为了获取已经打包的tx进行实验
// func GetTxByHash(tx_hash common.Hash) {
// 	fmt.Println("Get", tx_hash.Hex(), "Hash")
// 	// 要发送的请求数据
// 	requestData := map[string]interface{}{
// 		"method":  "eth_getTransactionByHash",
// 		"params":  []string{tx_hash.Hex()},
// 		"id":      1,
// 		"jsonrpc": "2.0",
// 	}
// 	// 将数据编码为 JSON
// 	jsonData, err := json.Marshal(requestData)
// 	if err != nil {
// 		log.Fatalf("Error encoding JSON: %v", err)
// 	}
// 	// 发送 POST 请求
// 	// http://10.119.187.21:8545
// 	// https://rpc.ankr.com/eth/7e0d2f6412f9595e078416601c04d718fa9695af283f70759f199a21f80e19f8
// 	response, err := http.Post("http://10.119.187.21:8545", "application/json", bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		log.Fatalf("Error sending POST request: %v", err)
// 	}
// 	defer response.Body.Close()
// 	// 检查响应状态
// 	if response.StatusCode != http.StatusOK {
// 		log.Fatalf("Error: received status code %d", response.StatusCode)
// 	}
// 	// 处理响应
// 	var responseBody map[string]interface{}
// 	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
// 		log.Fatalf("Error decoding response: %v", err)
// 	}
// 	fmt.Println(responseBody)
// }

func main() {
	_, bc := GetBlockChain()

	//SavePathDBParam(db)
	//DeletePathDB(db)

	// //获取当前状态stateDB
	// start_block_num := uint64(9900000)
	// start_block_hash := rawdb.ReadCanonicalHash(db, start_block_num)
	// start_block := rawdb.ReadBlock(db, start_block_hash, start_block_num)
	// //_, err := bc.StateAt(start_block.Root())
	// //header := start_block.Header()
	// //fmt.Println(start_block.Hash())
	// //check(err)
	// for _, tx := range start_block.Transactions() {
	// 	GetTxByHash(tx.Hash())
	// }

	// header := bc.CurrentBlock()
	// fmt.Println("Current Header:", header.Number)
	// // ⭐️TODO:现在的问题是数据库的current block是第0个块，导致gas limit只有5000，导致当前最新的txpool的交易没法放进池里
	tx_pool := GetTxPool(bc, false)

}

//---------------------------------------------------------以下为Geth中的Internal结构体和函数这里拿出来调用----------------------------------------------------
//有些结构体和函数更改过
// // RPCTransaction represents a transaction that will serialize to the RPC representation of a transaction
// type RPCTransaction struct {
// 	BlockHash   *common.Hash   `json:"blockHash"`
// 	BlockNumber *hexutil.Big   `json:"blockNumber"`
// 	From        common.Address `json:"from"`
// 	Gas         hexutil.Uint64 `json:"gas"`
// 	GasPrice    *hexutil.Big   `json:"gasPrice"`
// 	//GasFeeCap           *hexutil.Big      `json:"maxFeePerGas,omitempty"`
// 	GasTipCap *hexutil.Big `json:"maxPriorityFeePerGas,omitempty"`
// 	//MaxFeePerBlobGas    *hexutil.Big      `json:"maxFeePerBlobGas,omitempty"`
// 	Hash             common.Hash       `json:"hash"`
// 	Input            hexutil.Bytes     `json:"input"`
// 	Nonce            hexutil.Uint64    `json:"nonce"`
// 	To               *common.Address   `json:"to"`
// 	TransactionIndex *hexutil.Uint64   `json:"transactionIndex"`
// 	Value            *hexutil.Big      `json:"value"`
// 	Type             hexutil.Uint64    `json:"type"`
// 	Accesses         *types.AccessList `json:"accessList,omitempty"`
// 	ChainID          *hexutil.Big      `json:"chainId,omitempty"`
// 	//BlobVersionedHashes []common.Hash     `json:"blobVersionedHashes,omitempty"`
// 	V *hexutil.Big `json:"v"`
// 	R *hexutil.Big `json:"r"`
// 	S *hexutil.Big `json:"s"`
// 	//YParity             *hexutil.Uint64   `json:"yParity,omitempty"`
// }

// TransactionArgs represents the arguments to construct a new transaction
// or a message call.
type TransactionArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Nonce                *hexutil.Uint64 `json:"nonce"`

	// We accept "data" and "input" for backwards-compatibility reasons.
	// "input" is the newer name and should be preferred by clients.
	// Issue detail: https://github.com/ethereum/go-ethereum/issues/15628
	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input"`

	// Introduced by AccessListTxType transaction.
	AccessList *types.AccessList `json:"accessList,omitempty"`
	ChainID    *hexutil.Big      `json:"chainId,omitempty"`

	// For BlobTxType
	BlobFeeCap *hexutil.Big  `json:"maxFeePerBlobGas"`
	BlobHashes []common.Hash `json:"blobVersionedHashes,omitempty"`

	// For BlobTxType transactions with blob sidecar
	Blobs       []kzg4844.Blob       `json:"blobs"`
	Commitments []kzg4844.Commitment `json:"commitments"`
	Proofs      []kzg4844.Proof      `json:"proofs"`

	V *hexutil.Big `json:"v"` //Brian Add
	R *hexutil.Big `json:"r"` //Brian Add
	S *hexutil.Big `json:"s"` //Brian Add

	// This configures whether blobs are allowed to be passed.
	blobSidecarAllowed bool
}

// toTransaction converts the arguments to a transaction.
// This assumes that setDefaults has been called.
func (args *TransactionArgs) toTransaction() *types.Transaction {
	var data types.TxData
	switch {
	case args.BlobHashes != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		data = &types.BlobTx{
			To:         *args.To,
			ChainID:    uint256.MustFromBig((*big.Int)(args.ChainID)),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasFeeCap:  uint256.MustFromBig((*big.Int)(args.MaxFeePerGas)),
			GasTipCap:  uint256.MustFromBig((*big.Int)(args.MaxPriorityFeePerGas)),
			Value:      uint256.MustFromBig((*big.Int)(args.Value)),
			Data:       args.data(),
			AccessList: al,
			BlobHashes: args.BlobHashes,
			BlobFeeCap: uint256.MustFromBig((*big.Int)(args.BlobFeeCap)),
			V:          new(uint256.Int).SetBytes((*big.Int)(args.V).Bytes()), //Brian Add
			R:          new(uint256.Int).SetBytes((*big.Int)(args.R).Bytes()), //Brian Add
			S:          new(uint256.Int).SetBytes((*big.Int)(args.S).Bytes()), //Brian Add
		}
		if args.Blobs != nil {
			data.(*types.BlobTx).Sidecar = &types.BlobTxSidecar{
				Blobs:       args.Blobs,
				Commitments: args.Commitments,
				Proofs:      args.Proofs,
			}
		}

	case args.MaxFeePerGas != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		data = &types.DynamicFeeTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasFeeCap:  (*big.Int)(args.MaxFeePerGas),
			GasTipCap:  (*big.Int)(args.MaxPriorityFeePerGas),
			Value:      (*big.Int)(args.Value),
			Data:       args.data(),
			AccessList: al,
			V:          (*big.Int)(args.V), //Brian Add
			R:          (*big.Int)(args.R), //Brian Add
			S:          (*big.Int)(args.S), //Brian Add
		}

	case args.AccessList != nil:
		data = &types.AccessListTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasPrice:   (*big.Int)(args.GasPrice),
			Value:      (*big.Int)(args.Value),
			Data:       args.data(),
			AccessList: *args.AccessList,
			V:          (*big.Int)(args.V), //Brian Add
			R:          (*big.Int)(args.R), //Brian Add
			S:          (*big.Int)(args.S), //Brian Add
		}

	default:
		data = &types.LegacyTx{
			To:       args.To,
			Nonce:    uint64(*args.Nonce),
			Gas:      uint64(*args.Gas),
			GasPrice: (*big.Int)(args.GasPrice),
			Value:    (*big.Int)(args.Value),
			Data:     args.data(),
			V:        (*big.Int)(args.V), //Brian Add
			R:        (*big.Int)(args.R), //Brian Add
			S:        (*big.Int)(args.S), //Brian Add
		}
		//fmt.Println("New Tx data:", args.To, uint64(*args.Nonce), uint64(*args.Gas), (args.GasPrice), args.Value, args.data()) // Brian Add
	}

	return types.NewTx(data)
}

// data retrieves the transaction calldata. Input field is preferred.
func (args *TransactionArgs) data() []byte {
	if args.Input != nil {
		return *args.Input
	}
	if args.Data != nil {
		return *args.Data
	}
	return nil
}
