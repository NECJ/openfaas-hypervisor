#!/bin/sh

curl $(route -n | grep 'UG[ \t]' | awk '{print $2}'):8080/ready