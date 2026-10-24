FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
ARG SERVICE
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY services/${SERVICE}/go.mod services/${SERVICE}/go.sum ./
RUN go mod download
COPY services/${SERVICE}/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/${SERVICE}
RUN if [ "$SERVICE" = auth ]; then CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate; fi
RUN if [ "$SERVICE" = core ]; then CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go install -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.29.0; find /go/bin -type f -name goose -exec cp {} /out/goose \;; cp -r migrations /out/migrations; fi

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/ /app/
USER 65532:65532
ENTRYPOINT ["/app/server"]
