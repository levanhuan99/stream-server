#!/bin/bash
# ============================================================
# Push MacBook Camera (video only, no audio) to MediaMTX
# Lighter version — no microphone permission needed
# Usage: ./test-camera-noaudio.sh [stream_name]
# ============================================================

STREAM_NAME="${1:-macbook}"
SERVER="localhost"
RTSP_PORT=8554

echo "============================================"
echo "  📹 MacBook Camera → MediaMTX (no audio)"
echo "============================================"
echo ""
echo "  Stream name : ${STREAM_NAME}"
echo "  Pushing to  : rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
echo ""
echo "  View stream:"
echo "    WebRTC : http://${SERVER}:8889/${STREAM_NAME}"
echo "    HLS    : http://${SERVER}:8888/${STREAM_NAME}"
echo ""
echo "  Press Ctrl+C to stop"
echo "============================================"
echo ""

# Video only — no microphone
ffmpeg -f avfoundation \
  -framerate 30 \
  -video_size 1280x720 \
  -i "0:none" \
  -pix_fmt yuv420p \
  -c:v libx264 \
  -preset ultrafast \
  -tune zerolatency \
  -profile:v baseline \
  -b:v 2000k \
  -maxrate 2000k \
  -bufsize 4000k \
  -g 60 \
  -an \
  -f rtsp \
  -rtsp_transport tcp \
  "rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
