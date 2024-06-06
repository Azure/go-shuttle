FROM golang:1.21-alpine3.20
WORKDIR pubsub
RUN apk --no-cache update && \
 	apk --no-cache upgrade && \
  	apk --no-cache add \
  	bash \
  	musl-dev \
  	gcc
COPY . .
RUN touch .env && chmod +x ./run-integration.sh
ENV INTEGRATION=true
#ENTRYPOINT ["tail", "-f", "/dev/null"]
ENTRYPOINT ["./run-integration.sh"]
