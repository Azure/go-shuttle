#!/usr/bin/env bash

containerId=$1
if [ ${#containerId} -eq 0 ]; then
  echo "not a valid container id: ${containerId}" ; exit
fi
while :
  do
    finishTime=$(az container show --ids "${containerId}" --query "containers[0].instanceView.currentState.finishTime")
    if [[ -n $finishTime ]]; then
      echo "${finishTime}"
      break
    else
      echo "[$(date -u)] Running"
    fi
    sleep 2s
  done