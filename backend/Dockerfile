FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o /muctr-drive-api .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /muctr-drive-api /usr/local/bin/muctr-drive-api
USER app
EXPOSE 8080
CMD ["muctr-drive-api"]
