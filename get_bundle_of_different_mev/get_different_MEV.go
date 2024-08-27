package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// 提取我们需要的数据
type CleanMEVTx struct {
	BlockNum         uint64
	Hash             common.Hash
	MEVType          string
	Protocol         string
	UserSwapCnt      int
	ExtractorSwapCnt int
	From             common.Address
	To               common.Address
}

// 单笔arbitrage交易
type Arbitrage struct {
	Tx CleanMEVTx
}

// 单笔liquidation交易
type Liquidation struct {
	Tx CleanMEVTx
}

type Sandwich struct {
	FrontRun CleanMEVTx
	VictimTx []CleanMEVTx
	BackRun  CleanMEVTx
}

var INPUT_PATH = "./output/"
var OUTPUT_PATH = "./output/"

var CLEANMEVTX_LIST []CleanMEVTx

//func OutputJSON(path string, )

func GetArbitrage(output_file_name string) {
	var arb_list = make([]Arbitrage, 0)
	for _, tx := range CLEANMEVTX_LIST {
		if tx.MEVType == "arb" { //如果标签是arb
			arb_list = append(arb_list, Arbitrage{Tx: tx}) //加入列表
		}
	}
	fmt.Println("Arbitrage List Length:", len(arb_list))

	// 输出成json格式
	jsonData, err := json.MarshalIndent(arb_list, "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + output_file_name + "_arb.json")
	check(err)
	output.Write(jsonData)
}

func GetLiquidation(output_file_name string) {
	var liquid_list = make([]Liquidation, 0)
	for _, tx := range CLEANMEVTX_LIST {
		if tx.MEVType == "liquid" { //如果标签是liquid
			liquid_list = append(liquid_list, Liquidation{Tx: tx}) //加入列表
		}
	}
	fmt.Println("Liquidation List Length:", len(liquid_list))

	// 输出成json格式
	jsonData, err := json.MarshalIndent(liquid_list, "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + output_file_name + "_liquid.json")
	check(err)
	output.Write(jsonData)
}

// 我们只关注传统sandwich，即frontrun-sandwich-backrun结构，多frontrun和多backrun我们不考虑
func GetSandwich(output_file_name string) {
	var sandwich_list = make([]Sandwich, 0)
	for i := 0; i < len(CLEANMEVTX_LIST); i += 0 {
		if CLEANMEVTX_LIST[i].MEVType == "frontrun" { //一个sandwich MEV开始
			sandwich := Sandwich{FrontRun: CLEANMEVTX_LIST[i], VictimTx: make([]CleanMEVTx, 0)}
			i += 1
			for { //循环读取受害交易
				if CLEANMEVTX_LIST[i].MEVType == "backrun" { //一个sandwich MEV结束
					sandwich.BackRun = CLEANMEVTX_LIST[i]
					i += 1
					sandwich_list = append(sandwich_list, sandwich) //正常结束获取到一个传统sandwich交易
					break
				} else if CLEANMEVTX_LIST[i].MEVType == "sandwich" {
					sandwich.VictimTx = append(sandwich.VictimTx, CLEANMEVTX_LIST[i])
					i += 1
				} else if CLEANMEVTX_LIST[i].MEVType == "frontrun" { //非传统sandwich交易
					fmt.Println("Found multi-frontrun at", CLEANMEVTX_LIST[i].BlockNum)
					//i 不加一，跳出循环
					break
					//panic("Error Frontrun position")
				} else { //一些不是受害交易的交易夹在中间则跳过
					i += 1
				}
			}
		} else if CLEANMEVTX_LIST[i].MEVType == "backrun" {
			fmt.Println("Found multi-backrun at", CLEANMEVTX_LIST[i].BlockNum)
			i += 1 //jump
			//panic("Error Backrun position")
		} else {
			i += 1
		}

	}
	fmt.Println("Sandwich List Length:", len(sandwich_list))

	// // 输出成json格式
	// jsonData, err := json.MarshalIndent(sandwich_list, "", "  ")
	// check(err)
	// //fmt.Println(string(jsonData))
	// output, err := os.Create(OUTPUT_PATH + output_file_name + "_sandwich.json")
	// check(err)
	// output.Write(jsonData)

}

func main() {
	file_name := os.Args[1]

	// 读取文件到DATA
	input, err := os.Open(INPUT_PATH + file_name + ".json")
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	err = json.Unmarshal([]byte(input_bytes), &CLEANMEVTX_LIST)
	check(err)
	fmt.Println("CLEANMEVTX_LIST length:", len(CLEANMEVTX_LIST))

	//GetArbitrage(file_name)
	//GetLiquidation(file_name)
	GetSandwich(file_name)
}

// go run get_different_MEV.go 19731000_20350000_clean
