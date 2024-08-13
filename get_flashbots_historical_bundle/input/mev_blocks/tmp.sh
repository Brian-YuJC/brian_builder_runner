#!/bin/bash
python ./Step3_1_get_all_flashbots_block_info.py 19700000 19800000 > 19700000_19800000.log & 
python ./Step3_1_get_all_flashbots_block_info.py 19800000 19900000 > 19800000_19900000.log &
python ./Step3_1_get_all_flashbots_block_info.py 19900000 20000000 > 19900000_20000000.log &
python ./Step3_1_get_all_flashbots_block_info.py 20000000 20100000 > 20000000_20100000.log &
python ./Step3_1_get_all_flashbots_block_info.py 20000000 20200000 > 20000000_20200000.log &

python ./Step3_1_get_all_flashbots_block_info.py 19700000 20200000 > 19700000_20200000.log &