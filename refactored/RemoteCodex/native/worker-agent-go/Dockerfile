# ============================================================================
# Velox Worker - Multi-stage Build
# ============================================================================
# Stage 1: Builder — compiles C++ video engine against Debian 12 GLIBC 2.36
# Stage 2: Runtime — minimal image with ffmpeg + compiled engine binary
#
# Build:
#   cd RemoteCodex/native/worker-agent-go
#   make agent        # produce bin/velox-worker-agent
#   docker build -t velox-worker:latest .
#
# ============================================================================

# ── STAGE 1: Builder ───────────────────────────────────────────────────────
FROM debian:bookworm-slim AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -qq && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    g++ \
    pkg-config \
    ca-certificates \
    file \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy C++ engine source and build script (paths relative to RemoteCodex/ build context)
COPY scripts/build-video-engine.sh /usr/local/bin/build-video-engine.sh
COPY native/video-engine-cpp /build/native/video-engine-cpp

RUN chmod +x /usr/local/bin/build-video-engine.sh && \
    VELOX_VIDEO_ENGINE_SRC=/build/native/video-engine-cpp \
    VELOX_VIDEO_ENGINE_OUT=/usr/local/bin/velox_video_engine \
    /usr/local/bin/build-video-engine.sh

# Verify the binary is linked against container GLIBC
RUN echo "=== Builder GLIBC ===" && \
    getconf GNU_LIBC_VERSION && \
    echo "=== Engine binary ===" && \
    ls -lh /usr/local/bin/velox_video_engine && \
    file /usr/local/bin/velox_video_engine && \
    echo "=== Engine ldd ===" && \
    ldd /usr/local/bin/velox_video_engine && \
    echo "=== Max GLIBC symbol ===" && \
    strings /usr/local/bin/velox_video_engine | grep -oE 'GLIBC_[0-9.]+' | sort -Vu | tail -n 5

# ── STAGE 2: Runtime ───────────────────────────────────────────────────────
FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV VELOX_VIDEO_ENGINE_BIN=/usr/local/bin/velox_video_engine

WORKDIR /app

# Runtime dependencies only: ffmpeg + minimal C++ runtime
RUN apt-get update -qq && apt-get install -y --no-install-recommends \
    ffmpeg \
    curl \
    ca-certificates \
    libstdc++6 \
    libc6 \
    && rm -rf /var/lib/apt/lists/*

# Copy compiled engine from builder stage
COPY --from=builder /usr/local/bin/velox_video_engine /usr/local/bin/velox_video_engine
RUN chmod +x /usr/local/bin/velox_video_engine

# Copy Go worker agent binary (relative to RemoteCodex/ build context)
COPY native/worker-agent-go/bin/velox-worker-agent /usr/local/bin/velox-worker-agent

# Create non-root user for runtime
RUN useradd -r -m -s /usr/sbin/nologin velox && \
    chown -R velox:velox /app /usr/local/bin/velox-worker-agent /usr/local/bin/velox_video_engine

USER velox

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

ENTRYPOINT ["/usr/local/bin/velox-worker-agent"]
CMD ["--master", "http://master:8000"]
