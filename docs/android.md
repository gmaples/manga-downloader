# Android

This fork runs on Android two ways: the command line in Termux, and an app
that shows the desktop app's window.

## The app

The desktop app's window is a web page (`gui/frontend`) that talks to the Go
backend (`gui/`) through Wails. Wails has no Android target, so on Android:

- The Go backend is built for Android with the NDK (`gui/main_bridge.go`, the
  `android` build) and packed in the APK as `lib/<abi>/libmdgui.so`. The app
  runs it as a program, so it's extracted on install.
- The backend serves the window on `127.0.0.1` at a random port
  (`gui/bridge.go`). `bridge.js`, put first in the page, gives the page the
  `window.go` and `window.runtime` that Wails would, over HTTP: each call is a
  POST to `/call/<Method>`, and the backend's events come over server-sent
  events from `/events`. Calls and events need the token the app starts the
  backend with, so other apps on the device can't drive it.
- The app (`android/`) shows the page in a WebView and keeps the backend
  running in a foreground service (its notification has a Quit button), so
  downloads go on with the screen off or the app in the background. Back moves
  the app to the background rather than closing it.
- What needs the system goes through `window.MDAndroid`: Browse… opens
  Android's folder picker, Open folder opens the Files app on the folder,
  links open in the browser, and Paste reads the clipboard.

Files are saved to `Download/Manga` until another folder is picked. Android 11
and later ask once to allow access to all files, which saving outside the
app's own storage needs.

What doesn't work on Android: the sites that need a browser window (the ones
the desktop app opens Chrome for), and the in-app update, which installs
desktop packages; the update notice links to the release instead.

## Building

Needs Go, the Android SDK (platform 35, build-tools 35), the NDK (r28c was
used), JDK 17 or later and Gradle 8.13 or later.

```bash
ANDROID_HOME=<sdk> ANDROID_NDK_HOME=<ndk> android/build-android.sh <build number>
```

The script builds the frontend itself, for Chrome 83 (the WebView Android 11
ships with) rather than the desktop's newer baseline, so a device whose WebView
was never updated still runs it; `bridge.js` adds `Object.hasOwn`, the one
newer function the frontend uses. Run `make build/gui/frontend` afterwards
before a desktop build.

If Android has a proxy set (Settings, or `settings put global http_proxy`),
the app passes it to the backend; a PAC proxy isn't followed.

The build number sets the app's version code, `PRODUCT_VERSION`
(`25.2.0.<n>`) and `TAG` (`v25.2.0-series-folders.<n>`). With `MD_KEYSTORE`
and `MD_KEYSTORE_PASSWORD` set, the APK is signed with that key, so a new build
installs over the last one; without them it's signed with the debug key.

To try the bridge without Android, build the backend for the desktop with the
`bridgedev` tag and open the page in a browser:

```bash
cd gui && go build -tags bridgedev,nodynamic -o /tmp/mdgui . && /tmp/mdgui -token t -addr 127.0.0.1:8765
# then open http://127.0.0.1:8765/?t=t
```
