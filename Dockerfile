FROM golang
WORKDIR pubsub
COPY . .
RUN touch .env
RUN go mod vendor
ENTRYPOINT ["go", "test", "-v"]
