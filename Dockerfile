FROM golang:1.22 as builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY . ./

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o nest .

FROM alpine:3.19
WORKDIR /
COPY --from=builder /workspace/nest .
RUN apk add --no-cache curl
USER 65532:65532
HEALTHCHECK CMD curl --fail http://localhost:8080/health

ENTRYPOINT ["/nest"]