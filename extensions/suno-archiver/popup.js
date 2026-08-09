// Popup script — manages token display, auto-send toggle, and manual send.

const BASE_URL = 'http://localhost:8080';
const AUTH_URL = 'http://localhost:8080/api/auth';
const HEALTH_URL = BASE_URL + '/api/health';
const STORAGE_KEY_AUTO = 'suno-archiver:auto-send';

// DOM refs
const statusBar = document.getElementById('status-bar');
const statusText = document.getElementById('status-text');
const statusDot = document.getElementById('status-dot');
const tokenSection = document.getElementById('token-section');
const tokenDisplay = document.getElementById('token-display');
const sendBtn = document.getElementById('send-btn');
const autoCheckbox = document.getElementById('auto-send');
const appStatusEl = document.getElementById('app-status');
const tokenSourceEl = document.getElementById('token-source');
const guidanceEl = document.getElementById('guidance');

let currentToken = null;
let currentSessionCookie = null;
let tokenKey = null;
let tokenSent = false;

// —— Helpers ——

function setStatus(className, text) {
  statusBar.className = 'status ' + className;
  statusText.textContent = text;
  if (statusDot) {
    statusDot.className = 'dot ' + (
      className === 'connected' ? 'green' :
      className === 'disconnected' ? 'red' :
      className === 'warning' ? 'yellow' : 'gray'
    );
  }
}

function showToken(key, token, isSession) {
  currentToken = isSession ? '' : token;
  currentSessionCookie = isSession ? token : '';
  tokenKey = key;
  tokenSection.style.display = 'block';
  const displayToken = token.substring(0, 24) + '...' + token.slice(-8);
  tokenDisplay.textContent = displayToken + (isSession ? ' (session cookie)' : ' (JWT)');
  sendBtn.disabled = false;
  tokenSourceEl.textContent = 'Источник: ' + key + (isSession ? ' [session]' : ' [JWT]');
}

function hideToken() {
  currentToken = null;
  currentSessionCookie = null;
  tokenKey = null;
  tokenSection.style.display = 'none';
  sendBtn.disabled = true;
  tokenSourceEl.textContent = 'Источник: —';
}

async function checkLocalApp() {
  try {
    const resp = await fetch(HEALTH_URL, { signal: AbortSignal.timeout(2000) });
    if (resp.ok) {
      appStatusEl.textContent = 'Приложение: ✓ запущено';
      guidanceEl.style.display = 'none';
      return true;
    } else {
      appStatusEl.textContent = 'Приложение: ошибка ' + resp.status;
      return false;
    }
  } catch {
    appStatusEl.textContent = 'Приложение: ✗ не запущено';
    guidanceEl.style.display = 'block';
    return false;
  }
}

// Read the __session cookie via chrome.cookies API (can read HttpOnly cookies,
// which content scripts cannot access through document.cookie).
function getSessionCookie() {
  return new Promise((resolve) => {
    const urls = ['https://suno.com', 'https://auth.suno.com'];
    const names = ['__session', '__clerk_client_jwt'];
    let found = null;

    let pending = urls.length * names.length;
    if (pending === 0) { resolve(null); return; }

    const checkDone = () => {
      pending--;
      if (pending <= 0) resolve(found);
    };

    for (const url of urls) {
      for (const name of names) {
        chrome.cookies.get({ url, name }, (cookie) => {
          if (chrome.runtime.lastError) { checkDone(); return; }
          if (cookie && cookie.value && !found) {
            found = { name, value: cookie.value, domain: cookie.domain };
          }
          checkDone();
        });
      }
    }
  });
}

async function sendAuth(token, sessionCookie) {
  sendBtn.textContent = '⏳ Отправка...';
  sendBtn.disabled = true;
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
      const data = await resp.json().catch(() => ({}));
      setStatus('connected', data.message || '✓ Auth отправлен!');
      tokenSent = true;
      return true;
    } else {
      const text = await resp.text();
      setStatus('disconnected', '✗ Ошибка: сервер вернул ' + resp.status);
      return false;
    }
  } catch (err) {
    setStatus('disconnected', '✗ Приложение не отвечает');
    return false;
  } finally {
    sendBtn.textContent = '📤 Отправить токен';
    sendBtn.disabled = currentToken === null && currentSessionCookie === null;
  }
}

// —— Initialization ——

// Load auto-send preference
chrome.storage.sync.get([STORAGE_KEY_AUTO], (data) => {
  const autoSend = data[STORAGE_KEY_AUTO];
  autoCheckbox.checked = autoSend !== false; // default true
});

// Step 1: check if the app is running
checkLocalApp().then((appRunning) => {
  if (!appRunning) {
    setStatus('warning', 'Приложение не запущено');
    hideToken();
    return;
  }

  // Step 2: app is running — look for Suno tab
  guidanceEl.style.display = 'none';

  chrome.tabs.query({ url: 'https://suno.com/*' }, async (tabs) => {
    if (!tabs || tabs.length === 0) {
      setStatus('unknown', 'Откройте suno.com в браузере и войдите');
      hideToken();
      return;
    }

    const tab = tabs[0];

    // Helper: try to inject content script if not already present
    async function ensureContentScript(tabId) {
      try {
        await chrome.scripting.executeScript({
          target: { tabId },
          files: ['content.js'],
        });
        await new Promise(r => setTimeout(r, 300));
      } catch (e) {
        // Already injected — ignore
      }
    }

    function queryToken(tabId) {
      return new Promise((resolve) => {
        chrome.tabs.sendMessage(tabId, { action: 'getToken' }, (response) => {
          resolve(chrome.runtime.lastError ? null : response);
        });
      });
    }

    // Try to get token — inject content script if needed, retry once
    let response = await queryToken(tab.id);
    if (!response) {
      await ensureContentScript(tab.id);
      response = await queryToken(tab.id);
    }

    // Read __session cookie via chrome.cookies API (handles HttpOnly cookies)
    const sessionCookie = await getSessionCookie();

    const contentToken = (response && response.token) || '';
    const contentSession = (response && response.sessionCookie) || '';

    if (!contentToken && !contentSession && !sessionCookie) {
      setStatus('unknown', 'Токен не найден — войдите в Suno');
      hideToken();
      // Show debug section when token not found
      document.getElementById('debug-section').style.display = 'block';
      return;
    }

    // Prefer the real __session cookie from chrome.cookies (most reliable),
    // then fall back to content-script values.
    const finalSession = sessionCookie ? sessionCookie.value : (contentSession || '');
    const finalToken = contentToken || '';

    setStatus('connected', '✓ Auth найден');
    const key = sessionCookie
      ? `cookie:${sessionCookie.name} (${sessionCookie.domain})`
      : (response && response.key) || 'clerk-session';
    const displayVal = finalSession || finalToken;
    const isSession = !!finalSession;
    showToken(key, displayVal, isSession);

    // Auto-send if enabled
    if (autoCheckbox.checked && !tokenSent) {
      console.log('[Suno Archiver] Auto-sending auth from popup...');
      sendAuth(finalToken, finalSession);
    }
  });
});

// —— Event handlers ——

sendBtn.addEventListener('click', () => {
  if (currentToken || currentSessionCookie) {
    sendAuth(currentToken, currentSessionCookie);
  }
});

autoCheckbox.addEventListener('change', () => {
  const enabled = autoCheckbox.checked;
  chrome.storage.sync.set({ [STORAGE_KEY_AUTO]: enabled }, () => {});
});

// —— Debug ——

document.getElementById('debug-toggle').addEventListener('click', async () => {
  const content = document.getElementById('debug-content');
  const arrow = document.getElementById('debug-arrow');
  const isHidden = content.style.display === 'none';

  if (isHidden) {
    content.style.display = 'block';
    arrow.textContent = '▼';
    content.textContent = 'Загрузка...';

    // Find suno.com tab and ask for debug info
    const tabs = await chrome.tabs.query({ url: 'https://suno.com/*' });
    if (!tabs || tabs.length === 0) {
      content.textContent = 'Нет открытых вкладок suno.com';
      return;
    }

    try {
      const resp = await chrome.tabs.sendMessage(tabs[0].id, { action: 'debug' });
      if (chrome.runtime.lastError || !resp) {
        content.textContent = 'Content script не отвечает — обновите страницу';
        return;
      }

      let out = '';
      out += `localStorage keys (${resp.storage.localStorageCount}):\n`;
      if (resp.storage.localStorageKeys.length === 0) {
        out += '  (пусто)\n';
      } else {
        for (const k of resp.storage.localStorageKeys) {
          const hasJWT = resp.storage.hasJWTs.includes(k) ? ' ← JWT!' : '';
          out += `  ${k}${hasJWT}\n`;
        }
      }
      out += `\ncookies (${resp.storage.cookieNames.length}):\n`;
      for (const c of resp.storage.cookieNames) {
        out += `  ${c}\n`;
      }
      if (resp.storage.cookieNames.length === 0) out += '  (пусто)\n';
      out += `\ntoken found: ${resp.found ? resp.found.key : '✗'}\n`;

      // Show __session cookie from chrome.cookies API
      const cookie = await getSessionCookie();
      out += `\nchrome.cookies __session: ${cookie ? `✓ ${cookie.name}@${cookie.domain}` : '✗ не найден'}\n`;

      // Show detected API endpoints
      if (resp.apiEndpoints && resp.apiEndpoints.length > 0) {
        out += `\ndetected API calls (${resp.apiEndpoints.length}):\n`;
        for (const ep of resp.apiEndpoints) {
          out += `  ${ep}\n`;
        }
      }

      content.textContent = out;
    } catch (e) {
      content.textContent = 'Ошибка: ' + e.message;
    }
  } else {
    content.style.display = 'none';
    arrow.textContent = '▶';
  }
});
