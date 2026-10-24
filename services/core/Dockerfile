FROM golang:1.27.3-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src

COPY services/core/go.mod services/core/go.sum* ./
RUN go mod download
COPY services/core/ ./
RUN CGO_ENABLED=0 go build ./...

FROM build AS binary
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/core ./cmd/core

FROM scratch AS runtime
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=binary /out/core /core
COPY LICENSE /LICENSE
USER 65532:65532
ENTRYPOINT ["/core"]
