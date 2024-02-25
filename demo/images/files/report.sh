#!/bin/sh
set -eu

OK=0
#FAILURE=1


STATE="$1"
shift

echo "Running state: $STATE" >&2

case "$STATE" in
    executing)
        echo "Generating report" >&2
        ;;
esac

sleep 1
exit "$OK"
