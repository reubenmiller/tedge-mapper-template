FROM alpine:3.18
WORKDIR /app
COPY tedge-mapper-template .
ENV MQTT_HOST="host.docker.internal:1883"
ENTRYPOINT [ "/app/tedge-mapper-template" ]
CMD ["serve", "--verbose", "--host", "${MQTT_HOST}"]