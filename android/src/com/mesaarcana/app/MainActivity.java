package com.mesaarcana.app;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import java.io.InputStream;

// O app é a versão de celular do Mesa Arcana (a página em assets/), servida por um endereço https falso
// para o navegador interno guardar os decks (IndexedDB) como num site normal.
public class MainActivity extends Activity {
    static final String HOST = "appassets.androidplatform.net";
    WebView web;
    ValueCallback<Uri[]> pick;

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        getWindow().setStatusBarColor(Color.parseColor("#12181c"));
        getWindow().setNavigationBarColor(Color.parseColor("#12181c"));
        web = new WebView(this);
        web.setBackgroundColor(Color.parseColor("#12181c"));
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setDatabaseEnabled(true);
        s.setAllowFileAccess(false);
        s.setCacheMode(WebSettings.LOAD_DEFAULT);
        s.setUserAgentString(s.getUserAgentString() + " MesaArcanaApp/1.0");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView v, WebResourceRequest r) {
                Uri u = r.getUrl();
                if (!HOST.equals(u.getHost())) return null;
                String p = u.getPath();
                if (p == null || p.equals("/") || p.isEmpty()) p = "/index.html";
                try {
                    InputStream in = getAssets().open("web" + p);
                    return new WebResourceResponse(mime(p), "utf-8", in);
                } catch (Exception e) {
                    return new WebResourceResponse("text/plain", "utf-8", 404, "Not Found", null, null);
                }
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest r) {
                Uri u = r.getUrl();
                if (HOST.equals(u.getHost())) return false;
                try { startActivity(new Intent(Intent.ACTION_VIEW, u)); } catch (Exception e) { }
                return true;
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView v, ValueCallback<Uri[]> cb, FileChooserParams p) {
                if (pick != null) pick.onReceiveValue(null);
                pick = cb;
                try { startActivityForResult(p.createIntent(), 7); } catch (Exception e) { pick = null; return false; }
                return true;
            }
        });
        setContentView(web);
        if (b != null) web.restoreState(b);
        else web.loadUrl("https://" + HOST + "/index.html");
    }

    static String mime(String p) {
        if (p.endsWith(".html")) return "text/html";
        if (p.endsWith(".js")) return "application/javascript";
        if (p.endsWith(".json")) return "application/json";
        if (p.endsWith(".png")) return "image/png";
        if (p.endsWith(".jpg")) return "image/jpeg";
        if (p.endsWith(".css")) return "text/css";
        return "application/octet-stream";
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req == 7 && pick != null) {
            pick.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(res, data));
            pick = null;
        }
    }

    @Override
    protected void onSaveInstanceState(Bundle o) {
        super.onSaveInstanceState(o);
        web.saveState(o);
    }

    @Override
    public void onBackPressed() {
        // o botão voltar fecha janelas e menus da página antes de sair do app
        web.evaluateJavascript("(function(){var m=document.getElementById('modal'),c=document.getElementById('ctx');if(c&&!c.hidden){c.hidden=true;return 1}if(m&&!m.hidden){closeModal();return 1}return 0})()",
            new ValueCallback<String>() {
                @Override
                public void onReceiveValue(String v) { if (!"1".equals(v)) finish(); }
            });
    }
}
