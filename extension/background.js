const OFFSCREEN_DOC = "offscreen.html";

async function ensureOffscreen() {
  const contexts = await chrome.runtime.getContexts({
    contextTypes: ["OFFSCREEN_DOCUMENT"],
    documentUrls: [chrome.runtime.getURL(OFFSCREEN_DOC)],
  });
  if (contexts.length > 0) return;

  await chrome.offscreen.createDocument({
    url: OFFSCREEN_DOC,
    reasons: ["WEB_RTC"],
    justification: "WebSocket connection to local cookie-bridge server",
  });
}

// offscreen 页面通过 runtime.sendMessage 转发请求到 background
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
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
    return true; // async sendResponse
  }

  if (msg.action === "listDomains") {
    chrome.cookies.getAll({}).then((cookies) => {
      const domains = [...new Set(cookies.map((c) => c.domain.replace(/^\./, "")))];
      domains.sort();
      sendResponse({ id: msg.id, ok: true, data: domains });
    });
    return true;
  }

  if (msg.action === "ping") {
    sendResponse({ id: msg.id, ok: true, data: "pong" });
    return false;
  }
});

ensureOffscreen();
