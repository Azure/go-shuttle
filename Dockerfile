FROM golang:1.17.0-alpine3.14
WORKDIR pubsub
RUN apk --no-cache update && \
 	apk --no-cache upgrade && \
  	apk --no-cache add \
  	bash \
  	musl-dev \
  	gcc
COPY . .
RUN touch .env && chmod +x ./run-integration.sh
#ENTRYPOINT ["tail", "-f", "/dev/null"]
ENTRYPOINT ["./run-integration.sh"]
