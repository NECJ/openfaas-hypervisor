#!/bin/bash

for i in {1..2}
do
   curl -X POST 'localhost:8080/function/calc-pi' &
done

wait