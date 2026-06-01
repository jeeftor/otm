FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/otm ./cmd/otm

FROM alpine:3.20
RUN addgroup -S otm && adduser -S -G otm otm
WORKDIR /app
COPY --from=build /out/otm /usr/local/bin/otm
RUN mkdir -p /data && chown -R otm:otm /data
USER otm
ENV OTM_DATA_DIR=/data
EXPOSE 8080/tcp 2055/udp
ENTRYPOINT ["/usr/local/bin/otm"]
