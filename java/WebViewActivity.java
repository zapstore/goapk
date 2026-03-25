// WebViewActivity.java — pre-compiled to classes.dex and embedded in the goapk binary.
//
// Compile with: make dex  (requires Android SDK build-tools)
//
// This activity loads either:
//   - Local assets from assets/www/index.html (when config.start_url is absent/relative)
//   - A remote URL from assets/config.json {"start_url": "https://..."}
//
// JavaScript bridge: window.NativeBridge is registered for extensions to use.
package com.zapstore.goapk.runtime;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.res.AssetManager;
import android.os.Bundle;
import android.util.Log;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import org.json.JSONObject;

import java.io.InputStream;
import java.nio.charset.StandardCharsets;

public class WebViewActivity extends Activity {

    private static final String TAG = "goapk";
    private static final String CONFIG_PATH = "config.json";
    private static final String ASSETS_INDEX = "file:///android_asset/www/index.html";

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
        settings.setAllowFileAccessFromFileURLs(true);
        settings.setAllowUniversalAccessFromFileURLs(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setMediaPlaybackRequiresUserGesture(false);

        // Register the native bridge for extensions
        webView.addJavascriptInterface(new NativeBridge(), "NativeBridge");

        webView.setWebChromeClient(new WebChromeClient());
        webView.setWebViewClient(new WebViewClient() {
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
