const WS_URL = "ws://127.0.0.1:8008/ws";
const RECONNECT_MS = 3000;

let ws = null;

function connect() {
  if (ws && ws.readyState <= WebSocket.OPEN) return;

  ws = new WebSocket(WS_URL);

  ws.onopen = () => console.log("[cookie-bridge] connected");

  ws.onmessage = async (event) => {
    let msg;
    try {
      msg = JSON.parse(event.data);
    } catch {
      return;
    }

    // 转发到 background service worker 处理 chrome.cookies API
    chrome.runtime.sendMessage(msg, (resp) => {
      if (chrome.runtime.lastError) {
        ws.send(JSON.stringify({ id: msg.id, ok: false, error: chrome.runtime.lastError.message }));
        return;
      }
      ws.send(JSON.stringify(resp));
    });
  };

  ws.onclose = () => {
    ws = null;
    setTimeout(connect, RECONNECT_MS);
  };

  ws.onerror = () => ws?.close();
}

connect();
