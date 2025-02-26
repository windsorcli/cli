# Stage 1: Aqua Installer
# -----------------------
FROM alpine:3.21.3 AS aqua

# Set environment variables
ENV PATH="/root/.local/share/aquaproj-aqua/bin:$PATH"
ENV AQUA_GLOBAL_CONFIG=/etc/aqua/aqua.yaml

# renovate: datasource=github-releases depName=aquaproj/aqua-installer
ARG AQUA_INSTALLER_VERSION=v3.1.1
# renovate: datasource=github-releases depName=aquaproj/aqua
ARG AQUA_VERSION=v2.45.0

# Install dependencies
RUN apk add --no-cache curl bash

# Copy aqua configuration
COPY aqua.docker.yaml /etc/aqua/aqua.yaml

# Install Aqua and tools
RUN curl -sSfL -O https://raw.githubusercontent.com/aquaproj/aqua-installer/${AQUA_INSTALLER_VERSION}/aqua-installer && \
    echo "e9d4c99577c6b2ce0b62edf61f089e9b9891af1708e88c6592907d2de66e3714  aqua-installer" | sha256sum -c - && \
    chmod +x aqua-installer && \
    ./aqua-installer -v ${AQUA_VERSION} && \
    aqua i -a || { echo "Failed to install Aqua tools" >&2; exit 1; } && \
    aqua cp -o /dist aws aws_completer containerd containerd-shim-runc-v2 ctr docker docker-cli-plugin-docker-compose docker-init docker-proxy dockerd flux helm kubectl runc talosctl terraform || { echo "Failed to copy some tools" >&2; exit 1; } && \
    rm aqua-installer

# Stage 2: Builder
# ----------------
FROM --platform=$BUILDPLATFORM golang:1.23.4-alpine AS builder

# Install dependencies
RUN apk add --no-cache git

# Build the windsor binary
COPY . .
RUN go build -o /work/windsor ./cmd/windsor || { echo "Failed to build windsor binary" >&2; exit 1; }

# Stage 3: Runtime
# ----------------
FROM alpine:3.21.3

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S windsor -G appgroup

# Install runtime dependencies
RUN apk add --no-cache bash

# Copy tools from aqua-installer
COPY --from=aqua /dist/* /usr/local/bin/

# Create windsor user
USER windsor

# Copy windsor binary
COPY --from=builder /work/windsor /usr/local/bin/

# Set working directory
WORKDIR /work

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/windsor", "exec", "--"]
