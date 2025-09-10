set -e
mkdir -p /opt/google/chrome
apk update
apk add chromium
ln -sf /usr/bin/chromium-browser /opt/google/chrome/chrome
