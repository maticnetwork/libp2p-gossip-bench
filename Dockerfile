FROM golang:1.17-alpine

WORKDIR /app

COPY . .
RUN go build -o /usr/local/bin/agent

COPY ./scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
