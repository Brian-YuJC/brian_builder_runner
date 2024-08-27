package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

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

type Sandwich struct {
	FrontRun        CleanMEVTx
	VictimTx        []CleanMEVTx
	BackRun         CleanMEVTx
	TouchAddressMap map[common.Address]TouchAddress
}

type TouchAddress struct {
	//Address   common.Address
	InvokeCnt uint64
	LabelInfo LabelInfo
	// Label     []string
	// Tag       string //Accounr use only
	// Name      string //Token use only
	// Symbol    string //Token use only
	// Type      string //Account or Token
	KeyMap map[common.Hash]int
}

type TokenLabel struct {
	Address string `json:"address"`
	ChainID int    `json:"chainId"`
	Label   string `json:"label"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
}

type AccountLabel struct {
	Address string `json:"address"`
	ChainID int    `json:"chainId"`
	Label   string `json:"label"`
	Tag     string `json:"nameTag"`
}

type LabelInfo struct {
	Label  []string
	Tag    string //Accounr use only
	Name   string //Token use only
	Symbol string //Token use only
	Type   string //Account or Token
}

var LABEL_MAP = make(map[string]LabelInfo)

func ReadLabel(file_name string, type_ string) {
	input, err := os.Open("./" + file_name + ".json")
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	var token_label_list = make([]TokenLabel, 0)
	var account_label_list = make([]AccountLabel, 0)

	if type_ == "Token" {
		err = json.Unmarshal([]byte(input_bytes), &token_label_list)
		check(err)
		for _, label := range token_label_list {
			label.Address = strings.ToLower(label.Address) //转小写
			if label.ChainID != 1 {                        //不是以太坊
				continue
			}
			if _, ok := LABEL_MAP[label.Address]; !ok {
				LABEL_MAP[label.Address] = LabelInfo{Label: make([]string, 0), Type: "Token"}
			}
			obj := LABEL_MAP[label.Address]
			obj.Label = append(obj.Label, label.Label)
			obj.Name = label.Name
			obj.Symbol = label.Symbol
			LABEL_MAP[label.Address] = obj
		}
	} else if type_ == "Account" {
		err = json.Unmarshal([]byte(input_bytes), &account_label_list)
		check(err)
		for _, label := range account_label_list {
			label.Address = strings.ToLower(label.Address) //转小写
			if label.ChainID != 1 {                        //不是以太坊
				continue
			}
			if _, ok := LABEL_MAP[label.Address]; !ok {
				LABEL_MAP[label.Address] = LabelInfo{Label: make([]string, 0), Type: "Account"}
			}
			obj := LABEL_MAP[label.Address]
			obj.Label = append(obj.Label, label.Label)
			obj.Tag = label.Tag
			LABEL_MAP[label.Address] = obj
		}
	} else {
		panic("Illigal type")
	}

}

var INPUT_PATH = "./output/"
var OUTPUT_PATH = "./output/"

func main() {
	ReadLabel("tokens", "Token")
	ReadLabel("accounts", "Account")
	fmt.Println("Label map length:", len(LABEL_MAP))

	path := INPUT_PATH + os.Args[1] + ".json"
	// 读取文件到DATA
	input, err := os.Open(path)
	check(err)
	defer input.Close()
	input_bytes, err := ioutil.ReadAll(input)
	check(err)
	var sandwich_list []Sandwich
	err = json.Unmarshal([]byte(input_bytes), &sandwich_list)
	check(err)
	fmt.Println("sandwich_list length:", len(sandwich_list))

	for _, bundle := range sandwich_list[0:6] { // !!!!!!!!!!!!!!!!!!!!!!!!!!!!!
		for touch_address, _ := range bundle.TouchAddressMap {
			if _, ok := LABEL_MAP[strings.ToLower(touch_address.Hex())]; !ok {
				//fmt.Println("Not Found")
				//fmt.Println(touch_address.Hex())
				obj := bundle.TouchAddressMap[touch_address]
				obj.LabelInfo = LabelInfo{Type: "Unknow"}
				bundle.TouchAddressMap[touch_address] = obj
			} else {
				obj := bundle.TouchAddressMap[touch_address]
				obj.LabelInfo = LABEL_MAP[strings.ToLower(touch_address.Hex())]
				bundle.TouchAddressMap[touch_address] = obj
			}
		}
	}

	jsonData, err := json.MarshalIndent(sandwich_list[0:6], "", "  ")
	check(err)
	//fmt.Println(string(jsonData))
	output, err := os.Create(OUTPUT_PATH + "sandwich_sample.json")
	check(err)
	output.Write(jsonData)

}

//
