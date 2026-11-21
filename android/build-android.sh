#!/usr/bin/env bash
# Builds the Android app: the desktop app's Go backend for each Android ABI
# (with the NDK, since Go's DNS needs cgo on Android), packed as
# lib/<abi>/libmdgui.so, then the APK around it.
#
#   ANDROID_HOME=<sdk> ANDROID_NDK_HOME=<ndk> android/build-android.sh <build number>
#
# PRODUCT_VERSION (25.3.0.<n>) and TAG (v25.3.0-series-folders.<n>) follow the
# build number unless set. MD_KEYSTORE (and MD_KEYSTORE_PASSWORD) sign it with
# a kept key, so each build installs over the last; without, the debug key.
set -euo pipefail

n=${1:?usage: build-android.sh <build number>}
here=$(cd "$(dirname "$0")" && pwd)
repo=$(dirname "$here")
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to the Android NDK}"
PRODUCT_VERSION=${PRODUCT_VERSION:-25.3.0.$n}
TAG=${TAG:-v25.3.0-series-folders.$n}
API=24

# The frontend is built for Chrome 83 (Android 11's own WebView), not the
# desktop's newer baseline, so a WebView that hasn't been updated still runs
# it. This leaves gui/frontend/dist with that build; make build/gui/frontend
# makes the desktop's again.
(
	cd "$repo/gui/frontend"
	pnpm install --frozen-lockfile
	pnpm exec tsc --noEmit
	pnpm exec vite build --target chrome83
)

toolchain="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin"
version=$(git -C "$repo" rev-parse --short HEAD)
jni="$here/app/src/main/jniLibs"
rm -rf "$jni"

for pair in arm64-v8a:arm64:aarch64 x86_64:amd64:x86_64; do
	IFS=: read -r abi goarch triple <<<"$pair"
	mkdir -p "$jni/$abi"
	(
		cd "$repo/gui"
		GOOS=android GOARCH=$goarch CGO_ENABLED=1 CC="$toolchain/${triple}-linux-android${API}-clang" \
			go build -trimpath -tags nodynamic \
			-ldflags "-s -w -X 'main.Version=$version' -X 'main.Tag=$TAG'" \
			-o "$jni/$abi/libmdgui.so" .
	)
	echo "built $abi"
done

cd "$here"
gradle --no-daemon -q assembleRelease -PmdVersionCode="$n" -PmdVersionName="$PRODUCT_VERSION"
echo "$here/app/build/outputs/apk/release/app-release.apk"
