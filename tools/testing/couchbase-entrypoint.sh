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
#
set -euo pipefail
#set +e # temporary

hostname=$(hostname)
cluster_id=${hostname%-}

function wait_for_uri() {
  expected=$1
  shift

 local ATTEMPTS=0
  # Wait for startup - no great way for this
  while true; do
    set +e
    status=$(curl -s -w "%{http_code}" -o /dev/null "$@")
    set -e
    if [ "x$status" == "x$expected" ]; then
      return
    fi
    # Prevent an infinite loop - at 2 seconds per go this is 5 minutes
    if [ $ATTEMPTS -gt "150" ]; then
        echo "Failed cURL: $*"
        exit 1
    fi
    ATTEMPTS=$((ATTEMPTS+1))
    echo "CURL code $status"
    sleep 2
  done
}

function startServer() {
    /opt/couchbase/bin/couchbase-server -- -kernel global_enable_tracing false -noinput &

    echo "Wait for it to be ready"
    wait_for_uri 200 http://127.0.0.1:8091/ui/index.html
}

function configureCluster() {
    echo "Configuring cluster"
    couchbase-cli cluster-init -c 127.0.0.1 \
        --cluster-username Administrator \
        --cluster-password password \
        --services data,index,query,fts,analytics,eventing \
        --cluster-ramsize 1024 \
        --cluster-index-ramsize 512 \
        --cluster-eventing-ramsize 512 \
        --cluster-fts-ramsize 512 \
        --cluster-analytics-ramsize 1024 \
        --cluster-fts-ramsize 512 \
        --cluster-name "CMOS-Exporter-test-$cluster_id" \
        --index-storage-setting default
}

function createBucket() {
    echo "Creating bucket"
    couchbase-cli bucket-create -c 127.0.0.1 \
        --username Administrator \
        --password password \
        --bucket testBucket \
        --bucket-type couchbase \
        --bucket-ramsize 256 \
        --max-ttl 500000000 \
        --durability-min-level persistToMajority \
        --enable-flush 0
}

function loadSampleBucket() {
  echo "Loading travel-sample..."
  curl -fv -u Administrator:password -X POST http://127.0.0.1:8091/sampleBuckets/install -H 'Content-Type: application/json' -d '["travel-sample"]'
  echo "travel-sample load started, waiting..."
  wait_for_uri 200 http://127.0.0.1:8091/pools/default/buckets/travel-sample -u Administrator:password
}

function createFTSIndexes() {
  echo "Creating test FTS indexes..."
  curl -fv -u Administrator:password -X PUT http://localhost:8094/api/index/test -H 'Content-Type: application/json' --data @/entrypoint/testFTSIndex1.json
  curl -fv -u Administrator:password -X PUT http://localhost:8094/api/index/test2 -H 'Content-Type: application/json' --data @/entrypoint/testFTSIndex2.json
}

function createEventingFunctions() {
  echo "Creating test Eventing functions..."
  curl -f -u Administrator:password -X POST http://localhost:8096/api/v1/import --data @/entrypoint/testEventingFunctions.json
  curl -f -u Administrator:password -XPOST http://Administrator:password@localhost:8096/api/v1/functions/test/deploy
  curl -f -u Administrator:password -XPOST http://Administrator:password@localhost:8096/api/v1/functions/test2/deploy
}

function setupXDCR() {
  if [[ "$(hostname)" == cb2-* ]]; then
    return
  fi
  echo "Setting up XDCR remote cluster..."
  # intentionally no -f here
  set +e
  curl -s -u Administrator:password http://localhost:8091/pools/default/remoteClusters \
    -d name=cb2 \
    -d hostname=cb2-1.local \
    -d username=Administrator \
    -d password=password
  set -e
  echo "Waiting for remote..."
  while true; do
    output=$(curl -s -u Administrator:password localhost:8091/pools/default/remoteClusters)
    if [[ "$output" == *RC_OK* ]]; then
      break
    fi
    echo "Remote not yet ready"
    sleep 2
  done
  echo "Setting up replications..."
  curl -fv -u Administrator:password http://localhost:8091/controller/createReplication \
    -d fromBucket=travel-sample \
    -d toBucket=travel-sample \
    -d toCluster=cb2 \
    -d priority=High \
    -d replicationType=continuous
  curl -fv -u Administrator:password http://localhost:8091/controller/createReplication \
    -d fromBucket=travel-sample \
    -d toBucket=testBucket \
    -d toCluster=cb2 \
    -d replicationType=continuous
}

function waitForStartup() {
    echo "Waiting for startup completion"
    wait_for_uri 200 http://127.0.0.1:8091/pools/default -u Administrator:password
    echo "Running"
}

echo "Overriding CBS entrypoint as this gives us declarative control"
startServer
configureCluster
waitForStartup
createBucket
loadSampleBucket
createFTSIndexes
createEventingFunctions
setupXDCR

# Hand control back to CB
echo "Setup complete!"
wait
