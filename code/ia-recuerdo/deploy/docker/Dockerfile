FROM golang:1.25-alpine AS builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build \
    -tags postgres \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /ia-recuerdo ./cmd/ia-recuerdo/

FROM alpine:3.21
RUN addgroup -S ia-recuerdo && adduser -S -G ia-recuerdo ia-recuerdo
COPY --from=builder /ia-recuerdo /usr/local/bin/ia-recuerdo
USER ia-recuerdo
EXPOSE 7438
ENTRYPOINT ["ia-recuerdo"]
CMD ["-transport", "http", "-addr", ":7438"]
