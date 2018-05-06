FROM alpine
LABEL maintainer="fabio@rapposelli.org"
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

COPY nocino /
WORKDIR /
ENTRYPOINT ["/nocino"]