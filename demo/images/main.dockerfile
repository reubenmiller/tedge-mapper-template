FROM ghcr.io/thin-edge/tedge-demo-main-systemd:20230517.2

# Install
COPY dist/tedge*.deb /setup/build/
COPY dist/c8y*.deb /setup/build/
COPY dist/tedge-mapper-template*.deb" /tmp/
RUN dpkg -i /tmp/*.deb \
    && systemctl enable tedge-mapper-template.service

# custom scripts
COPY images/files/50-main-setup.sh /etc/boostrap/post.d/