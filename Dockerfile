FROM golang:1.27.1-alpine3.24@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN mkdir -p /out/data && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/bootstrap-admin ./cmd/bootstrap-admin && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/dbtool ./cmd/dbtool && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/maintain ./cmd/maintain && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG VERSION=dev
ARG VCS_REF=unknown
ARG CREATED=unknown
ARG SOURCE=local
LABEL org.opencontainers.image.title="Dynamis Code Apps Template" \
      org.opencontainers.image.description="Resource-conscious Go web application template" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$VCS_REF" \
      org.opencontainers.image.created="$CREATED" \
      org.opencontainers.image.source="$SOURCE"

COPY --from=build --chown=65532:65532 /out/server /server
COPY --from=build --chown=65532:65532 /out/bootstrap-admin /bootstrap-admin
COPY --from=build --chown=65532:65532 /out/dbtool /dbtool
COPY --from=build --chown=65532:65532 /out/maintain /maintain
COPY --from=build --chown=65532:65532 /out/healthcheck /healthcheck
COPY --from=build --chown=65532:65532 /out/data /data

USER 65532:65532
ENV HTTP_ADDRESS=0.0.0.0:8080 \
    SQLITE_PATH=/data/app.db
EXPOSE 8080
VOLUME ["/data"]
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/healthcheck"]
ENTRYPOINT ["/server"]
