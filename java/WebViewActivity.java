// WebViewActivity.java — pre-compiled to classes.dex and embedded in the goapk binary.
//
// Compile with: make dex  (requires Android SDK build-tools)
//
// This activity loads either:
//   - Local assets served via a virtual https://appassets.androidplatform.net/ origin
//     so that absolute paths (e.g. /assets/app.js) and CSP 'self' work correctly.
//   - A remote URL from assets/config.json {"start_url": "https://..."}
//
// JavaScript bridge: window.NativeBridge is registered for extensions to use.
package com.zapstore.goapk.runtime;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.pm.PackageManager;
import android.content.res.AssetManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.util.Log;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.webkit.GeolocationPermissions;
import android.webkit.JavascriptInterface;
import android.webkit.MimeTypeMap;
import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import org.json.JSONObject;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class WebViewActivity extends Activity {

    private static final String TAG = "goapk";
    private static final String CONFIG_PATH = "config.json";
    // Virtual origin used for local assets — absolute paths and CSP 'self' work correctly.
    private static final String ASSET_HOST = "https://appassets.androidplatform.net";
    private static final String ASSETS_INDEX = ASSET_HOST + "/";
    // APK asset prefix that maps to the virtual origin root.
    private static final String ASSET_PREFIX = "www";

    private static final int REQUEST_CODE_PERMISSIONS = 1001;

    // Maps WebKit resource strings to Android manifest permissions.
    private static final Map<String, String[]> WEBKIT_TO_ANDROID = new HashMap<>();
    static {
        WEBKIT_TO_ANDROID.put(PermissionRequest.RESOURCE_VIDEO_CAPTURE,
            new String[]{"android.permission.CAMERA"});
        WEBKIT_TO_ANDROID.put(PermissionRequest.RESOURCE_AUDIO_CAPTURE,
            new String[]{"android.permission.RECORD_AUDIO"});
    }

    private WebView webView;
    private PermissionRequest pendingPermissionRequest;
    private GeolocationPermissions.Callback pendingGeoCallback;
    private String pendingGeoOrigin;

    @Override
    @SuppressLint({"SetJavaScriptEnabled", "JavascriptInterface"})
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Allow the WebView to resize when the soft keyboard appears.
        // FLAG_FULLSCREEN must NOT be used — it silently disables SOFT_INPUT_ADJUST_RESIZE.
        // The SYSTEM_UI_FLAG_FULLSCREEN flag below provides the same visual effect.
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        applyImmersiveMode();
        getWindow().getDecorView().setOnSystemUiVisibilityChangeListener(
            visibility -> {
                if ((visibility & View.SYSTEM_UI_FLAG_FULLSCREEN) == 0) {
                    applyImmersiveMode();
                }
            }
        );

        webView = new WebView(this);
        setContentView(webView);

        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setMediaPlaybackRequiresUserGesture(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
        settings.setGeolocationEnabled(true);

        // Inform WebView of device connectivity so navigator.onLine works correctly.
        webView.setNetworkAvailable(true);

        // Register the native bridge for extensions
        webView.addJavascriptInterface(new NativeBridge(), "NativeBridge");

        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onPermissionRequest(final PermissionRequest request) {
                String[] resources = request.getResources();
                List<String> needed = new ArrayList<>();
                for (String res : resources) {
                    String[] androidPerms = WEBKIT_TO_ANDROID.get(res);
                    if (androidPerms != null) {
                        for (String perm : androidPerms) {
                            if (checkSelfPermission(perm) != PackageManager.PERMISSION_GRANTED) {
                                needed.add(perm);
                            }
                        }
                    }
                }

                if (needed.isEmpty()) {
                    request.grant(resources);
                } else {
                    pendingPermissionRequest = request;
                    requestPermissions(needed.toArray(new String[0]), REQUEST_CODE_PERMISSIONS);
                }
            }

            @Override
            public void onPermissionRequestCanceled(PermissionRequest request) {
                if (pendingPermissionRequest == request) {
                    pendingPermissionRequest = null;
                }
            }

            @Override
            public void onGeolocationPermissionsShowPrompt(String origin,
                    GeolocationPermissions.Callback callback) {
                String perm = "android.permission.ACCESS_FINE_LOCATION";
                if (checkSelfPermission(perm) == PackageManager.PERMISSION_GRANTED) {
                    callback.invoke(origin, true, false);
                } else {
                    pendingGeoCallback = callback;
                    pendingGeoOrigin = origin;
                    requestPermissions(new String[]{perm}, REQUEST_CODE_PERMISSIONS);
                }
            }
        });

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view,
                                                              WebResourceRequest request) {
                return maybeServeAsset(request.getUrl());
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                // setNetworkAvailable(true) alone may not update navigator.onLine
                // on all WebView versions. Inject JS to force the correct state and
                // dispatch the 'online' event so apps relying on event listeners
                // (e.g. React state) pick up the change.
                view.evaluateJavascript(
                    "if(!navigator.onLine){" +
                    "Object.defineProperty(navigator,'onLine'," +
                    "{get:function(){return true},configurable:true});" +
                    "window.dispatchEvent(new Event('online'));" +
                    "}",
                    null
                );
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request,
                                        WebResourceError error) {
                Log.e(TAG, "WebView error: " + error.getDescription()
                    + " url=" + request.getUrl());
            }
        });

        String startUrl = resolveStartUrl();
        Log.i(TAG, "Loading: " + startUrl);
        webView.loadUrl(startUrl);
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions,
                                           int[] grantResults) {
        if (requestCode != REQUEST_CODE_PERMISSIONS) {
            super.onRequestPermissionsResult(requestCode, permissions, grantResults);
            return;
        }

        boolean allGranted = true;
        for (int result : grantResults) {
            if (result != PackageManager.PERMISSION_GRANTED) {
                allGranted = false;
                break;
            }
        }

        if (pendingPermissionRequest != null) {
            if (allGranted) {
                pendingPermissionRequest.grant(pendingPermissionRequest.getResources());
            } else {
                pendingPermissionRequest.deny();
            }
            pendingPermissionRequest = null;
        }

        if (pendingGeoCallback != null) {
            pendingGeoCallback.invoke(pendingGeoOrigin, allGranted, false);
            pendingGeoCallback = null;
            pendingGeoOrigin = null;
        }
    }

    // Intercepts requests to ASSET_HOST and serves them from the APK's assets/www/ directory.
    // Includes SPA fallback: paths without a file extension that don't match a real asset
    // are served as index.html so client-side routers work correctly.
    private WebResourceResponse maybeServeAsset(Uri uri) {
        if (!ASSET_HOST.equals(uri.getScheme() + "://" + uri.getHost())) {
            return null;
        }
        String path = uri.getPath();
        if (path == null || path.isEmpty() || path.equals("/")) {
            path = "/index.html";
        }
        // Strip leading slash; resolve under assets/www/
        String assetPath = ASSET_PREFIX + path;
        AssetManager am = getAssets();
        try {
            InputStream is = am.open(assetPath);
            String mimeType = guessMimeType(path);
            return new WebResourceResponse(mimeType, "utf-8", is);
        } catch (IOException e) {
            // SPA fallback: if the path has no file extension it is a client-side route
            // (e.g. /settings, /chat). Serve index.html and let the JS router handle it.
            String leaf = path.substring(path.lastIndexOf('/') + 1);
            if (!leaf.contains(".")) {
                try {
                    InputStream is = am.open(ASSET_PREFIX + "/index.html");
                    return new WebResourceResponse("text/html", "utf-8", is);
                } catch (IOException e2) {
                    Log.e(TAG, "SPA fallback failed — index.html missing");
                }
            }
            Log.w(TAG, "Asset not found: " + assetPath);
            return null;
        }
    }

    private static String guessMimeType(String path) {
        int dot = path.lastIndexOf('.');
        if (dot >= 0) {
            String ext = path.substring(dot + 1).toLowerCase();
            switch (ext) {
                case "html": return "text/html";
                case "js":   return "application/javascript";
                case "mjs":  return "application/javascript";
                case "css":  return "text/css";
                case "json": return "application/json";
                case "png":  return "image/png";
                case "jpg":
                case "jpeg": return "image/jpeg";
                case "svg":  return "image/svg+xml";
                case "ico":  return "image/x-icon";
                case "woff": return "font/woff";
                case "woff2":return "font/woff2";
                case "webmanifest": return "application/manifest+json";
                case "wasm": return "application/wasm";
            }
            String fromMap = MimeTypeMap.getSingleton()
                .getMimeTypeFromExtension(ext);
            if (fromMap != null) return fromMap;
        }
        return "application/octet-stream";
    }

    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK && webView.canGoBack()) {
            webView.goBack();
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }

    @Override
    protected void onResume() {
        super.onResume();
        webView.onResume();
        applyImmersiveMode();
    }

    @Override
    protected void onPause() {
        webView.onPause();
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        webView.destroy();
        super.onDestroy();
    }

    // Reads assets/config.json and returns the start URL.
    // Falls back to loading local assets if no remote URL is configured.
    private String resolveStartUrl() {
        try {
            AssetManager am = getAssets();
            InputStream is = am.open(CONFIG_PATH);
            byte[] buf = new byte[is.available()];
            is.read(buf);
            is.close();
            String json = new String(buf, StandardCharsets.UTF_8);
            JSONObject cfg = new JSONObject(json);
            String url = cfg.optString("start_url", "");
            if (!url.isEmpty() && (url.startsWith("http://") || url.startsWith("https://"))) {
                return url;
            }
        } catch (Exception e) {
            Log.d(TAG, "No config.json or no start_url, loading local assets");
        }
        return ASSETS_INDEX;
    }

    private void applyImmersiveMode() {
        getWindow().getDecorView().setSystemUiVisibility(
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
            | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
            | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
            | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
            | View.SYSTEM_UI_FLAG_FULLSCREEN
            | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
        );
    }

    // NativeBridge is the JavaScript interface registered as window.NativeBridge.
    // Extensions add their own methods via separate DEX + JS shim injection.
    public class NativeBridge {
        @JavascriptInterface
        public String getPlatform() {
            return "android";
        }
    }
}
