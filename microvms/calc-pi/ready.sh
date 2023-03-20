#!/bin/sh

/bin/server $(route -n | grep 'UG[ \t]' | awk '{print $2}')