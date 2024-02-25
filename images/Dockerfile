FROM alpine:3.18
WORKDIR /app
COPY bin/tedge-mapper-template /usr/bin
COPY routes/c8y-operations.yaml /default-routes/
ENV MQTT_HOST="host.docker.internal:1883"
ENTRYPOINT [ "/usr/bin/tedge-mapper-template" ]
VOLUME [ "/routes" ]
CMD ["serve", "--host", "${MQTT_HOST}", "--dir", "/default-routes", "--dir", "/routes"]