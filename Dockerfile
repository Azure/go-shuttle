FROM golang:1.14
WORKDIR pubsub
COPY . .
RUN touch .env
ENTRYPOINT ["go", "test", "-v", "./integration"]
