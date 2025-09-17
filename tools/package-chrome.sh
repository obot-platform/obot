set -e
mkdir -p /opt/google/chrome
apk add --no-cache chromium
ln -sf /usr/bin/chromium-browser /opt/google/chrome/chrome
