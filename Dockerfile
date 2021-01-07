FROM golang:1.14
WORKDIR pubsub
COPY . .
RUN touch .env
#ENTRYPOINT ["tail", "-f", "/dev/null"]
ENTRYPOINT ["go", "test", "-v", "./integration", "-run", "TestConnectionString/TestPublishAndConcurrentListen"]
