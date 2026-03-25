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
import android.content.res.AssetManager;
import android.net.Uri;
import android.os.Bundle;
import android.util.Log;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.MimeTypeMap;
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

public class WebViewActivity extends Activity {

    private static final String TAG = "goapk";
    private static final String CONFIG_PATH = "config.json";
    // Virtual origin used for local assets — absolute paths and CSP 'self' work correctly.
    private static final String ASSET_HOST = "https://appassets.androidplatform.net";
    private static final String ASSETS_INDEX = ASSET_HOST + "/index.html";
    // APK asset prefix that maps to the virtual origin root.
    private static final String ASSET_PREFIX = "www";

    private WebView webView;

    @Override
    @SuppressLint({"SetJavaScriptEnabled", "JavascriptInterface"})
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Fullscreen: hide status bar and navigation bar
        getWindow().setFlags(
            WindowManager.LayoutParams.FLAG_FULLSCREEN,
            WindowManager.LayoutParams.FLAG_FULLSCREEN
        );
        getWindow().getDecorView().setSystemUiVisibility(
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
            | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
            | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
            | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
            | View.SYSTEM_UI_FLAG_FULLSCREEN
            | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
        );

        webView = new WebView(this);
        setContentView(webView);

        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setMediaPlaybackRequiresUserGesture(false);

        // Register the native bridge for extensions
        webView.addJavascriptInterface(new NativeBridge(), "NativeBridge");

        webView.setWebChromeClient(new WebChromeClient());
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view,
                                                              WebResourceRequest request) {
                return maybeServeAsset(request.getUrl());
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

    // Intercepts requests to ASSET_HOST and serves them from the APK's assets/www/ directory.
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
        try {
            AssetManager am = getAssets();
            InputStream is = am.open(assetPath);
            String mimeType = guessMimeType(path);
            return new WebResourceResponse(mimeType, "utf-8", is);
        } catch (IOException e) {
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

    // NativeBridge is the JavaScript interface registered as window.NativeBridge.
    // Extensions add their own methods via separate DEX + JS shim injection.
    public class NativeBridge {
        @JavascriptInterface
        public String getPlatform() {
            return "android";
        }
    }
}
