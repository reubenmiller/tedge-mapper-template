#!/bin/bash

echo "Creating a local child device" >&2

# For now prefix the child device with the device id (until the registration can be fixed)
# We will only concentrate on the operation handling for now
DEVICE_ID=$(tedge config get device.id)
CHILD=child01
CHILD_DIR="/etc/tedge/operations/c8y/${DEVICE_ID}_${CHILD}"
sudo -u tedge mkdir -p "$CHILD_DIR"
sudo -u tedge touch "$CHILD_DIR/c8y_SoftwareUpdate"

echo "Stopping tedge-mapper-c8y"
sudo systemctl disable tedge-mapper-c8y
sudo systemctl stop tedge-mapper-c8y

sudo systemctl disable tedge-container-monitor ||:
sudo systemctl stop tedge-container-monitor ||:
