# 📹 CCTV Stream Server — MediaMTX + Docker

Máy chủ restream CCTV sử dụng [MediaMTX](https://github.com/bluenviron/mediamtx) (formerly rtsp-simple-server), hỗ trợ xem trực tiếp trên web qua **WebRTC** (low-latency) và **HLS** (fallback).

## 📐 Kiến trúc

```
┌──────────────┐     RTSP Pull     ┌─────────────┐     WebRTC (0.5s)     ┌─────────┐
│  Camera CCTV │ ──────────────▶   │  MediaMTX   │ ───────────────────▶  │ Browser │
│  (RTSP out)  │                   │  (Docker)   │ ───────────────────▶  │  / App  │
└──────────────┘                   └─────────────┘     HLS/LL-HLS (1-3s) └─────────┘
                                        │
                                        ├── RTSP  :8554 (re-publish)
                                        ├── RTMP  :1935 (OBS, FFmpeg)
                                        ├── HLS   :8888 (web fallback)
                                        ├── WebRTC:8889 (web primary)
                                        └── API   :9997 (management)
```

## 📁 Cấu trúc thư mục

```
stream-server/
├── docker-compose.yml          # Docker compose config
├── mediamtx.yml                # MediaMTX configuration
├── .env.example                # Environment variables template
├── recordings/                 # Video recordings (if enabled)
├── web/
│   ├── index.html              # Multi-camera WebUI dashboard
│   └── embed-example.html      # Ví dụ nhúng camera vào web
└── Readme.md
```

## 🚀 Khởi chạy nhanh

### 1. Cấu hình camera

Sửa file `mediamtx.yml`, phần `paths:` — thay URL RTSP bằng camera thực:

```yaml
paths:
  cam1:
    source: rtsp://admin:password@192.168.1.101:554/Streaming/Channels/101
  cam2:
    source: rtsp://admin:password@192.168.1.102:554/Streaming/Channels/101
```

**URL RTSP theo hãng camera:**

| Hãng      | URL Pattern                                                    |
|-----------|----------------------------------------------------------------|
| Hikvision | `rtsp://USER:PASS@IP:554/Streaming/Channels/101`              |
| Dahua     | `rtsp://USER:PASS@IP:554/cam/realmonitor?channel=1&subtype=0` |
| Reolink   | `rtsp://USER:PASS@IP:554/h264Preview_01_main`                 |
| Generic   | `rtsp://USER:PASS@IP:554/stream1`                             |

### 2. Chạy Docker

```bash
# Clone & cd vào thư mục
cd stream-server

# Khởi chạy
docker compose up -d

# Xem logs
docker compose logs -f mediamtx

# Dừng
docker compose down
```

### 3. Xem stream

| Protocol   | URL                              | Độ trễ  | Ghi chú                  |
|------------|----------------------------------|---------|--------------------------|
| **WebRTC** | `http://SERVER_IP:8889/cam1`     | ~0.5s   | ⭐ Khuyên dùng            |
| **HLS**    | `http://SERVER_IP:8888/cam1`     | ~1-3s   | Fallback cho Safari/mobile|
| **RTSP**   | `rtsp://SERVER_IP:8554/cam1`     | ~0.3s   | VLC, FFmpeg, NVR          |
| **RTMP**   | `rtmp://SERVER_IP:1935/cam1`     | ~1s     | OBS, FFmpeg               |

### 4. WebUI Dashboard

Mở [web/index.html](web/index.html) trong browser hoặc serve bằng bất kỳ HTTP server:

```bash
# Python
python3 -m http.server 8080 --directory web

# Hoặc npx
npx serve web -l 8080
```

Truy cập: `http://SERVER_IP:8080`

## 🌐 Nhúng vào Website

### WebRTC (iframe — đơn giản nhất)

```html
<iframe
  src="http://SERVER_IP:8889/cam1"
  width="640" height="360"
  allow="autoplay"
  style="border: none;">
</iframe>
```

### WebRTC (JavaScript WHEP API)

```html
<video id="myVideo" autoplay muted playsinline></video>
<script>
async function startWebRTC(videoEl, url) {
  const pc = new RTCPeerConnection({
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
  });
  pc.addTransceiver('video', { direction: 'recvonly' });
  pc.addTransceiver('audio', { direction: 'recvonly' });
  pc.ontrack = (evt) => { videoEl.srcObject = evt.streams[0]; };

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);

  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/sdp' },
    body: pc.localDescription.sdp,
  });
  await pc.setRemoteDescription({ type: 'answer', sdp: await res.text() });
}

startWebRTC(document.getElementById('myVideo'), 'http://SERVER_IP:8889/cam1/whep');
</script>
```

### HLS (video + HLS.js)

```html
<video id="hlsVideo" autoplay muted playsinline controls></video>
<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
<script>
const video = document.getElementById('hlsVideo');
const url = 'http://SERVER_IP:8888/cam1/index.m3u8';

if (video.canPlayType('application/vnd.apple.mpegurl')) {
  video.src = url; // Safari native
} else if (Hls.isSupported()) {
  const hls = new Hls({ lowLatencyMode: true });
  hls.loadSource(url);
  hls.attachMedia(video);
}
</script>
```

## 🛡️ WebRTC vs HLS — So sánh

| Tiêu chí        | WebRTC                    | HLS (LL-HLS)              |
|------------------|---------------------------|----------------------------|
| **Độ trễ**       | ~0.5s (gần real-time)     | ~1-3s (LL-HLS), ~6s (HLS) |
| **Browser**      | Chrome, Firefox, Edge, Safari 14+ | Tất cả               |
| **NAT/Firewall** | Cần STUN/TURN nếu qua NAT | Hoạt động qua HTTP chuẩn  |
| **CPU Server**   | Thấp (no transcoding)    | Thấp (no transcoding)      |
| **Mobile**       | Tốt                      | Rất tốt                    |
| **Khi nào dùng** | Dashboard giám sát real-time | Chia sẻ link, embed, mobile|

**→ Khuyến nghị: WebRTC làm primary, HLS làm fallback.**

## ⚙️ Cấu hình nâng cao

### Bật recording

Thêm vào `mediamtx.yml` trong `pathDefaults:` hoặc path cụ thể:

```yaml
pathDefaults:
  record: true
  recordPath: ./recordings/%path/%Y-%m-%d_%H-%M-%S-%f
  recordFormat: fmp4
  recordSegmentDuration: 1h
  recordDeleteAfter: 7d   # Tự xoá sau 7 ngày
```

### Pull on-demand (tiết kiệm bandwidth)

```yaml
paths:
  cam1:
    source: rtsp://admin:pass@192.168.1.101:554/stream1
    sourceOnDemand: true
    sourceOnDemandStartTimeout: 10s
    sourceOnDemandCloseAfter: 30s
```

### WebRTC qua NAT/Internet

```yaml
# mediamtx.yml
webrtcAdditionalHosts: ['YOUR_PUBLIC_IP']
webrtcICEServers2:
  - url: stun:stun.l.google.com:19302
  # Nếu cần TURN server:
  # - url: turn:turn.example.com:3478
  #   username: user
  #   password: pass
```

### HTTPS (production)

```yaml
# WebRTC
webrtcEncryption: true
webrtcServerKey: /certs/server.key
webrtcServerCert: /certs/server.crt

# HLS (required for LL-HLS on iOS)
hlsEncryption: true
hlsServerKey: /certs/server.key
hlsServerCert: /certs/server.crt
```

Mount certs trong `docker-compose.yml`:

```yaml
volumes:
  - ./certs:/certs:ro
```

## 🔍 Troubleshooting

| Vấn đề | Giải pháp |
|---------|-----------|
| WebRTC không kết nối được | Kiểm tra firewall port UDP 8189. Thêm `webrtcAdditionalHosts` nếu qua NAT |
| HLS trễ cao (~6s) | Đảm bảo `hlsVariant: lowLatency` trong config |
| Camera báo lỗi 401 | Kiểm tra user/pass RTSP. Một số camera cần encode URL |
| Stream đen/không hình | Kiểm tra `docker compose logs -f`, thử `rtspTransport: tcp` |
| Nhiều viewer → lag | Tăng `writeQueueSize`, kiểm tra bandwidth server |

## 📚 Tham khảo

- [MediaMTX GitHub](https://github.com/bluenviron/mediamtx)
- [MediaMTX Docker Hub](https://hub.docker.com/r/bluenviron/mediamtx)
- [WHEP Spec](https://www.ietf.org/archive/id/draft-murillo-whep-03.html)
