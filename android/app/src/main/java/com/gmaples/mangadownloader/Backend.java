package com.gmaples.mangadownloader;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.ProxyInfo;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.text.TextUtils;
import android.util.Log;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * The Go backend (libmdgui.so, the desktop app's backend built for Android),
 * run as a program. It serves the window on the loopback, on a port it prints
 * as "READY <port>", and refuses calls without the token it's given.
 * BackendService owns it, so it outlives the activity.
 */
final class Backend {
    static final String TAG = "MangaDownloader";

    interface Listener {
        void onReady(int port, String token);

        void onFailed(String reason);
    }

    private static final Handler main = new Handler(Looper.getMainLooper());
    private static final List<Listener> waiting = new ArrayList<>();
    private static Process process;
    private static int port;
    private static String token;
    private static String failure;

    private Backend() {
    }

    /** Starts the backend unless it runs already */
    static synchronized void start(Context ctx) {
        if (process != null && process.isAlive()) {
            return;
        }
        port = 0;
        failure = null;
        token = newToken();
        File exe = new File(ctx.getApplicationInfo().nativeLibraryDir, "libmdgui.so");
        File output = new File(Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS), "Manga");
        ProcessBuilder pb = new ProcessBuilder(exe.getAbsolutePath(),
                "-addr", "127.0.0.1:0",
                "-token", token,
                "-output-dir", output.getAbsolutePath());
        Map<String, String> env = pb.environment();
        File files = ctx.getFilesDir();
        env.put("HOME", files.getAbsolutePath());
        env.put("XDG_CONFIG_HOME", new File(files, "config").getAbsolutePath());
        env.put("TMPDIR", ctx.getCacheDir().getAbsolutePath());
        useSystemProxy(ctx, env);
        pb.directory(files);
        pb.redirectErrorStream(true);
        final Process p;
        try {
            p = pb.start();
        } catch (IOException e) {
            fail(e.toString());
            return;
        }
        process = p;
        Thread reader = new Thread(() -> read(p), "backend-output");
        reader.setDaemon(true);
        reader.start();
    }

    /** Stops the backend, letting it cancel its downloads first */
    static synchronized void stop() {
        if (process == null) {
            return;
        }
        Process p = process;
        process = null;
        port = 0;
        // SIGTERM: the backend shuts its window down cleanly
        p.destroy();
        new Thread(() -> {
            try {
                Thread.sleep(3000);
            } catch (InterruptedException ignored) {
            }
            if (p.isAlive()) {
                p.destroyForcibly();
            }
        }, "backend-stop").start();
    }

    /** Calls l on the main thread once the backend serves, at once when it does already */
    static synchronized void whenReady(Listener l) {
        if (port > 0) {
            int pt = port;
            String tk = token;
            main.post(() -> l.onReady(pt, tk));
        } else if (failure != null) {
            String f = failure;
            main.post(() -> l.onFailed(f));
        } else {
            waiting.add(l);
        }
    }

    /** Forgets l if it's still waiting */
    static synchronized void forget(Listener l) {
        waiting.remove(l);
    }

    private static void read(Process p) {
        try (BufferedReader in = new BufferedReader(new InputStreamReader(p.getInputStream(), StandardCharsets.UTF_8))) {
            String line;
            while ((line = in.readLine()) != null) {
                if (line.startsWith("READY ")) {
                    try {
                        ready(p, Integer.parseInt(line.substring(6).trim()));
                    } catch (NumberFormatException e) {
                        fail("bad READY line: " + line);
                    }
                }
                Log.i(TAG, line);
            }
        } catch (IOException e) {
            Log.w(TAG, "backend output", e);
        }
        try {
            int code = p.waitFor();
            Log.i(TAG, "backend exited with " + code);
            synchronized (Backend.class) {
                if (process == p) {
                    process = null;
                    port = 0;
                    fail("it exited with code " + code);
                }
            }
        } catch (InterruptedException ignored) {
        }
    }

    private static synchronized void ready(Process p, int pt) {
        if (process != p) {
            return;
        }
        port = pt;
        String tk = token;
        for (Listener l : waiting) {
            main.post(() -> l.onReady(pt, tk));
        }
        waiting.clear();
    }

    private static synchronized void fail(String reason) {
        failure = reason;
        Log.e(TAG, "backend: " + reason);
        for (Listener l : waiting) {
            main.post(() -> l.onFailed(reason));
        }
        waiting.clear();
    }

    /**
     * Passes the proxy set in Android's settings to the backend, which (being
     * Go) only reads it from the environment. A PAC proxy isn't followed.
     */
    private static void useSystemProxy(Context ctx, Map<String, String> env) {
        if (android.os.Build.VERSION.SDK_INT < 23) {
            return;
        }
        ConnectivityManager cm = ctx.getSystemService(ConnectivityManager.class);
        ProxyInfo pi = cm == null ? null : cm.getDefaultProxy();
        if (pi == null || TextUtils.isEmpty(pi.getHost()) || pi.getPort() <= 0) {
            return;
        }
        String proxy = "http://" + pi.getHost() + ":" + pi.getPort();
        env.put("HTTPS_PROXY", proxy);
        env.put("HTTP_PROXY", proxy);
        String excluded = TextUtils.join(",", pi.getExclusionList());
        env.put("NO_PROXY", excluded.isEmpty() ? "127.0.0.1,localhost" : excluded + ",127.0.0.1,localhost");
        Log.i(TAG, "using the system proxy " + proxy);
    }

    private static String newToken() {
        byte[] raw = new byte[24];
        new SecureRandom().nextBytes(raw);
        StringBuilder sb = new StringBuilder();
        for (byte b : raw) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }
}
