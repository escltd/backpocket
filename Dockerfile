FROM golang:1.21.3-bullseye AS golang-base

WORKDIR /go/src/app/
COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app.elf && chmod +x app.elf


FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /deploy
COPY --from=golang-base /go/src/app/app.elf .
# COPY --from=golang-base /go/src/app/*.xlsx ./
COPY --from=golang-base /go/src/app/*.pem ./

CMD ["./app.elf"]  