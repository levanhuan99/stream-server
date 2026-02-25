#!/bin/bash
# ============================================================
# Push MacBook Camera to MediaMTX for testing
# Usage: ./test-camera.sh [stream_name]
# ============================================================

STREAM_NAME="${1:-macbook}"
SERVER="localhost"
RTSP_PORT=8554

echo "============================================"
echo "  📹 MacBook Camera → MediaMTX Test Stream"
echo "============================================"
echo ""
echo "  Stream name : ${STREAM_NAME}"
echo "  Pushing to  : rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
echo ""
echo "  View stream:"
echo "    WebRTC : http://${SERVER}:8889/${STREAM_NAME}"
echo "    HLS    : http://${SERVER}:8888/${STREAM_NAME}"
echo "    RTSP   : rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
echo ""
echo "  Press Ctrl+C to stop"
echo "============================================"
echo ""

# FFmpeg: capture FaceTime camera (device 0) + microphone (device 0)
# Push to MediaMTX via RTSP (supports Opus audio, unlike RTMP which only supports AAC)
# -pix_fmt yuv420p is required for WebRTC compatibility (422 is not supported)
# -c:a libopus is required for WebRTC audio (AAC is skipped by WebRTC)
ffmpeg -f avfoundation \
  -framerate 30 \
  -video_size 1280x720 \
  -capture_cursor 0 \
  -i "0:0" \
  -pix_fmt yuv420p \
  -c:v libx264 \
  -preset ultrafast \
  -tune zerolatency \
  -profile:v baseline \
  -b:v 2500k \
  -maxrate 2500k \
  -bufsize 5000k \
  -g 60 \
  -c:a libopus \
  -b:a 128k \
  -ar 48000 \
  -f rtsp \
  -rtsp_transport tcp \
  "rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
