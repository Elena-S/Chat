FROM golang:1.20 AS debug

WORKDIR /usr/src/app

COPY . .

RUN go mod download
RUN go mod tidy
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.10.0
RUN go install github.com/go-delve/delve/cmd/dlv@v1.20.1
# RUN go install github.com/cespare/reflex@latest
# CMD reflex -R "__debug_bin" -s -- sh -c "dlv debug --listen=:2345 --headless --accept-multiclient --log --api-version=2 --disable-aslr"


FROM debug AS build
WORKDIR /usr/src/app/cmd/server
RUN go build -o server ./main.go


FROM debian AS prod
COPY --from=build ["/usr/src/app/cmd/server/server", "/usr/src/app/cmd/server/"]
EXPOSE 8000
WORKDIR /usr/src/app/cmd/server
ENTRYPOINT ./server
