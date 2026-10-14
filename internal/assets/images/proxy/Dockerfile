FROM alpine:3.20.10
RUN apk add --no-cache squid
COPY squid.conf /etc/squid/squid.conf
COPY jailwick-proxy /usr/local/bin/jailwick-proxy
RUN chmod 0755 /usr/local/bin/jailwick-proxy
USER squid
EXPOSE 3128
CMD ["/usr/local/bin/jailwick-proxy", "run"]
