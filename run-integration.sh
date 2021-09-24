#!/usr/bin/env bash

set -ex

go install gotest.tools/gotestsum@latest

if [ -n "$LOG_DIRECTORY" ]; then
  echo "Log directory defined"
else
  LOG_DIRECTORY="."
fi
echo "test result will be in ${LOG_DIRECTORY}"

mkdir -p "${LOG_DIRECTORY}"
if [ -n "${SUITE}" ]; then
  echo "running suite ${SUITE}"
else
   SUITE="$*"
fi

if [ -n "${START_SLEEP}" ]; then
  echo "START_SLEEP = ${START_SLEEP}"
else
    echo "START_SLEEP = 0"
   START_SLEEP=0
fi
sleep "${START_SLEEP}"
#DEBUG_LEVEL=3 go test -v -timeout 120m --tags=integration,debug ./integration "$@" 2>&1 | tee "${LOG_DIRECTORY}/raw.log" >(go-junit-report -set-exit-code > "${LOG_DIRECTORY}/integration.junit.xml")
# go test -v -timeout 120m --tags=integration ./integration "$@" 2>&1 | tee "${LOG_DIRECTORY}/raw.log" >(go-junit-report -set-exit-code > "${LOG_DIRECTORY}/integration.junit.xml")
TEST_DIRECTORY=./integration gotestsum --format standard-verbose -- -p 1 -timeout 120m --tags=integration -run "${SUITE}" ./integration | tee "${LOG_DIRECTORY}/raw_${SUITE}.log"