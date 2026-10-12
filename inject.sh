#!/usr/bin/env bash
exec 3<lwl.log
while read -u 3 -r IP JSON
do
  [[ $IP == "#" ]] && continue
  printf "${JSON}" | tee >(socat -u STDIN UDP-SENDTO:127.0.0.1:9761)
  # read -p ""
done
