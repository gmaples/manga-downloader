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
(`25.3.0.<n>`) and `TAG` (`v25.3.0-series-folders.<n>`). With `MD_KEYSTORE`
and `MD_KEYSTORE_PASSWORD` set, the APK is signed with that key, so a new build
installs over the last one; without them it's signed with the debug key.

To try the bridge without Android, build the backend for the desktop with the
`bridgedev` tag and open the page in a browser:

```bash
cd gui && go build -tags bridgedev,nodynamic -o /tmp/mdgui . && /tmp/mdgui -token t -addr 127.0.0.1:8765
# then open http://127.0.0.1:8765/?t=t
```

## Trying it in an emulator

Build 10 was tested in an Android 11 emulator (x86_64, no Google APIs, the
WebView it ships with is Chrome 83), made with the SDK's own tools:

```bash
sdkmanager "platform-tools" "emulator" "system-images;android-30;default;x86_64"
avdmanager create avd -n md30 -k "system-images;android-30;default;x86_64"
emulator -avd md30 -no-snapshot -memory 3072
adb install -r manga-downloader-v25.2.0-series-folders.<n>-android.apk
```

Without hardware acceleration (`-accel off`) it still runs, but slowly: it
took about 7 minutes to boot, and Android's own processes often stopped
answering in time.

If the emulator reaches the internet only through a proxy, set it in
Android, not with the emulator's `-http-proxy` option (that cut the emulator
off from the host at `10.0.2.2`), and restart the app afterwards:

```bash
adb shell settings put global http_proxy 10.0.2.2:<port>
```

## What was tested

Build 10 (`v25.2.0-series-folders.10`, commit d745ffe), in the emulator above:

- The app starts, the backend prints `READY <port>`, and the page shows the
  version.
- A MangaDex chapter link loads its series (GATE, 919 chapters, all
  languages listed).
- Downloading one chapter shows the per-site and overall bars, greys out
  Download while it runs, and saves a CBZ to `Download/Manga` whose 24 pages
  all open as images. A second run of the same chapter, with Skip existing
  files off, saved it again with ` v2` added to the name.
- Open folder opens the Files app on the folder, with the file listed.
- Browse… opens Android's folder picker, and Back cancels it with the folder
  unchanged.
- The command line (built the same way, for x86_64) downloads the same
  chapter, 24 of 24 pages.

Not tested yet: picking a folder in Browse… (the emulator stopped taking
taps in the picker), the Paste button, Quit in the notification, the layout
on a phone screen, and a real device.

## Trying it on a Windows PC

The same emulator runs on Windows, with the app in a window on the PC.
Everything below installs into one folder and needs no administrator rights.
It was set up this way on Windows 11 (AMD Ryzen, Windows Hypervisor Platform
present), where `emulator -accel-check` reported
`WHPX(10.0.26200) is installed and usable`. It takes about 10 GB.

1. Get a JDK 17 or later. A ZIP such as Eclipse Temurin 21 works; point
   `JAVA_HOME` at it.
2. Unzip the Android command-line tools (`commandlinetools-win-*_latest.zip`
   from developer.android.com) so that `sdk\cmdline-tools\latest\bin\sdkmanager.bat`
   exists, and set `ANDROID_HOME` to `sdk`.
3. Install the emulator, platform-tools and an image, accepting the licences:

   ```bash
   yes | sdkmanager --licenses
   sdkmanager "platform-tools" "emulator" "system-images;android-35;google_apis;x86_64"
   ```

4. Create a tablet AVD, keeping it in your folder with `ANDROID_AVD_HOME`:

   ```bash
   echo no | avdmanager create avd -n MangaTablet -k "system-images;android-35;google_apis;x86_64" -d pixel_tablet
   ```

   Set `hw.keyboard=yes` in `MangaTablet.avd\config.ini`, so the PC's
   keyboard types into the tablet.

5. Start it with a window:

   ```bat
   start "MangaTablet" "%ANDROID_HOME%\emulator\emulator.exe" -avd MangaTablet -gpu auto -no-snapshot-save
   ```

   Put the `set` lines for `JAVA_HOME`, `ANDROID_HOME`, `ANDROID_SDK_ROOT` and
   `ANDROID_AVD_HOME` and this line in a `.cmd` file to reopen it with a
   double-click.

6. Install the APK, give it access to all files and open it:

   ```bash
   adb install -r manga-downloader-v25.2.0-series-folders.<n>-android.apk
   adb shell appops set com.gmaples.mangadownloader MANAGE_EXTERNAL_STORAGE allow
   adb shell am start -n com.gmaples.mangadownloader/.MainActivity
   adb logcat -d -s MangaDownloader:*   # shows READY <port> once the backend serves
   ```

### When adb won't start

On one PC, every program in platform-tools (`adb`, `fastboot`, even
`hprof-conv`) froze as soon as it started, with only `ntdll.dll` loaded, and
couldn't be ended (`taskkill` said there was no running instance) until a
reboot. Releases 35.0.2, 36.0.0, 37.0.1 and 37.0.3 all did the same, while the
emulator, Java and other programs ran. Defender logged no detection. The
emulator still works then; it shows a "Could not automatically detect an ADB
binary" dialog that can be dismissed. Without adb:

- **Install through the tablet's browser.** Serve the folder holding the APK
  from the PC (`python -m http.server 8765 --bind 127.0.0.1`). In the tablet's
  Chrome, open `http://10.0.2.2:8765/<apk name>` (10.0.2.2 is the PC as the
  emulator sees it), open the download, and allow installs from Chrome when
  asked. Then turn on Settings > Apps > Special app access > All files access
  for Manga Downloader.
- **Control and screenshots** go through the emulator console on
  `127.0.0.1:5554`, which needs the token in
  `%USERPROFILE%\.emulator_console_auth_token` (`auth <token>` first).
  `screenrecord screenshot <folder>` saves a PNG of the screen,
  `event mouse <x> <y> 0 1` then `... 0 0` taps, and
  `event send EV_KEY:KEY_A:1 EV_KEY:KEY_A:0` presses a key (needs
  `hw.keyboard=yes`). `event text` didn't type into Chrome.
