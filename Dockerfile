FROM alpine:3.9

ADD . .

RUN apk update && apk add ca-certificates

CMD ["./linux"]
