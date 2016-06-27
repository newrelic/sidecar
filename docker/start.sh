#!/bin/sh

cd /sidecar
for entry in $SIDECAR_SEEDS; do
	CLI="$CLI --cluster-ip $entry"
done

if [[ -n "$SIDECAR_ADVERTISE_IP" ]]; then
	CLI="$CLI --advertise-ip $SIDECAR_ADVERTISE_IP"
fi

if [[ -n "$SIDECAR_CLUSTERNAME" ]]; then
	CLI="$CLI --cluster-name $SIDECAR_CLUSTERNAME"
fi

if [[ -n "$SIDECAR_LOGGING_LEVEL" ]]; then
	sed -i.bak 's/logging_level *= *"info"/logging_level = "'"$SIDECAR_LOGGING_LEVEL"'"/' sidecar.toml
fi

BIND_IP=`grep bind_ip sidecar.toml | grep -o "[0-9]*\.[0-9]*\.[0-9]*\.[0-9]*"`

# If there's a BIND_IP and we don't already have it, add the
# address to the loopback interface.
if [[ -n $BIND_IP ]] && [[ $BIND_IP != "0.0.0.0" ]] || [[ ! ip addr show | grep $BIND_IP ]]; then
	echo "Adding $BIND_IP to the loopback interface"
	ip addr add $BIND_IP/32 dev lo
fi

./sidecar $CLI
