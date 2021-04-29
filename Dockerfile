FROM  golang:1.15-buster as builder

WORKDIR /tmp/tiny-golang-image
COPY cmd ./cmd/
COPY go.mod .
COPY go.sum .

RUN go mod tidy && go get -u -d -v ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-s -w' -o main ./cmd/

FROM scratch
COPY --from=builder /tmp/tiny-golang-image /
ENTRYPOINT ["/main"]