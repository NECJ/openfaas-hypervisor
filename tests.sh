#!/bin/bash

cold_start()
{
   # Create data file
   datafile="cold_start_data.csv"
   echo "NumbInitVms,VmInitTimeNanoAvg,VmInitTimeNanoStd,VmInitTimeNano95,VmInitTimeNanoMax,FuncExecTimeNanoAvg,FuncExecTimeNanoStd,FuncExecTimeNano95,FuncExecTimeNanoMax" >> $datafile
   for invokes in $(seq 1 1 60)
   do
      cold_start_internal $invokes
   done

   for invokes in $(seq 60 10 100)
   do
      cold_start_internal $invokes
   done

   for invokes in $(seq 100 100 1000)
   do
      cold_start_internal $invokes
   done
}

cold_start_internal()
{
   echo "Number: $1"
   # Start server
   DISABLE_VM_REUSE=TRUE ./openfaas_hypervisor &
   openfaas_pid=$!
   trap "kill -SIGINT $openfaas_pid" EXIT

   # Wait for server to start up
   sleep 4

   # Invoke function
   curlPids=()
   for j in $(seq 1 $1)
   do
      curl -X POST 'localhost:8080/function/calc-pi' &>/dev/null &
      curlPids+=($!)
   done
   # Wait for all curl processes to finish
   for pid in ${curlPids[@]}; do
      wait $pid
   done

   sleep 4

   # Get stats
   curl -s -X POST 'localhost:8080/stats' | jq -r '[.NumbInitVms, .VmInitTimeNanoAvg, .VmInitTimeNanoStd, .VmInitTimeNano95, .VmInitTimeNanoMax, .FuncExecTimeNanoAvg, .FuncExecTimeNanoStd, .FuncExecTimeNano95, .FuncExecTimeNanoMax] | @csv' >> $datafile

   # End server
   echo "Shutting down server..."
   if ! kill -SIGINT $openfaas_pid ; then
      exit
   fi
   wait $openfaas_pid
   sleep 4
}

warm_start()
{
   # Create data file
   datafile="warm_start_data.csv"
   echo "NumbInitVms,FuncExecTimeNanoAvg,FuncExecTimeNanoStd,FuncExecTimeNano95,FuncExecTimeNanoMax" >> $datafile
   for invokes in $(seq 1 1 60)
   do
      warm_start_internal $invokes
   done

   for invokes in $(seq 60 10 100)
   do
      warm_start_internal $invokes
   done

   for invokes in $(seq 100 100 1000)
   do
      warm_start_internal $invokes
   done
}

warm_start_internal() 
{
   echo "Number: $1"
   # Start server
   ./openfaas_hypervisor &
   openfaas_pid=$!
   trap "kill -SIGINT $openfaas_pid" EXIT

   # Wait for server to start up
   sleep 4

   # Pre-boot VMs
   curl -X POST --data-raw $1 'localhost:8080/preBoot/calc-pi'
   # Wait for them to boot
   booted=$(curl -s -X POST 'localhost:8080/stats' | jq -r '.NumbInitVms')
   while [[ $booted != $1 ]]
   do
      sleep 0.5
      booted=$(curl -s -X POST 'localhost:8080/stats' | jq -r '.NumbInitVms')
   done

   # Invoke function
   curlPids=()
   for j in $(seq 1 $1)
   do
      curl -X POST 'localhost:8080/function/calc-pi' &>/dev/null &
      curlPids+=($!)
   done
   # Wait for all curl processes to finish
   for pid in ${curlPids[@]}; do
      wait $pid
   done

   sleep 4

   # Get stats
   curl -s -X POST 'localhost:8080/stats' | jq -r '[.NumbInitVms, .FuncExecTimeNanoAvg, .FuncExecTimeNanoStd, .FuncExecTimeNano95, .FuncExecTimeNanoMax] | @csv' >> $datafile

   # End server
   echo "Shutting down server..."
   if ! kill -SIGINT $openfaas_pid ; then	
      exit	
   fi
   wait $openfaas_pid
   sleep 4
}

help()
{
   echo "Usage: $0 <command>" 1>&2
   echo "  command: cold_start warm_start" 1>&2
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
      
   "warm_start")
      warm_start
      ;;

   *)
      echo "Unknown command: $command"
      help
esac