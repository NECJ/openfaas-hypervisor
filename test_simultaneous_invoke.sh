#!/bin/bash

for i in {1..3}
do
   curl -X POST 'localhost:8080/function/calc-pi' &
done

wait


for i in {1..10}
do
   curl -X POST 'localhost:8080/function/calc-pi' &
done

wait