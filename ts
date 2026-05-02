#!/usr/bin/env bash
IFS=$'\n'
PREV=${EPOCHREALTIME/./}
while read -r LINE
do
  CURR=${EPOCHREALTIME/./}
  DUR=$(( CURR - PREV ))
  printf "%10d %s\n" ${DUR} "${LINE}"
  PREV=${CURR}
done
