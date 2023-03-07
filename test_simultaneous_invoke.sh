#!/bin/bash

for i in {1..10}
do
   curl -X POST 'localhost:8080/invoke' &
done

wait