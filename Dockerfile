# One container, one process: the helpdesk binary with PocketBase and the SPA
# inside it.
#
#   docker build -t helpdesk .
#   docker run -d --name helpdesk -p 8090:8090 -v helpdesk-data:/data helpdesk
#
# Nothing is required on first boot. The app seeds a staff admin
# (admin@helpdesk.local) and prints its password ONCE to stdout, so read it
# with `docker logs helpdesk` and change it after signing in. That is the whole
# of first-run setup; SMTP and the application URL are PocketBase settings, set
# in the dashboard at /_ afterwards.
#
# Two stages rather than the platform's three: it builds its Vue console here
# because it commits a placeholder, where this repo commits the built SPA under
# internal/webui/public. The bundle in the tag is the bundle that ships, so
# there is no Node in this file at all.

# ---------------------------------------------------------------- 1. binary
# --platform=$BUILDPLATFORM: with CGO_ENABLED=0 the Go toolchain cross-compiles
# for free, so running an emulated compiler under QEMU to produce the same
# static binary is pure cost. TARGETOS/TARGETARCH come from buildx, one value
# per target leg.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src

# Dependencies first, so a source-only change does not re-download the module
# cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# VERSION is passed rather than read from git, because .dockerignore excludes
# .git on purpose: without it `debug.ReadBuildInfo` has nothing to read and an
# image tagged v0.1.0 would report "(devel)" when asked. See the kit's version package.
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -trimpath \
      -ldflags "-s -w -X github.com/stone-age-io/kit/version.stamped=${VERSION}" \
      -o /out/helpdesk ./cmd/helpdesk

# --------------------------------------------------------------- 2. runtime
FROM alpine:3

# ca-certificates is for outbound TLS: SMTP, the NATS hub, and the email
# provider's webhook replies. tzdata because work dates, visit times and the
# auto-close cron are all read by people in a local timezone.
#
# The floating `alpine:3` tag rather than a pinned minor, on purpose: it always
# resolves to current stable, so the image picks up base security fixes without
# anyone here watching Alpine release notes.
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 helpdesk

COPY --from=build /out/helpdesk /usr/local/bin/helpdesk

# The SQLite database and uploaded attachments. One volume covers both:
# PocketBase keeps storage/ inside its data directory.
# /creds holds what the platform keeps current (platform.url): the creds
# file, the session and a device-held seed. It is a volume of its own because
# /data is PocketBase's data_dir, and PocketBase backups archive all of it.
RUN mkdir -p /data /creds && chown helpdesk /data /creds && chmod 700 /creds
VOLUME /data
VOLUME /creds
USER helpdesk

ENV HELPDESK_DATA_DIR=/data
EXPOSE 8090

# /api/ready rather than PocketBase's /api/health, which reports only that the
# HTTP server is listening — true through every failure worth catching. This
# answers 503 when the database is unreachable or the schema never applied.
#
# It stays healthy on a `warn`, which is deliberate and matches the endpoint's
# contract: unset SMTP and a default application URL are real problems, and
# neither is fixed by restarting or by removing the container from a pool.
#
# busybox wget is already in the base image and exits non-zero on 503, so this
# needs no curl and no shell arithmetic.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8090/api/ready || exit 1

# No entrypoint script. The platform needs one because `serve --nats` cannot
# start until an operator exists in the database, so first boot has four
# ordered commands to run; here the record hooks seed what they need and serve
# is the only command.
ENTRYPOINT ["/usr/local/bin/helpdesk"]
CMD ["serve", "--http=0.0.0.0:8090"]
