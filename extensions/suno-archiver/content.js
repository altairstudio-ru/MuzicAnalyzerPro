// Content script — runs on suno.ai
// Reads Clerk JWT from localStorage and sends to the extension popup or auto-sends.

const LOCAL_AUTH_URL = 'http://localhost:8080/api/auth';
const STORAGE_KEY_AUTO = 'suno-archiver:auto-send';

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
      // localStorage access may be blocked in some contexts
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

// Send token to the local app.
async function sendToken(token) {
  try {
    const resp = await fetch(LOCAL_AUTH_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    });
    if (resp.ok) {
      console.log('[Suno Archiver] Token sent successfully');
      return true;
    } else {
      const text = await resp.text();
      console.warn('[Suno Archiver] Token send failed:', resp.status, text);
      return false;
    }
  } catch (err) {
    // Connection refused — app not running, that's okay
    console.log('[Suno Archiver] Local app not reachable:', err.message);
    return false;
  }
}

// Main logic: check if auto-send is enabled, and send if so.
async function tryAutoSend() {
  const result = findClerkJWT();
  if (!result) {
    console.log('[Suno Archiver] No Clerk JWT found on page');
    return;
  }

  chrome.storage.sync.get([STORAGE_KEY_AUTO], async (data) => {
    const autoSend = data[STORAGE_KEY_AUTO];
    if (autoSend === true || autoSend === undefined) {
      // Default: enabled
      await sendToken(result.token);
    }
  });
}

// Run on page load
tryAutoSend();

// Listen for manual trigger from popup
chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === 'getToken') {
    const result = findClerkJWT();
    sendResponse(result);
    return true; // keep channel open for async response
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
