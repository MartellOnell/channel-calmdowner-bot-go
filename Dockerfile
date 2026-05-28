FROM public.ecr.aws/docker/library/golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot

FROM public.ecr.aws/docker/library/alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /bot /usr/local/bin/bot
ENTRYPOINT ["/usr/local/bin/bot"]
