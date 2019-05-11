FROM golang:1.12-alpine AS builder

ENV GO111MODULE=on
RUN apk --no-cache add git
WORKDIR $GOPATH/src/github.com/dennisv/internaldns
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /internaldns .

FROM scratch
COPY --from=builder /internaldns ./
ENTRYPOINT ["/internaldns"]
