package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// 用于解析JSON文件
type MEVTx struct {
	Block_number              uint64    `json:"block_number"`
	Tx_index                  int       `json:"tx_index"`
	Mev_type                  string    `json:"mev_type"`
	Protocol                  string    `json:"protocol"`
	User_loss_usd             float64   `json:"user_loss_usd"`
	Extractor_profit_usd      float64   `json:"extractor_profit_usd"`
	User_swap_volume_usd      float64   `json:"user_swap_volume_usd"`
	User_swap_count           int       `json:"user_swap_count"`
	Extractor_swap_volume_usd float64   `json:"extractor_swap_volume_usd"`
	Extractor_swap_count      int       `json:"extractor_swap_count"`
	Imbalance                 float64   `json:"imbalance"`
	Address_from              string    `json:"address_from"`
	Address_to                string    `json:"address_to"`
	Arrival_time_us           time.Time `json:"arrival_time_us"`
}

var MEVTX_LIST []MEVTx

func RequestDataFromAPI(start_block uint64, block_cnt int) int {
	params := url.Values{}
	params.Add("block_number", strconv.Itoa(int(start_block)))
	params.Add("count", strconv.Itoa(block_cnt))
	URL := "https://data.zeromev.org/v1/mevBlock?" + params.Encode()
	resp, err := http.Get(URL)
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}
	defer resp.Body.Close()

	//fmt.Println("Status Code:", resp.StatusCode)
	if resp.StatusCode != 200 {
		fmt.Println("Status Code:", resp.StatusCode)
		return resp.StatusCode
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}

	var data []MEVTx //直接映射成结构体
	err = json.Unmarshal([]byte(body), &data)
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}

	//With Filter！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！！
	for _, tx := range data {
		if tx.Mev_type != "swap" {
			MEVTX_LIST = append(MEVTX_LIST, tx) //维护全局变量
		}
	}
	//MEVTX_LIST = append(MEVTX_LIST, data...) //维护全局变量, No filter

	//fmt.Println(data)
	fmt.Println("Status Code: 200")
	return 200

}

func LoopRequest(start_block_num uint64, end_block_num uint64) {
	var current_max_block_num uint64 = start_block_num
	for {
		if current_max_block_num >= end_block_num {
			break
		}
		status := RequestDataFromAPI(uint64(current_max_block_num), 100)
		if status != 200 {
			time.Sleep(time.Millisecond * 100)
		} else { //成功
			current_max_block_num = MEVTX_LIST[len(MEVTX_LIST)-1].Block_number + 1
			fmt.Println(current_max_block_num)
		}
	}
}

var OUTPUT_PATH = "./output/"

func main() {

	// TODO 根据命令行输入的范围生成文件名
	begin := os.Args[1]
	end := os.Args[2]
	output_path := OUTPUT_PATH + os.Args[1] + "_" + os.Args[2] + ".json"
	begin_uint64, err := strconv.ParseUint(begin, 10, 64)
	check(err)
	end_uint64, err := strconv.ParseUint(end, 10, 64)
	check(err)
	LoopRequest(begin_uint64, end_uint64)
	//LoopRequest(19731000, 20350000)
	//LoopRequest(19731000, 19750000)

	//输出json
	jsonData, err := json.MarshalIndent(MEVTX_LIST, "", "  ")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	//fmt.Println(string(jsonData))
	output, err := os.Create(output_path)
	if err != nil {
		fmt.Println("Error:", err)
	}
	output.Write(jsonData)
}

//！！！！！！！！！！！！！！！！！！！注意这里是加了filter不提取swag类型的mev！！！！！！！！！！！！！！！！！！！！！！
// go run get_data_from_api.go 19731000 19741000
