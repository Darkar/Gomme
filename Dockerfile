FROM golang:1.25-alpine AS builder

COPY src/ /src/

WORKDIR /src

RUN go mod download

RUN go build -o gomme .

FROM alpine:latest

RUN apk add --no-cache git openssh-client

WORKDIR /app

COPY app/ .

COPY --from=builder /src/gomme .

EXPOSE 3000

CMD ["./gomme"]