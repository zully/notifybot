FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ircd-ircu \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/ircd && chown irc:irc /run/ircd
COPY ircd.conf /etc/ircu/ircd.conf
USER irc
ENTRYPOINT ["/usr/sbin/ircd-ircu", "-n", "-f", "/etc/ircu/ircd.conf"]
