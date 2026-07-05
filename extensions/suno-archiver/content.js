// Content script — runs on suno.com
// Reads Clerk JWT from localStorage and sends to the extension popup or auto-sends.

const HEALTH_URL = 'http://localhost:8080/api/health';
const AUTH_URL = 'http://localhost:8080/api/auth';
const STORAGE_KEY_AUTO = 'suno-archiver:auto-send';
const MAX_RETRY_SEC = 60; // retry sending for up to 60 seconds after initial failure

// Known Clerk localStorage keys that may contain the JWT session token.
const CLERK_KEYS = [
  '__session',
  '__clerk_client_jwt',
  'clerk-jwt',
  '__clerk_js_version',
];

// Try to extract a Clerk JWT from localStorage.
function findClerkJWT() {
  for (const key of CLERK_KEYS) {
    try {
      const val = localStorage.getItem(key);
      if (val && typeof val === 'string' && val.startsWith('ey')) {
        return { key, token: val };
      }
    } catch {
      continue;
    }
  }

  // Fallback: scan all localStorage keys for Clerk-like JWTs
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (!key) continue;
      if (key.startsWith('__clerk') || key.includes('clerk') || key.includes('session')) {
        const val = localStorage.getItem(key);
        if (val && typeof val === 'string' && val.startsWith('ey')) {
          return { key, token: val };
        }
      }
    }
  } catch {
    // ignore
  }

  return null;
}

// Check if the local app is running.
async function isAppRunning() {
  try {
    const resp = await fetch(HEALTH_URL, { signal: AbortSignal.timeout(3000) });
    return resp.ok;
  } catch {
    return false;
  }
}

// Send token to the local app.
async function sendToken(token) {
  try {
    const resp = await fetch(AUTH_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
      signal: AbortSignal.timeout(5000),
    });
    if (resp.ok) {
      console.log('[Suno Archiver] ✓ Token sent successfully');
      return true;
    } else {
      console.warn('[Suno Archiver] Token send failed:', resp.status);
      return false;
    }
  } catch (err) {
    console.log('[Suno Archiver] Local app not reachable:', err.message);
    return false;
  }
}

// Try to send token, retrying if the app is offline.
async function tryAutoSend() {
  const result = findClerkJWT();
  if (!result) {
    console.log('[Suno Archiver] No Clerk JWT found on page');
    return;
  }
  console.log('[Suno Archiver] Found Clerk JWT (key:', result.key, ')');

  // Get auto-send preference
  const data = await chrome.storage.sync.get([STORAGE_KEY_AUTO]);
  const autoSend = data[STORAGE_KEY_AUTO];
  if (autoSend === false) {
    console.log('[Suno Archiver] Auto-send disabled in settings');
    return;
  }

  // Try immediately
  const ok = await sendToken(result.token);
  if (ok) return;

  // First attempt failed (app likely offline) — retry in background
  console.log('[Suno Archiver] App offline, will retry for ' + MAX_RETRY_SEC + 's...');
  const startTime = Date.now();
  const retryInterval = setInterval(async () => {
    const elapsed = (Date.now() - startTime) / 1000;
    if (elapsed > MAX_RETRY_SEC) {
      clearInterval(retryInterval);
      console.log('[Suno Archiver] Retry timeout — token not sent');
      return;
    }

    const running = await isAppRunning();
    if (!running) return; // still offline, keep retrying

    const ok = await sendToken(result.token);
    if (ok) {
      clearInterval(retryInterval);
      console.log('[Suno Archiver] Token sent on retry (app came online)');
    }
  }, 5000); // check every 5 seconds
}

// Run on page load
tryAutoSend();

// Listen for manual trigger from popup
chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === 'getToken') {
    const result = findClerkJWT();
    sendResponse(result);
    return true;
  }

  if (request.action === 'sendToken') {
    const result = findClerkJWT();
    if (result) {
      sendToken(result.token).then((ok) => sendResponse({ ok }));
    } else {
      sendResponse({ ok: false, error: 'No Clerk JWT found' });
    }
    return true;
  }
});
