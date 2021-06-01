FROM golang:1.14
WORKDIR pubsub
COPY . .
RUN touch .env && chmod +x ./run-integration.sh
#ENTRYPOINT ["tail", "-f", "/dev/null"]
ENTRYPOINT ["./run-integration.sh"]
