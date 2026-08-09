// Content script — runs on suno.com
// Reads Clerk JWT from localStorage and sends to the extension popup or auto-sends.
(function() {
'use strict';

const HEALTH_URL = 'http://localhost:8080/api/health';
const AUTH_URL='http://localhost:8080/api/auth';
const STORAGE_KEY_AUTO = 'suno-archiver:auto-send';
const MAX_RETRY_SEC = 60;

// Extensive list of Clerk-related localStorage keys that might hold a JWT.
const CLERK_KEYS = [
  '__session',
  '__clerk_client_jwt',
  'clerk-jwt',
  '__clerk_js_version',
  '__clerk_session',
  'clerk_session',
  'session',
  'suno_session',
  'supabase-auth-token',
  '__oauth_session',
];

// Try to extract a Clerk JWT from localStorage.
function findClerkJWT() {
  // 1. Check known Clerk keys
  for (const key of CLERK_KEYS) {
    try {
      const val = localStorage.getItem(key);
      if (val && typeof val === 'string') {
        // For __session, accept any non-empty value (not just JWTs)
        if (key === '__session' && val.length > 0) {
          return { key, token: val, isSession: true };
        }
        // For other keys, only accept JWTs (starting with 'ey')
        if (val.startsWith('ey')) {
          return { key, token: val, isSession: false };
        }
      }
    } catch {
      continue;
    }
  }

  // 2. Scan ALL localStorage keys for anything starting with 'ey' (JWT prefix)
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (!key) continue;
      const val = localStorage.getItem(key);
      if (val && typeof val === 'string') {
        // Try the value directly
        if (val.startsWith('ey')) {
          return { key, token: val, isSession: false };
        }
        // Try parsing as JSON (Clerk sometimes wraps in an object)
        try {
          const parsed = JSON.parse(val);
          if (typeof parsed === 'string' && parsed.startsWith('ey')) {
            return { key, token: parsed, isSession: false };
          }
          if (parsed && typeof parsed === 'object') {
            // Check common JWT fields
            for (const field of ['jwt', 'token', 'accessToken', 'access_token', 'session', 'idToken']) {
              if (typeof parsed[field] === 'string' && parsed[field].startsWith('ey')) {
                return { key: `${key}.${field}`, token: parsed[field], isSession: false };
              }
            }
          }
        } catch {
          // Not JSON, skip
        }
      }
    }
  } catch {
    // ignore
  }

  // 3. Check cookies for Clerk tokens (alternative storage)
  try {
    const cookies = document.cookie.split(';');
    for (const cookie of cookies) {
      const parts = cookie.trim().split('=');
      if (parts.length < 2) continue;
      const name = parts[0].trim();
      const val = decodeURIComponent(parts.slice(1).join('='));
      if (name === '__session' && val.length > 0) {
        return { key: `cookie:${name}`, token: val, isSession: true };
      }
      if ((name.includes('__session') || name.includes('clerk') || name.includes('token')) && val.startsWith('ey')) {
        return { key: `cookie:${name}`, token: val, isSession: false };
      }
    }
  } catch {
    // ignore
  }

  return null;
}

// Debug: dump all localStorage keys and cookie names (no values) for diagnostics.
function debugStorage() {
  const info = {
    localStorageKeys: [],
    localStorageCount: 0,
    cookieNames: [],
    hasJWTs: [],
  };
  try {
    info.localStorageCount = localStorage.length;
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (!key) continue;
      const val = localStorage.getItem(key);
      info.localStorageKeys.push(key);
      if (val && typeof val === 'string' && val.startsWith('ey')) {
        info.hasJWTs.push(key);
      }
    }
  } catch {}
  try {
    document.cookie.split(';').forEach(c => {
      const name = c.trim().split('=')[0];
      if (name) info.cookieNames.push(name);
    });
  } catch {}
  return info;
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

// Send token and/or session cookie to the local app.
async function sendAuth(token, sessionCookie) {
  try {
    const body = {};
    if (token) body.token = token;
    if (sessionCookie) body.session_cookie = sessionCookie;
    
    const resp = await fetch(AUTH_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(5000),
    });
    if (resp.ok) {
      console.log('[Suno Archiver] ✓ Auth sent successfully');
      return true;
    } else {
      console.warn('[Suno Archiver] Auth send failed:', resp.status);
      return false;
    }
  } catch (err) {
    console.log('[Suno Archiver] Local app not reachable:', err.message);
    return false;
  }
}

// Try to send auth, retrying if the app is offline.
async function tryAutoSend() {
  const result = findClerkJWT();
  if (!result) {
    console.log('[Suno Archiver] No Clerk auth found on page');
    return;
  }
  console.log('[Suno Archiver] Found Clerk auth (key:', result.key, ', isSession:', result.isSession, ')');

  const data = await chrome.storage.sync.get([STORAGE_KEY_AUTO]);
  const autoSend = data[STORAGE_KEY_AUTO];
  if (autoSend === false) {
    console.log('[Suno Archiver] Auto-send disabled');
    return;
  }

  const token = result.isSession ? '' : result.token;
  const sessionCookie = result.isSession ? result.token : '';

  // Try immediately
  const ok = await sendAuth(token, sessionCookie);
  if (ok) return;

  // Retry if app offline
  console.log('[Suno Archiver] App offline, retrying for ' + MAX_RETRY_SEC + 's...');
  const startTime = Date.now();
  const retryInterval = setInterval(async () => {
    if ((Date.now() - startTime) / 1000 > MAX_RETRY_SEC) {
      clearInterval(retryInterval);
      return;
    }
    if (!(await isAppRunning())) return;
    if (await sendAuth(token, sessionCookie)) {
      clearInterval(retryInterval);
    }
  }, 5000);
}

// Run on page load
tryAutoSend();

// Patch fetch to detect Suno API calls
(function patchFetch() {
  const origFetch = window.fetch;
  window.fetch = function(...args) {
    const url = typeof args[0] === 'string' ? args[0] : args[0]?.url;
    if (url && (
      url.includes('suno.com') || url.includes('suno.ai') || url.includes('api')
    ) && (
      url.includes('/api/') || url.match(/\/v\d+\//)
    )) {
      console.log('[Suno Archiver] 🎯 API call detected:', url);
      // Save to sessionStorage so popup can read it
      try {
        sessionStorage.setItem('suno-archiver:api-endpoints',
          JSON.stringify([...(JSON.parse(sessionStorage.getItem('suno-archiver:api-endpoints') || '[]')), url].slice(-10)));
      } catch {}
    }
    return origFetch.apply(this, args);
  };

  // Also patch XMLHttpRequest
  const origOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    if (url && typeof url === 'string' && (
      url.includes('suno.com') || url.includes('suno.ai')
    ) && (
      url.includes('/api/') || url.match(/\/v\d+\//)
    )) {
      console.log('[Suno Archiver] 🎯 XHR detected:', method, url);
      try {
        const existing = JSON.parse(sessionStorage.getItem('suno-archiver:api-endpoints') || '[]');
        existing.push(url);
        sessionStorage.setItem('suno-archiver:api-endpoints', JSON.stringify(existing.slice(-10)));
      } catch {}
    }
    return origOpen.apply(this, arguments);
  };
})();

// Listen for messages from popup
chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === 'getToken') {
    const result = findClerkJWT();
    if (result) {
      sendResponse({
        key: result.key,
        token: result.isSession ? '' : result.token,
        sessionCookie: result.isSession ? result.token : '',
      });
    } else {
      sendResponse({ key: null, token: null, sessionCookie: null });
    }
    return true;
  }

  if (request.action === 'sendToken') {
    const result = findClerkJWT();
    if (result) {
      const token = result.isSession ? '' : result.token;
      const sessionCookie = result.isSession ? result.token : '';
      sendAuth(token, sessionCookie).then((ok) => sendResponse({ ok }));
    } else {
      sendResponse({ ok: false, error: 'No Clerk auth found' });
    }
    return true;
  }

  if (request.action === 'debug') {
    const result = findClerkJWT();
    sendResponse({
      found: result ? { key: result.key, isSession: result.isSession } : null,
      storage: debugStorage(),
      apiEndpoints: getDetectedEndpoints(),
    });
    return true;
  }

  if (request.action === 'getEndpoints') {
    sendResponse({ endpoints: getDetectedEndpoints() });
    return true;
  }
});

function getDetectedEndpoints() {
  try {
    return JSON.parse(sessionStorage.getItem('suno-archiver:api-endpoints') || '[]');
  } catch {
    return [];
  }
}

})();