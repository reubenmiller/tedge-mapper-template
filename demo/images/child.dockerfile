FROM ghcr.io/thin-edge/tedge-demo-main-systemd:20230517.2

COPY dist/tedge_*.deb /tmp/
COPY dist/tedge-agent*.deb /tmp/
COPY dist/tedge-dummy-plugin_*.deb /tmp/

# Install component which will act like a child device
RUN dpkg -i /tmp/tedge_*.deb \
    && dpkg -i /tmp/tedge-agent*.deb \
    && dpkg -i /tmp/tedge-dummy-plugin*.deb

# Overwrite tedge-agent setup
COPY images/files/tedge-agent.service /lib/systemd/system/

# bootstrapping settings
ENV BOOTSTRAP=0
ENV CONNECT=0
ENV INSTALL=0
ENV SHOULD_PROMPT=0

# custom scripts
COPY images/files/50-child-setup.sh /etc/boostrap/post.d/
