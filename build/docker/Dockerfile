# Agentic Identity Broker - Production Docker Image
# This Dockerfile packages pre-built backend and frontend artifacts
# BASE_IMAGE lets internal release pipelines override the public default
# with an internally-mirrored/allowed base image (see delivery.yaml).
ARG BASE_IMAGE=alpine:3@sha256:5b02b42e375f7426f8d65c3af331ca05d9878f9989230354504e0b9dfd431f60
FROM ${BASE_IMAGE}

# Build arguments for multi-architecture support via docker buildx
# When using: docker buildx build --platform linux/amd64,linux/arm64 ...
# TARGETARCH is automatically set to: amd64, arm64, etc.
ARG TARGETARCH

ARG VERSION
ARG REVISION
ARG CREATED

LABEL org.opencontainers.image.created="${CREATED}" \
      org.opencontainers.image.title="Agentic Identity Broker" \
      org.opencontainers.image.description="Identity broker for AI agents with OAuth2 delegation and consent management" \
      org.opencontainers.image.vendor="Zalando SE" \
      org.opencontainers.image.authors="Magnus Jungsbluth <magnus.jungsbluth@zalando.de>, Jan Brennenstuhl <jan.brennenstuhl@zalando.de>" \
      org.opencontainers.image.url="https://github.com/zalando-incubator/agentic-identity-broker" \
      org.opencontainers.image.documentation="https://github.com/zalando-incubator/agentic-identity-broker#readme" \
      org.opencontainers.image.source="https://github.com/zalando-incubator/agentic-identity-broker" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

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

# Copy pre-built frontend assets from ./web/dist/.
# Assumes frontend is already built and available at ./web/dist/.
COPY --chown=1000:1000 ./web/dist /app/web/dist

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
