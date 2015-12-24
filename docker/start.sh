#!/bin/sh

cd /sidecar
for entry in $SIDECAR_SEEDS; do
	CLI="$CLI --cluster-ip $entry"
done

if [[ -n $SIDECAR_ADVERTISE_IP ]]; then
	CLI="$CLI --advertise-ip $SIDECAR_ADVERTISE_IP"
fi

./sidecar $CLI
