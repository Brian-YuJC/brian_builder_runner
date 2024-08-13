## Final Data Exclude to_block_num
## Example from_18000000_to_19000000, it only includes 18000000~18999999

import requests
import sys
import csv
from datetime import datetime
import time
import json
import os

FLASHBOTS_URL = "https://blocks.flashbots.net/v1/blocks"
HTTP_MAX_RETRIES_TIME = 300
SLEEP_SECOND = 5



if len(sys.argv) > 1 and sys.argv[1] == "test":
    ## test mode

    from_block_num = int(sys.argv[2])
    to_block_num = int(sys.argv[3])
else:
    ## production mode

    from_block_num = int(sys.argv[1])
    to_block_num = int(sys.argv[2])

path = './'
if not os.path.isdir(path):
    os.makedirs(path)

write_filename = path + 'flashbots_info_from_%s_to_%s.csv' % (from_block_num, to_block_num)
error_filename = path + "Step3_1_from_%s_to_%s_error.txt" % (from_block_num, to_block_num)
error_file = open(error_filename, "w")





def check_flashbots(block_num):
    url = FLASHBOTS_URL + "?before=%s&limit=100" % block_num
    http_response = requests.get(url)
    if http_response.status_code != 200:
        for retry_time in range(HTTP_MAX_RETRIES_TIME):
            retry_time += 1
            print("http request retry time: %s. url is %s" % (retry_time, url))
            if retry_time == HTTP_MAX_RETRIES_TIME:
                error_file.write("Fail over %s times. URL: %s\n" % (retry_time, url))
                return ["network_error"]

            time.sleep(SLEEP_SECOND)
            http_response = requests.get(url)
            if http_response.status_code == 200:
                break

    response = http_response.json()
    flashbots_blocks_info = response["blocks"]
    return flashbots_blocks_info


with open(write_filename, "w") as write_file:
    writer = csv.writer(write_file)
    writer.writerow(["block_num", "flashbots_block_info_json"])

    # from end number to start number
    current_block_num = to_block_num

    while (current_block_num >= from_block_num):
        print("[%s]Request Block Number: %s" % (datetime.now(), current_block_num))
        flashbots_blocks_info = check_flashbots(current_block_num)
        block_num_list = []

        if len(flashbots_blocks_info) == 0:
            ## Prevent Unlimit Loop
            print("Request blocks info is 0, request blocknum %s" % current_block_num)
            current_block_num = current_block_num - 99

        if len(flashbots_blocks_info) > 0:
            min_block_num = int(flashbots_blocks_info[0]["block_number"])
            max_block_num = int(flashbots_blocks_info[0]["block_number"])

            for block_info in flashbots_blocks_info:
                get_block_num = int(block_info["block_number"])
                block_num_list.append(get_block_num)
                min_block_num = min(get_block_num, min_block_num)
                max_block_num = max(get_block_num, max_block_num)

                if get_block_num >= from_block_num:
                    write_block_info = json.dumps(block_info)
                    writer.writerow([get_block_num, write_block_info])

            print("Block Num List: %s" % block_num_list)
            print("Lowest Block Num is %s" % min_block_num)
            current_block_num = min_block_num