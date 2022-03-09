#!/bin/bash

i=0

bind=30000
http=40000
prox=50000

for i in $(seq 1 1 $1)
do
   echo $i
   echo $prox, $http, $bind

   docker run -d --label gossip=true --net=host yyyy agent server --min-interval 2 --max-peers $2 --bind-addr 127.0.0.1:$bind --http-addr 127.0.0.1:$http --proxy-addr "127.0.0.1:${prox}"

   ((i=i+1))

   ((bind=bind+1))
   ((http=http+1))
   ((prox=prox+1))
done
