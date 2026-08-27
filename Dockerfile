# Agentic Identity Broker - Production Docker Image
# This Dockerfile packages pre-built backend and frontend artifacts
ARG BASE_IMAGE=default
FROM alpine:3 AS default
FROM ${BASE_IMAGE}

# Build arguments for multi-architecture support via docker buildx
# When using: docker buildx build --platform linux/amd64,linux/arm64 ...
# TARGETARCH is automatically set to: amd64, arm64, etc.
ARG TARGETARCH

ARG VERSION

# Create non-root user for security (uid=1000, gid=1000)
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Copy pre-built backend binary from architecture-specific directory
# Supports multi-architecture builds via docker buildx
# TARGETARCH automatically set to: amd64, arm64, etc.
# Binary should be built with: just build-linux-amd64 or just build-linux-arm64
COPY --chown=1000:1000 ./bin/linux/${TARGETARCH}/agentic-identity-broker /app/agentic-identity-broker

# Copy pre-built frontend assets from ./web/dist/
# Assumes frontend is already built and available at ./web/dist/consent/
COPY --chown=1000:1000 ./web/dist/consent /app/web/dist/consent

# Ensure binary is executable
RUN chmod +x /app/agentic-identity-broker

# Expose backend service ports
EXPOSE 8000 14000

# Switch to non-root user for container execution
USER 1000:1000

# Application entrypoint
ENTRYPOINT ["/app/agentic-identity-broker"]

# Default to empty CMD; arguments can be passed at runtime
CMD []
