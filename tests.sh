#!/bin/bash

cold_start()
{
   # Create data file
   datafile="cold_start_data.csv"
   echo "NumbInitVms,VmInitTimeNanoAvg,VmInitTimeNanoStd,FuncExecTimeNanoAvg,FuncExecTimeNanoStd" >> $datafile
   for invokes in $(seq 1 5 100)
   do
      # Start server
      DISABLE_VM_REUSE=TRUE ./openfaas_hypervisor &
      openfaas_pid=$!
      trap "kill -SIGINT $openfaas_pid" EXIT

      # Wait for server to start up
      sleep 1

      # Invoke function
      curlPids=()
      for j in $(seq 1 $invokes)
      do
         curl -X POST 'localhost:8080/function/calc-pi' &>/dev/null &
         curlPids+=($!)
      done
      # Wait for all curl processes to finish
      for pid in ${curlPids[@]}; do
         wait $pid
      done

      # Get stats
      curl -s -X POST 'localhost:8080/stats' | jq -r '[.NumbInitVms, .VmInitTimeNanoAvg, .VmInitTimeNanoStd, .FuncExecTimeNanoAvg, .FuncExecTimeNanoStd] | @csv' >> $datafile

      # End server
      kill -SIGINT $openfaas_pid
      wait $openfaas_pid
   done
}

help()
{
   echo "Usage: $0 <command>" 1>&2
   echo "  command: cold_start" 1>&2
   exit 1
}

if test $# -ne 1; then
    help
fi

command="$1"

case "$command" in

    "cold_start")
        cold_start
        ;;

    *)
        echo "Unknown command: $command"
        help
esac