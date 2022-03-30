#!/usr/bin/env bash
#
# Copyright 2022 Couchbase, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
set -exuo pipefail

# Waits for the given cURL call to succeed.
# Parameters:
# $1: the number of attempts to try loading before failing
# Remaining parameters: passed directly to cURL.
function wait_for_curl() {
    local MAX_ATTEMPTS=$1
    shift
    local ATTEMPTS=0
    echo "Curl command: curl -s -o /dev/null $*"
    until curl -s -o /dev/null "$@"; do
        # Prevent an infinite loop - at 2 seconds per go this is 10 minutes
        if [ $ATTEMPTS -gt "300" ]; then
            fail "wait_for_curl ultimate max exceeded"
        fi
        if [ $ATTEMPTS -gt "$MAX_ATTEMPTS" ]; then
            fail "unable to perform cURL"
        fi
        ATTEMPTS=$((ATTEMPTS+1))
        sleep 2
    done
}

docker compose up -d --build

wait_for_curl 100 http://localhost:8091 -f -u Administrator:password
# Gets rather spammy
set +x
wait_for_curl 100 http://localhost:9091/metrics

# XDCR setup is the last step so wait for that
while true; do
  output=$(curl -s -u Administrator:password localhost:8091/pools/default/replications)
  if [[ "$output" == *testBucket* ]]; then
    break
  fi
  echo "Not yet done"
  sleep 2
done
# And a little buffer
sleep 10

output=$(curl -s http://localhost:9091/metrics)

if [[ "$output" =~ .*'An error has occurred while serving metrics'.* ]]; then
  echo "FAILURE!"
  echo "$output"
  exit 1
fi

echo "Success! $(echo "$output" | wc -l) lines received. Cleaning up..."
docker compose down -v
exit 0
