FROM golang:1.17-alpine

WORKDIR /app

COPY . .
RUN go build -o /agent

EXPOSE 8080

CMD [ "/agent" ]
