#!/bin/sh

apk add curl

# Waits for the given cURL call to succeed.
# Parameters:
# $1: the number of attempts to try loading before failing
# Remaining parameters: passed directly to cURL.
function wait_for_curl() {
    local MAX_ATTEMPTS=$1
    shift
    local ATTEMPTS=0
    echo "Curl command: curl -s -o /dev/null -f $*"
    until curl -s -o /dev/null -f "$@"; do
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

# Waits for the given URL to return 200
# Parameters:
# $1: the number of attempts to try loading before failing
# $2: the URL to load
# $3: HTTP basic authentication credentials (format: username:password) [optional]
function wait_for_url() {
    local MAX_ATTEMPTS=$1
    local URL=$2
    local CREDENTIALS=${3-}
    local extra_args=""
    if [ -n "$CREDENTIALS" ]; then
        extra_args="-u $CREDENTIALS"
    fi
    # shellcheck disable=SC2086
    wait_for_curl "$MAX_ATTEMPTS" "$URL" $extra_args
}

wait_for_url 30 "http://cb6:8091/ui"

sleep 10

exec /yacpe
