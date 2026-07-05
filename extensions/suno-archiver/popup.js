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

function showToken(key, token) {
  currentToken = token;
  tokenKey = key;
  tokenSection.style.display = 'block';
  tokenDisplay.textContent = token.substring(0, 24) + '...' + token.slice(-8);
  sendBtn.disabled = false;
  tokenSourceEl.textContent = 'Источник: ' + key;
}

function hideToken() {
  currentToken = null;
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

async function sendToken(token) {
  sendBtn.textContent = '⏳ Отправка...';
  sendBtn.disabled = true;
  try {
    const resp = await fetch(AUTH_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
      signal: AbortSignal.timeout(5000),
    });
    if (resp.ok) {
      const data = await resp.json().catch(() => ({}));
      setStatus('connected', data.message || '✓ Токен отправлен!');
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
    sendBtn.disabled = currentToken === null;
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

    if (!response || !response.token) {
      setStatus('unknown', 'Токен не найден — войдите в Suno');
      hideToken();
      // Show debug section when token not found
      document.getElementById('debug-section').style.display = 'block';
      return;
    }

    // Token found!
    setStatus('connected', '✓ Токен найден');
    showToken(response.key || 'clerk-session', response.token);

    // Auto-send if enabled
    if (autoCheckbox.checked && !tokenSent) {
      console.log('[Suno Archiver] Auto-sending token from popup...');
      sendToken(response.token);
    }
  });
});

// —— Event handlers ——

sendBtn.addEventListener('click', () => {
  if (currentToken) {
    sendToken(currentToken);
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
