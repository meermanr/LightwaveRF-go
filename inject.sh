#!/usr/bin/env bash
< <(tail -n1 lwl.log) read IP JSON; declare -p IP; declare -p JSON
socat -u STDIN UDP-SENDTO:127.0.0.1:9761 <<<"${JSON}"
