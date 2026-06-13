#!/usr/bin/env bash
set -euo pipefail

ENGINE_SRC="${VELOX_VIDEO_ENGINE_SRC:-/app/native/video-engine-cpp}"
OUT_BIN="${VELOX_VIDEO_ENGINE_OUT:-/usr/local/bin/velox_video_engine}"

echo "== Velox C++ engine build =="
echo "Source: $ENGINE_SRC"
echo "Output: $OUT_BIN"

if [ ! -d "$ENGINE_SRC" ]; then
  echo "ERROR: C++ engine source directory not found: $ENGINE_SRC" >&2
  exit 10
fi

find "$ENGINE_SRC" -type f \( \
  -name "velox_video_engine" -o \
  -name "video_engine" -o \
  -name "*.so" \
\) -delete || true

cd "$ENGINE_SRC"

if [ -f "CMakeLists.txt" ]; then
  echo "Detected CMake project"
  rm -rf build
  cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
  cmake --build build -j"$(nproc)"

  if [ -f "build/velox_video_engine" ]; then
    install -m 0755 build/velox_video_engine "$OUT_BIN"
  elif [ -f "build/video_engine" ]; then
    install -m 0755 build/video_engine "$OUT_BIN"
  else
    echo "ERROR: CMake build completed but engine binary was not found" >&2
    find build -maxdepth 3 -type f -perm -111 -print
    exit 11
  fi

elif [ -f "Makefile" ] || [ -f "makefile" ]; then
  echo "Detected Makefile project"
  make clean || true
  make -j"$(nproc)"

  if [ -f "./velox_video_engine" ]; then
    install -m 0755 ./velox_video_engine "$OUT_BIN"
  elif [ -f "./video_engine" ]; then
    install -m 0755 ./video_engine "$OUT_BIN"
  else
    echo "ERROR: Make build completed but engine binary was not found" >&2
    find . -maxdepth 3 -type f -perm -111 -print
    exit 12
  fi

else
  echo "ERROR: no CMakeLists.txt or Makefile found in $ENGINE_SRC" >&2
  exit 13
fi

echo "== Built binary =="
ls -lh "$OUT_BIN"
file "$OUT_BIN" || true
ldd "$OUT_BIN" || true
