package com.gmaples.mangadownloader;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.provider.DocumentsContract;
import android.provider.Settings;
import android.view.Gravity;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.lang.ref.WeakReference;

/**
 * Shows the desktop app's window, served by the backend, in a WebView. The
 * page's window.MDAndroid (bridge.js uses it) opens links and folders, shows
 * the folder picker and reads the clipboard.
 */
public class MainActivity extends Activity implements Backend.Listener {
    private static final int PICK_FOLDER = 1;
    private static final int STORAGE = 2;
    private static WeakReference<MainActivity> shown = new WeakReference<>(null);

    private WebView web;
    private TextView status;
    private String origin;
    private int pickId;

    static void finishIfShown() {
        MainActivity a = shown.get();
        if (a != null) {
            a.finishAndRemoveTask();
        }
    }

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        shown = new WeakReference<>(this);
        status = new TextView(this);
        status.setGravity(Gravity.CENTER);
        status.setText(R.string.starting);
        setContentView(status);

        Intent svc = new Intent(this, BackendService.class);
        if (Build.VERSION.SDK_INT >= 26) {
            startForegroundService(svc);
        } else {
            startService(svc);
        }
        if (Build.VERSION.SDK_INT >= 33
                && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 0);
        }
        askForStorage();
        Backend.whenReady(this);
    }

    @Override
    protected void onDestroy() {
        Backend.forget(this);
        if (shown.get() == this) {
            shown = new WeakReference<>(null);
        }
        if (web != null) {
            web.destroy();
        }
        super.onDestroy();
    }

    @Override
    public void onReady(int port, String token) {
        origin = "http://127.0.0.1:" + port;
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        web.addJavascriptInterface(new Native(), "MDAndroid");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                Uri u = req.getUrl();
                if (isOwn(u)) {
                    return false;
                }
                // links never open inside the window, as on the desktop
                openExternal(u.toString());
                return true;
            }
        });
        web.loadUrl(origin + "/?t=" + token);
        setContentView(web);
    }

    @Override
    public void onFailed(String reason) {
        status.setText(getString(R.string.failed, reason));
    }

    @Override
    public void onBackPressed() {
        // the window has no history to go back through; keep it running
        moveTaskToBack(true);
    }

    private boolean isOwn(Uri u) {
        return origin != null && u.toString().startsWith(origin + "/");
    }

    private void openExternal(String url) {
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        } catch (ActivityNotFoundException e) {
            Toast.makeText(this, url, Toast.LENGTH_LONG).show();
        }
    }

    /** Asks for access to all files once, which saving into Download/Manga needs */
    private void askForStorage() {
        if (Build.VERSION.SDK_INT >= 30) {
            if (Environment.isExternalStorageManager()) {
                return;
            }
            new AlertDialog.Builder(this)
                    .setTitle(R.string.storage_title)
                    .setMessage(R.string.storage_text)
                    .setPositiveButton(R.string.storage_allow, (d, w) -> {
                        try {
                            startActivity(new Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION,
                                    Uri.parse("package:" + getPackageName())));
                        } catch (ActivityNotFoundException e) {
                            startActivity(new Intent(Settings.ACTION_MANAGE_ALL_FILES_ACCESS_PERMISSION));
                        }
                    })
                    .setNegativeButton(R.string.storage_later, null)
                    .show();
        } else if (Build.VERSION.SDK_INT >= 23
                && checkSelfPermission(Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.WRITE_EXTERNAL_STORAGE}, STORAGE);
        }
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request != PICK_FOLDER) {
            return;
        }
        String path = "";
        if (result == RESULT_OK && data != null && data.getData() != null) {
            path = treePath(data.getData());
        }
        answerPick(pickId, path);
    }

    private void answerPick(int id, String path) {
        if (web == null) {
            return;
        }
        web.evaluateJavascript("window.__mdFolderPicked(" + id + "," + JSONObject.quote(path) + ")", null);
    }

    /**
     * The file path of a folder the system picker returned, "" when it isn't
     * on a storage volume (a cloud provider's folder has no path)
     */
    static String treePath(Uri tree) {
        if (!"com.android.externalstorage.documents".equals(tree.getAuthority())) {
            return "";
        }
        String doc = DocumentsContract.getTreeDocumentId(tree);
        int colon = doc.indexOf(':');
        if (colon < 0) {
            return "";
        }
        String volume = doc.substring(0, colon);
        String rel = doc.substring(colon + 1);
        String root = "primary".equalsIgnoreCase(volume)
                ? Environment.getExternalStorageDirectory().getAbsolutePath()
                : "/storage/" + volume;
        return rel.isEmpty() ? root : root + "/" + rel;
    }

    /** The document id of path on the device's storage, for showing it in the Files app */
    private static String documentId(String path) {
        String primary = Environment.getExternalStorageDirectory().getAbsolutePath();
        if (path.equals(primary)) {
            return "primary:";
        }
        if (path.startsWith(primary + "/")) {
            return "primary:" + path.substring(primary.length() + 1);
        }
        if (path.startsWith("/storage/")) {
            String rest = path.substring("/storage/".length());
            int slash = rest.indexOf('/');
            return slash < 0 ? rest + ":" : rest.substring(0, slash) + ":" + rest.substring(slash + 1);
        }
        return null;
    }

    /** What the page calls as window.MDAndroid. These run on a binder thread. */
    private final class Native {
        @JavascriptInterface
        public void openURL(String url) {
            runOnUiThread(() -> openExternal(url));
        }

        @JavascriptInterface
        public void openFolder(String path) {
            runOnUiThread(() -> {
                String id = documentId(path);
                if (id != null) {
                    Uri uri = DocumentsContract.buildDocumentUri("com.android.externalstorage.documents", id);
                    Intent view = new Intent(Intent.ACTION_VIEW)
                            .setDataAndType(uri, DocumentsContract.Document.MIME_TYPE_DIR)
                            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                    try {
                        startActivity(view);
                        return;
                    } catch (ActivityNotFoundException ignored) {
                    }
                }
                Toast.makeText(MainActivity.this, getString(R.string.no_folder_app, path), Toast.LENGTH_LONG).show();
            });
        }

        @JavascriptInterface
        public void pickFolder(int id) {
            runOnUiThread(() -> {
                pickId = id;
                try {
                    startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE), PICK_FOLDER);
                } catch (ActivityNotFoundException e) {
                    answerPick(id, "");
                }
            });
        }

        @JavascriptInterface
        public String clipboardText() {
            ClipboardManager cm = getSystemService(ClipboardManager.class);
            ClipData clip = cm == null ? null : cm.getPrimaryClip();
            if (clip == null || clip.getItemCount() == 0) {
                return "";
            }
            CharSequence text = clip.getItemAt(0).coerceToText(MainActivity.this);
            return text == null ? "" : text.toString();
        }
    }
}
