#!/bin/bash

if [ $# -ne 2 ]; then
  echo "usage: $0 level count" >&2
  exit 1
fi

level=$1
count=$2

function mkdirs {
  local level=$1
  local prefix=$2
  if [ $level -eq 0 ]; then
    local i=0
    while [ $i -lt $count ]; do
      touch $prefix/$i
      ((i++))
    done
  else
    local i=0
    while [ $i -lt $count ]; do
      mkdir $prefix/$i
      mkdirs $((level-1)) $prefix/$i
      ((i++))
    done
  fi
}

echo "creating files..."
time mkdirs $level "."
echo "syncing..."
time sync ; sync ; sync
