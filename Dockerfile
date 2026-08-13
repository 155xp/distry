FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /distry .

FROM debian:bookworm-slim AS wasmtime
ARG WASMTIME_VERSION=45.0.0
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl xz-utils && \
    curl -fsSL "https://github.com/bytecodealliance/wasmtime/releases/download/v${WASMTIME_VERSION}/wasmtime-v${WASMTIME_VERSION}-x86_64-linux.tar.xz" | \
    tar -xJ --strip-components=1 -C /usr/local/bin "wasmtime-v${WASMTIME_VERSION}-x86_64-linux/wasmtime"

FROM debian:bookworm-slim
COPY --from=build /distry /usr/local/bin/distry
COPY --from=wasmtime /usr/local/bin/wasmtime /usr/local/bin/wasmtime
USER 65532:65532
ENTRYPOINT ["distry", "coordinator"]
