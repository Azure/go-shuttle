FROM golang
WORKDIR pubsub
COPY . .
RUN touch .env
ENTRYPOINT ["go", "test", "-v"]
