#!/bin/bash
# ============================================================
# Push a video file to MediaMTX for demo/testing
# Usage: ./test-video.sh <video_file> [stream_name]
# Example: ./test-video.sh demo-video.mp4 demo
# ============================================================

VIDEO_FILE="${1}"
STREAM_NAME="${2:-demo}"
SERVER="localhost"
RTSP_PORT=8554

if [ -z "$VIDEO_FILE" ]; then
  echo "❌ Usage: $0 <video_file> [stream_name]"
  echo ""
  echo "   Examples:"
  echo "     $0 demo-video.mp4"
  echo "     $0 demo-video.mp4 demo"
  echo "     $0 /path/to/clip.mp4 lobby-cam"
  exit 1
fi

if [ ! -f "$VIDEO_FILE" ]; then
  echo "❌ File not found: $VIDEO_FILE"
  exit 1
fi

echo "============================================"
echo "  🎬 Video File → MediaMTX Test Stream"
echo "============================================"
echo ""
echo "  File        : ${VIDEO_FILE}"
echo "  Stream name : ${STREAM_NAME}"
echo "  Pushing to  : rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
echo ""
echo "  View stream:"
echo "    WebRTC : http://${SERVER}:8889/${STREAM_NAME}"
echo "    HLS    : http://${SERVER}:8888/${STREAM_NAME}"
echo "    RTSP   : rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
echo ""
echo "  ♻️  Video will loop infinitely"
echo "  Press Ctrl+C to stop"
echo "============================================"
echo ""

# Check if the video has an audio stream
HAS_AUDIO=$(ffprobe -v error -select_streams a -show_entries stream=codec_type -of csv=p=0 "$VIDEO_FILE" 2>/dev/null | head -1)

if [ -n "$HAS_AUDIO" ]; then
  # Video WITH audio → transcode audio to Opus for WebRTC compatibility
  echo "🔊 Audio detected → encoding with Opus"
  echo ""
  ffmpeg -stream_loop -1 \
    -re \
    -i "$VIDEO_FILE" \
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
else
  # Video WITHOUT audio
  echo "🔇 No audio track → video only"
  echo ""
  ffmpeg -stream_loop -1 \
    -re \
    -i "$VIDEO_FILE" \
    -pix_fmt yuv420p \
    -c:v libx264 \
    -preset ultrafast \
    -tune zerolatency \
    -profile:v baseline \
    -b:v 2500k \
    -maxrate 2500k \
    -bufsize 5000k \
    -g 60 \
    -an \
    -f rtsp \
    -rtsp_transport tcp \
    "rtsp://${SERVER}:${RTSP_PORT}/${STREAM_NAME}"
fi
