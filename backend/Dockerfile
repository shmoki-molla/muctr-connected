FROM golang:1.23-alpine AS build
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -o /muctr-drive-api .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /muctr-drive-api /usr/local/bin/muctr-drive-api
EXPOSE 8080
CMD ["muctr-drive-api"]