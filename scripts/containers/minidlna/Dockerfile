FROM alpine:3.22

RUN apk add --no-cache minidlna=1.3.3-r1 && mkdir -p /media

COPY minidlna.conf /etc/minidlna.conf

CMD ["minidlnad", "-d", "-f", "/etc/minidlna.conf"]
