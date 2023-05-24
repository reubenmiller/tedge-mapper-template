#!/bin/bash

echo "Enabling dummy sm-plugin"

if [ ! -f /etc/tedge/sm-plugins/dummy ]; then
    ln -s /usr/bin/tedge-dummy-plugin /etc/tedge/sm-plugins/dummy
fi

# Removing other sm-plugins
rm -f /etc/tedge/sm-plugins/container*

systemctl start tedge-agent
systemctl enable tedge-agent
