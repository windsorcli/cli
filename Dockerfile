# Stage 1: Aqua Installer
# -----------------------
FROM alpine:3.21.3 AS aqua

# Set environment variables
ENV PATH="/root/.local/share/aquaproj-aqua/bin:$PATH"
ENV AQUA_GLOBAL_CONFIG=/etc/aqua/aqua.yaml

# renovate: datasource=github-releases depName=aquaproj/aqua-installer
ARG AQUA_INSTALLER_VERSION=v3.1.1
# renovate: datasource=github-releases depName=aquaproj/aqua
ARG AQUA_VERSION=v2.46.0

# Update package index and install dependencies
RUN apk update && apk add bash wget --no-cache wget

# Copy aqua configuration
COPY aqua.docker.yaml /etc/aqua/aqua.yaml

# Install Aqua and tools from aqua.docker.yaml using wget instead of curl
RUN wget -q https://raw.githubusercontent.com/aquaproj/aqua-installer/${AQUA_INSTALLER_VERSION}/aqua-installer -O aqua-installer && \
    echo "e9d4c99577c6b2ce0b62edf61f089e9b9891af1708e88c6592907d2de66e3714  aqua-installer" | sha256sum -c - && \
    chmod +x aqua-installer && \
    ./aqua-installer -v ${AQUA_VERSION} && \
    aqua i && \
    aqua cp -o /dist kubectl talosctl terraform && \
    rm aqua-installer

# Stage 2: Builder
# ----------------
FROM --platform=$BUILDPLATFORM golang:1.24.1-alpine AS builder

# Install dependencies
RUN apk add --no-cache git

# Build the windsor binary
COPY . .
RUN go build -o /work/windsor ./cmd/windsor

# Stage 3: Runtime
# ----------------
FROM alpine:3.21.3

# Install runtime dependencies
RUN apk add --no-cache bash git wget unzip

# Copy tools from aqua-installer
COPY --from=aqua /dist/* /usr/local/bin/

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S windsor -G appgroup

# Switch to windsor user
USER windsor

# Copy windsor binary
COPY --from=builder /work/windsor /usr/local/bin/

# Create the .trusted file and add the file pointing to /work
RUN mkdir -p /home/windsor/.config/windsor && echo "/work" > /home/windsor/.config/windsor/.trusted

# Set working directory
WORKDIR /work

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/windsor", "exec", "--"]
