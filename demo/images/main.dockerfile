FROM ghcr.io/thin-edge/tedge-demo-main-systemd:20240223.1219

# custom configuration
COPY images/files/tedge-mapper-template.env /etc/tedge-mapper-template/env

# Install
COPY dist/tedge-mapper-template*.deb /setup/build/
COPY dist/tedge-mapper-template*.deb /tmp/
RUN dpkg -i /tmp/*.deb

COPY images/files/report.sh /usr/bin/
COPY images/files/report.toml /etc/tedge/operations/
