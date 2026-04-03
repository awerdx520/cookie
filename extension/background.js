const OFFSCREEN_DOC = "offscreen.html";
const NATIVE_HOST_NAME = "com.cookie.bridge";
const NATIVE_RECONNECT_MS = 5000;

let nativePort = null;

// ── Cookie API handlers ──────────────────────────────────────

function handleCookieRequest(msg, sendResponse) {
  if (msg.action === "getCookies") {
    const details = {};
    if (msg.domain) details.domain = msg.domain;
    if (msg.url) details.url = msg.url;
    if (msg.name) details.name = msg.name;

    chrome.cookies.getAll(details).then((cookies) => {
      sendResponse({
        id: msg.id,
        ok: true,
        data: cookies.map((c) => ({
          name: c.name,
          value: c.value,
          domain: c.domain,
          path: c.path,
          secure: c.secure,
          httpOnly: c.httpOnly,
          expirationDate: c.expirationDate || 0,
          sameSite: c.sameSite,
        })),
      });
    });
    return true;
  }

  if (msg.action === "listDomains") {
    chrome.cookies.getAll({}).then((cookies) => {
      const domains = [
        ...new Set(cookies.map((c) => c.domain.replace(/^\./, ""))),
      ];
      domains.sort();
      sendResponse({ id: msg.id, ok: true, data: domains });
    });
    return true;
  }

  if (msg.action === "exportCookies") {
    const details = {};
    if (msg.domain) details.domain = msg.domain;
    chrome.cookies.getAll(details).then((cookies) => {
      sendResponse({
        id: msg.id,
        ok: true,
        data: cookies.map((c) => ({
          name: c.name,
          value: c.value,
          domain: c.domain,
          path: c.path,
          secure: c.secure,
          httpOnly: c.httpOnly,
          expirationDate: c.expirationDate || 0,
          sameSite: c.sameSite,
        })),
      });
    });
    return true;
  }

  if (msg.action === "ping") {
    sendResponse({ id: msg.id, ok: true, data: "pong" });
    return false;
  }

  return false;
}

// ── WebSocket mode (via offscreen document) ──────────────────

async function ensureOffscreen() {
  const contexts = await chrome.runtime.getContexts({
    contextTypes: ["OFFSCREEN_DOCUMENT"],
    documentUrls: [chrome.runtime.getURL(OFFSCREEN_DOC)],
  });
  if (contexts.length > 0) return;

  await chrome.offscreen
    .createDocument({
      url: OFFSCREEN_DOC,
      reasons: ["WEB_RTC"],
      justification: "WebSocket connection to local cookie-bridge server",
    })
    .catch(() => {});
}

// Messages from offscreen document (WebSocket relay) or internal
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  return handleCookieRequest(msg, sendResponse);
});

// ── Native Messaging mode ────────────────────────────────────

function connectNative() {
  if (nativePort) return;

  try {
    nativePort = chrome.runtime.connectNative(NATIVE_HOST_NAME);
  } catch (e) {
    console.log("[cookie-bridge] Native Messaging host 不可用:", e.message);
    scheduleNativeReconnect();
    return;
  }

  console.log("[cookie-bridge] Native Messaging 已连接");

  nativePort.onMessage.addListener((msg) => {
    // msg is a request from the native host (forwarded from CLI)
    handleCookieRequest(msg, (response) => {
      try {
        nativePort.postMessage(response);
      } catch (e) {
        console.log("[cookie-bridge] 发送 Native Messaging 响应失败:", e.message);
      }
    });
  });

  nativePort.onDisconnect.addListener(() => {
    const error = chrome.runtime.lastError;
    if (error) {
      console.log("[cookie-bridge] Native Messaging 断开:", error.message);
    }
    nativePort = null;
    scheduleNativeReconnect();
  });
}

function scheduleNativeReconnect() {
  setTimeout(connectNative, NATIVE_RECONNECT_MS);
}

// ── Startup ──────────────────────────────────────────────────

ensureOffscreen();
connectNative();
