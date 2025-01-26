#! /bin/sh

# Get path
VER_PATH=$(echo `dirname $0`/../main.go)
cat ${VER_PATH} | grep VERSION | head -1 | awk -F= '{print $2}' | tr -d '"' | tr -d ' '
