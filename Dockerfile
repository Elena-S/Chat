FROM golang:latest AS debug

WORKDIR /usr/src/app

COPY . .

RUN go install github.com/go-delve/delve/cmd/dlv@latest
# RUN go install github.com/cespare/reflex@latest
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

# CMD reflex -R "__debug_bin" -s -- sh -c "dlv debug --listen=:2345 --headless --accept-multiclient --log --api-version=2 --disable-aslr"
