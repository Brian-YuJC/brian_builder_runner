package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Discard 太早数据获取不到
// 调用接口获取MEV block
func GetBlock(block_number uint64) bool {
	time.Sleep(time.Second)
	params := url.Values{}
	params.Add("block_number", strconv.Itoa(int(block_number)))
	URL := "https://blocks.flashbots.net/v1/blocks?" + params.Encode()
	resp, err := http.Get(URL)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	//fmt.Println("Status Code:", resp.StatusCode)
	if resp.StatusCode != 200 {
		fmt.Println("Status Code:", resp.StatusCode)
		return false
	}

	body_string := string(body)
	s := strings.Split(body_string[1:len(body_string)-1], ",")
	//fmt.Println(len(s))
	if len(s) <= 3 {
		return false
	}
	return true
}

// 读取大JSON文件
func processLargeJSON(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.UseNumber() //不使用这个数字会被解析成1.497381e+07这样

	// 读取 JSON 数组的开始标记
	_, err = decoder.Token()
	if err != nil {
		log.Fatal(err)
	}

	// 逐个读取 JSON 对象
	for decoder.More() {
		var item map[string]interface{}
		err := decoder.Decode(&item)
		if err != nil {
			log.Fatal(err)
		}
		// 处理每个 JSON 对象
		fmt.Println(item["block_number"])
	}

	// 读取 JSON 数组的结束标记
	_, err = decoder.Token()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {

	//processLargeJSON("./input/all_blocks.json")

	f, err := os.Open("./block_range.csv")
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	for {
		rec, err := csvReader.Read()
		if err == io.EOF || rec[0] == "" {
			break
		}
		block_number, _ := strconv.ParseUint(rec[0], 10, 64)
		if GetBlock(block_number) {
			fmt.Println(block_number)
		}
	}

}

//This file is discard, because we have more better way to obtain the MEV block, using Ben's method (./output/mev_blocks/Step3_1_get_all_flashbots_block_info.py)
