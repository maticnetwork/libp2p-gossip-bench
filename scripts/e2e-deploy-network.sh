#!/bin/bash

for i in $(seq 1 1 $1)
do
   docker run -d --label gossip=true yyyy agent server --max-peers $2 --bind-addr 127.0.0.1:3000 --proxy-addr '{{ GetInterfaceIP "eth0" }}:8000'
done
