#!/bin/bash

work=1

_exit() {
  echo "[DO] entering _exit()"
  sleep 3
  work=0
}

trap "_exit" SIGTERM SIGINT

echo "[DO] Starting"

while [ $work -eq 1 ]
do
  echo "[DO] Still in the loop"
  sleep 3
  echo "[DO] Still in the loop, after sleep"
done

echo "[DO] Exiting"
