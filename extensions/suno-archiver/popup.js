// Popup script — manages token display, auto-send toggle, and manual send.

const LOCAL_AUTH_URL = 'http://localhost:8080/api/auth';
const STORAGE_KEY_AUTO = 'suno-archiver:auto-send';

// DOM refs
const statusBar = document.getElementById('status-bar');
const statusText = document.getElementById('status-text');
const tokenSection = document.getElementById('token-section');
const tokenDisplay = document.getElementById('token-display');
const sendBtn = document.getElementById('send-btn');
const autoCheckbox = document.getElementById('auto-send');
const appStatusEl = document.getElementById('app-status');
const tokenSourceEl = document.getElementById('token-source');

let currentToken = null;
let tokenKey = null;

// —— Helpers ——

function setStatus(className, text) {
  statusBar.className = 'status ' + className;
  statusText.textContent = text;
}

function showToken(key, token) {
  currentToken = token;
  tokenKey = key;
  tokenSection.style.display = 'block';
  tokenDisplay.textContent = token.substring(0, 32) + '...' + token.slice(-8);
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
    const resp = await fetch(LOCAL_AUTH_URL, { method: 'OPTIONS' });
    appStatusEl.textContent = resp.ok || resp.status === 405
      ? 'Приложение: ✓'
      : `Приложение: ${resp.status}`;
  } catch {
    appStatusEl.textContent = 'Приложение: ✗ (не запущено)';
  }
}

async function sendToken(token) {
  sendBtn.textContent = '⏳ Отправка...';
  sendBtn.disabled = true;
  try {
    const resp = await fetch(LOCAL_AUTH_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    });
    if (resp.ok) {
      const data = await resp.json().catch(() => ({}));
      setStatus('connected', data.message || '✓ Токен отправлен!');
      return true;
    } else {
      const text = await resp.text();
      setStatus('disconnected', '✗ Ошибка: ' + resp.status + ' ' + text);
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

// Find active Suno tab and get token
chrome.tabs.query({ url: 'https://suno.ai/*' }, (tabs) => {
  if (!tabs || tabs.length === 0) {
    setStatus('unknown', 'Откройте suno.ai в браузере');
    hideToken();
    checkLocalApp();
    return;
  }

  const tab = tabs[0]; // use first Suno tab

  // Ask content script for token
  chrome.tabs.sendMessage(tab.id, { action: 'getToken' }, (response) => {
    if (chrome.runtime.lastError) {
      setStatus('disconnected', 'Расширение не сработало — перезагрузите suno.ai');
      hideToken();
      checkLocalApp();
      return;
    }

    if (response && response.token) {
      setStatus('connected', '✓ Токен найден');
      showToken(response.key || 'clerk-session', response.token);
    } else {
      setStatus('unknown', 'Токен не найден — войдите в Suno');
      hideToken();
    }
    checkLocalApp();
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
  chrome.storage.sync.set({ [STORAGE_KEY_AUTO]: enabled }, () => {
    console.log('[Suno Archiver] Auto-send:', enabled);
  });
});
