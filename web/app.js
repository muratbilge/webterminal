/* webterminal frontend: xterm.js + websocket with auto-reconnect. */
(function () {
  "use strict";

  var FONT_MIN = 8, FONT_MAX = 28;
  var fontSize = parseInt(localStorage.getItem("wt-font") || "15", 10);

  var term = new Terminal({
    fontSize: fontSize,
    fontFamily: '"DejaVu Sans Mono", Menlo, Consolas, monospace',
    cursorBlink: true,
    scrollback: 5000,
    theme: {
      background: "#0d1117",
      foreground: "#c9d1d9",
      cursor: "#3fb950",
      selectionBackground: "#264f78"
    }
  });
  var fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(document.getElementById("terminal"));
  fit.fit();
  term.focus();

  var statusDot = document.getElementById("status-dot");
  var banner = document.getElementById("banner");
  var ws = null;
  var retryDelay = 500;
  var closedByServer = false;

  function setStatus(state, msg) {
    statusDot.className = "dot " + state;
    if (msg) {
      banner.textContent = msg;
      banner.hidden = false;
    } else {
      banner.hidden = true;
    }
  }

  function sendResize() {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send("1" + JSON.stringify({ cols: term.cols, rows: term.rows }));
    }
  }

  function connect() {
    setStatus("connecting", null);
    var proto = location.protocol === "https:" ? "wss://" : "ws://";
    // Keep a local reference: handlers must only act if this socket is still
    // the current one, or late events from an abandoned socket would inject
    // stale output, close the healthy replacement, or multiply reconnects.
    var sock = new WebSocket(proto + location.host + location.pathname.replace(/[^/]*$/, "") + "ws");
    ws = sock;
    sock.binaryType = "arraybuffer";

    sock.onopen = function () {
      if (sock !== ws) { sock.close(); return; }
      retryDelay = 500;
      closedByServer = false;
      setStatus("connected", null);
      fit.fit();
      sendResize();
      term.focus();
    };

    sock.onmessage = function (ev) {
      if (sock !== ws) return;
      term.write(new Uint8Array(ev.data));
    };

    sock.onclose = function (ev) {
      if (sock !== ws) return;
      if (ev.reason === "shell exited") {
        closedByServer = true;
        setStatus("", "Shell exited — press Enter to start a new session.");
        return;
      }
      setStatus("", "Disconnected — reconnecting…");
      setTimeout(connect, retryDelay);
      retryDelay = Math.min(retryDelay * 2, 10000);
    };

    sock.onerror = function () { sock.close(); };
  }

  var ctrlSticky = false, altSticky = false;
  var modCtrl = document.getElementById("mod-ctrl");
  var modAlt = document.getElementById("mod-alt");

  function sendInput(data) {
    if (closedByServer) {
      if (data.indexOf("\r") !== -1) {
        closedByServer = false; // clear now, not in onopen — blocks double-Enter duplicates
        term.reset();
        connect();
      }
      return;
    }
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send("0" + data);
    }
  }

  term.onData(function (data) {
    // Apply sticky modifiers from the key bar to the next typed character.
    if (ctrlSticky && data.length === 1) {
      var c = data.toUpperCase().charCodeAt(0);
      if (c >= 64 && c <= 95) data = String.fromCharCode(c & 31);
      ctrlSticky = false;
      modCtrl.classList.remove("active");
    }
    if (altSticky && data.length === 1) {
      data = "\x1b" + data;
      altSticky = false;
      modAlt.classList.remove("active");
    }
    sendInput(data);
  });

  /* ---- copy & paste (must also work on plain HTTP, where the browser
     blocks navigator.clipboard) ---- */

  function copyText(text) {
    if (!text) return;
    if (window.isSecureContext && navigator.clipboard) {
      navigator.clipboard.writeText(text);
      return;
    }
    // execCommand fallback works in insecure contexts
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); } catch (e) {}
    document.body.removeChild(ta);
    term.focus();
  }

  function promptPaste() {
    var t = window.prompt("Browser blocks clipboard read over HTTP.\nPaste your text here:");
    if (t) term.paste(t);
    term.focus();
  }

  function doPaste() {
    if (window.isSecureContext && navigator.clipboard && navigator.clipboard.readText) {
      navigator.clipboard.readText()
        .then(function (t) { if (t) term.paste(t); term.focus(); })
        .catch(promptPaste);
    } else {
      promptPaste();
    }
  }

  // Copy-on-select, like PuTTY — but only once the selection is finished
  // (on mouse/touch release). Copying during the drag would steal the
  // document selection via the HTTP fallback and break selecting entirely.
  function copySelection() {
    setTimeout(function () {
      if (term.hasSelection()) copyText(term.getSelection());
    }, 50);
  }
  var termEl = document.getElementById("terminal");
  termEl.addEventListener("mouseup", copySelection);
  termEl.addEventListener("touchend", copySelection);

  // Ctrl+Shift+C = copy. Ctrl+V / Ctrl+Shift+V reach the browser's native
  // paste, which xterm.js picks up as a paste event even on plain HTTP.
  term.attachCustomKeyEventHandler(function (ev) {
    if (ev.type === "keydown" && ev.ctrlKey && ev.shiftKey && ev.code === "KeyC") {
      copyText(term.getSelection());
      return false;
    }
    if (ev.ctrlKey && ev.code === "KeyV") return false; // let browser paste
    return true;
  });

  document.getElementById("copy").addEventListener("click", function () {
    copyText(term.getSelection());
  });
  document.getElementById("paste").addEventListener("click", doPaste);

  /* ---- selection mode: xterm.js has no touch selection, so show the
     buffer as plain text where native long-press selection works ---- */

  var overlay = document.getElementById("select-overlay");
  var overlayText = document.getElementById("overlay-text");

  function bufferText() {
    var buf = term.buffer.active;
    var lines = [];
    for (var i = 0; i < buf.length; i++) {
      var line = buf.getLine(i);
      if (line) lines.push(line.translateToString(true));
    }
    return lines.join("\n").replace(/\s+$/, "");
  }

  document.getElementById("select-mode").addEventListener("click", function () {
    overlayText.textContent = bufferText();
    overlay.hidden = false;
    overlayText.scrollTop = overlayText.scrollHeight;
  });
  document.getElementById("overlay-close").addEventListener("click", function () {
    overlay.hidden = true;
    term.focus();
  });
  document.getElementById("copy-all").addEventListener("click", function () {
    copyText(overlayText.textContent);
  });
  // Inside the overlay the browser's native selection takes over: long-press
  // shows the system Copy menu, so no extra handling is needed here.

  /* ---- shortcut key bar ---- */

  var KEYSEQ = {
    "esc": "\x1b", "tab": "\t",
    "up": "\x1b[A", "down": "\x1b[B", "left": "\x1b[D", "right": "\x1b[C",
    "home": "\x1b[H", "end": "\x1b[F",
    "ctrl-c": "\x03", "ctrl-d": "\x04"
  };
  document.querySelectorAll("#keys button[data-key]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      sendInput(KEYSEQ[btn.dataset.key]);
      term.focus();
    });
  });
  modCtrl.addEventListener("click", function () {
    ctrlSticky = !ctrlSticky;
    modCtrl.classList.toggle("active", ctrlSticky);
    term.focus();
  });
  modAlt.addEventListener("click", function () {
    altSticky = !altSticky;
    modAlt.classList.toggle("active", altSticky);
    term.focus();
  });

  term.onResize(sendResize);

  var resizeTimer;
  window.addEventListener("resize", function () {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(function () { fit.fit(); }, 100);
  });

  function setFont(delta) {
    fontSize = Math.max(FONT_MIN, Math.min(FONT_MAX, fontSize + delta));
    term.options.fontSize = fontSize;
    localStorage.setItem("wt-font", String(fontSize));
    fit.fit();
    term.focus();
  }
  document.getElementById("font-plus").addEventListener("click", function () { setFont(1); });
  document.getElementById("font-minus").addEventListener("click", function () { setFont(-1); });

  // Toggle the shortcut key bar; also re-focus the terminal, which raises
  // the on-screen keyboard on touch devices.
  document.getElementById("kbd").addEventListener("click", function () {
    document.getElementById("keys").classList.toggle("hidden");
    fit.fit();
    term.focus();
    var ta = document.querySelector(".xterm-helper-textarea");
    if (ta) ta.focus();
  });

  document.getElementById("hostname").textContent = location.hostname || "webterminal";

  connect();
})();
