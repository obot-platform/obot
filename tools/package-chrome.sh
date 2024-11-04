set -e
mkdir -p /opt/google/chrome

if [ $(uname -m) = aarch64 ]; then
    apk add --no-cache \
        curl \
        font-opensans \
        fontconfig \
        gtk-3 \
        icu-data-full \
        libnss \
        mesa \
        nss \
        so:libFLAC.so.12 \
        so:libQt5Core.so.5 \
        so:libQt5Gui.so.5 \
        so:libQt5Widgets.so.5 \
        so:libQt6Core.so.6 \
        so:libQt6Gui.so.6 \
        so:libQt6Widgets.so.6 \
        so:libX11.so.6 \
        so:libXcomposite.so.1 \
        so:libXdamage.so.1 \
        so:libXext.so.6 \
        so:libXfixes.so.3 \
        so:libXrandr.so.2 \
        so:libasound.so.2 \
        so:libatk-1.0.so.0 \
        so:libatk-bridge-2.0.so.0 \
        so:libatspi.so.0 \
        so:libbrotlidec.so.1 \
        so:libc.so.6 \
        so:libcairo.so.2 \
        so:libcrc32c.so.1 \
        so:libcups.so.2 \
        so:libdav1d.so.7 \
        so:libdbus-1.so.3 \
        so:libdouble-conversion.so.3 \
        so:libdrm.so.2 \
        so:libevent-2.1.so.7 \
        so:libexpat.so.1 \
        so:libffi.so.8 \
        so:libfontconfig.so.1 \
        so:libfreetype.so.6 \
        so:libgbm.so.1 \
        so:libgcc_s.so.1 \
        so:libgio-2.0.so.0 \
        so:libglib-2.0.so.0 \
        so:libgobject-2.0.so.0 \
        so:libharfbuzz-subset.so.0 \
        so:libharfbuzz.so.0 \
        so:libicui18n.so.75 \
        so:libicuuc.so.75 \
        so:libjpeg.so.8 \
        so:liblcms2.so.2 \
        so:libm.so.6 \
        so:libminizip.so.1 \
        so:libopenh264.so.7 \
        so:libopus.so.0 \
        so:libpango-1.0.so.0 \
        so:libpulse.so.0 \
        so:libstdc++.so.6 \
        so:libudev.so.1 \
        so:libwebp.so.7 \
        so:libwebpdemux.so.2 \
        so:libwebpmux.so.3 \
        so:libxcb.so.1 \
        so:libxkbcommon.so.0 \
        so:libxml2.so.2 \
        so:libxslt.so.1 \
        so:libz.so.1 \
        so:libzstd.so.1 \
        systemd \
        xdg-utils
    cd /
    curl -O https://playwright.azureedge.net/builds/chromium/1140/chromium-linux-arm64.zip
    unzip chromium-linux-arm64.zip
    rm chromium-linux-arm64.zip
    ln -sf /chrome-linux/chrome /opt/google/chrome/chrome
else
    apk add chromium
    ln -sf /usr/bin/chromium-browser /opt/google/chrome/chrome
fi
