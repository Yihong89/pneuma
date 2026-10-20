// pneuma web client. No framework, no build step.

const $ = (id) => document.getElementById(id);
const els = {
  app: $("app"),
  log: $("log"),
  messages: $("messages"),
  uploads: $("uploads"),
  empty: $("empty"),
  emptyText: $("empty-text"),
  loader: $("loader"),
  input: $("input"),
  composer: $("composer"),
  attach: $("attach"),
  fileInput: $("file-input"),
  drop: $("drop"),
  toast: $("toast"),
  status: $("status"),
  chatList: $("chat-list"),
  chatTitle: $("chat-title"),
  newChat: $("new-chat"),
  newChatForm: $("new-chat-form"),
  newChatInput: $("new-chat-input"),
  renameChat: $("rename-chat"),
  deleteChat: $("delete-chat"),
  deviceList: $("device-list"),
  menuBtn: $("menu-btn"),
  scrim: $("scrim"),
};

// ---------- i18n ----------

const STRINGS = {
  en: {
    placeholder: "message, paste or drop files",
    empty: "Type, paste or drop anything. It shows up on all your devices.",
    drop: "DROP TO SEND",
    copied: "copied",
    copyFailed: "couldn't copy",
    copy: "copy",
    copyImage: "copy image",
    download: "download",
    delete: "delete",
    confirmDelete: "sure?",
    rename: "rename",
    deleteChatConfirm: "delete chat + files?",
    today: "today",
    yesterday: "yesterday",
    online: "online",
    offline: "reconnecting…",
    tooLarge: (max) => `too large (max ${max})`,
    uploadFailed: "upload failed",
    sendFailed: "couldn't send",
    chats: "chats",
    devices: "devices",
    newChat: "new chat",
    newChatPlaceholder: "chat name ⏎",
    here: "here",
    open: "open",
    idle: "online",
    away: "offline",
    attach: "attach files",
    send: "send",
  },
  es: {
    placeholder: "escribe, pega o suelta archivos",
    empty: "Escribe, pega o arrastra lo que sea. Aparecerá en todos tus equipos.",
    drop: "SUELTA PARA ENVIAR",
    copied: "copiado",
    copyFailed: "no se pudo copiar",
    copy: "copiar",
    copyImage: "copiar imagen",
    download: "descargar",
    delete: "borrar",
    confirmDelete: "¿seguro?",
    rename: "renombrar",
    deleteChatConfirm: "¿borrar chat y archivos?",
    today: "hoy",
    yesterday: "ayer",
    online: "conectado",
    offline: "reconectando…",
    tooLarge: (max) => `demasiado grande (máx. ${max})`,
    uploadFailed: "error al subir",
    sendFailed: "no se pudo enviar",
    chats: "chats",
    devices: "dispositivos",
    newChat: "nuevo chat",
    newChatPlaceholder: "nombre del chat ⏎",
    here: "aquí",
    open: "abierto",
    idle: "en línea",
    away: "desconectado",
    attach: "adjuntar archivos",
    send: "enviar",
  },
};
const lang = navigator.language?.toLowerCase().startsWith("es") ? "es" : "en";
const t = STRINGS[lang];
document.documentElement.lang = lang;
for (const node of document.querySelectorAll("[data-t]")) node.textContent = t[node.dataset.t];
for (const node of document.querySelectorAll("[data-t-title]")) {
  node.title = t[node.dataset.tTitle];
  node.setAttribute("aria-label", t[node.dataset.tTitle]);
}
els.input.placeholder = t.placeholder;
els.newChatInput.placeholder = t.newChatPlaceholder;
els.emptyText.textContent = t.empty;

// ---------- API ----------

// In -dev mode, ?as=<device> lets two tabs pose as two machines.
const AS = new URLSearchParams(location.search).get("as");
function api(path) {
  if (!AS) return path;
  return path + (path.includes("?") ? "&" : "?") + "as=" + encodeURIComponent(AS);
}

async function request(method, path, body) {
  const res = await fetch(api(path), {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
  return res.status === 204 ? null : res.json();
}
const getJSON = (path) => request("GET", path);

// ---------- State ----------

const PAGE = 50;
const state = {
  me: null,
  maxUpload: 0,
  chats: new Map(), // id -> chat
  chat: null, // current chat id
  unread: new Map(), // chat id -> count
  msgs: new Map(), // id -> message (current chat only)
  nodes: new Map(), // id -> element
  hasMore: true,
  loadingOlder: false,
  stick: true, // keep the view pinned to the bottom
  devices: [],
  active: new Set(),
};

const storage = {
  get(k) {
    try {
      return localStorage.getItem(k);
    } catch {
      return null;
    }
  },
  set(k, v) {
    try {
      localStorage.setItem(k, v);
    } catch {}
  },
};

// ---------- Formatting ----------

const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const dateFmt = new Intl.DateTimeFormat(undefined, { weekday: "long", day: "numeric", month: "long" });
const shortDateFmt = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });

const sameDay = (a, b) => a.toDateString() === b.toDateString();
function isYesterday(d) {
  const y = new Date();
  y.setDate(y.getDate() - 1);
  return sameDay(d, y);
}
function dayLabel(d) {
  if (sameDay(d, new Date())) return t.today;
  if (isYesterday(d)) return t.yesterday;
  return dateFmt.format(d);
}
function shortWhen(d) {
  if (sameDay(d, new Date())) return timeFmt.format(d);
  if (isYesterday(d)) return t.yesterday;
  return shortDateFmt.format(d);
}

function formatSize(n) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i && n < 10 ? 1 : 0)} ${units[i]}`;
}

// Each device gets a color from the logo's flame, stable per name.
const FLAME = ["--flame-1", "--flame-2", "--flame-3", "--flame-4", "--flame-5", "--flame-6"];
function deviceColor(name) {
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return `var(${FLAME[h % FLAME.length]})`;
}

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === "class") node.className = v;
    else if (k === "style") node.style.cssText = v;
    else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
    else node.setAttribute(k, v);
  }
  node.append(...children.filter((c) => c != null));
  return node;
}

// Plain text with clickable links. Built with text nodes, never innerHTML.
const URL_RE = /\bhttps?:\/\/[^\s<>"']+[^\s<>"'.,;:!?)\]]/g;
function linkify(text) {
  const frag = document.createDocumentFragment();
  let last = 0;
  for (const m of text.matchAll(URL_RE)) {
    frag.append(text.slice(last, m.index));
    frag.append(el("a", { href: m[0], target: "_blank", rel: "noopener noreferrer" }, m[0]));
    last = m.index + m[0].length;
  }
  frag.append(text.slice(last));
  return frag;
}

// Splits ``` fenced blocks into <pre> elements.
function renderText(body) {
  const frag = document.createDocumentFragment();
  const parts = body.split(/```[^\n]*\n?/);
  parts.forEach((part, i) => {
    if (i % 2 === 1) {
      frag.append(el("pre", { title: t.copy }, part.replace(/\n$/, "")));
    } else {
      const trimmed = i > 0 ? part.replace(/^\n/, "") : part;
      const clean = i < parts.length - 1 ? trimmed.replace(/\n$/, "") : trimmed;
      if (clean) frag.append(linkify(clean));
    }
  });
  return frag;
}

const fileURL = (f, download) => api(`/f/${f.id}/${encodeURIComponent(f.name)}${download ? "?dl=1" : ""}`);
const isImage = (f) => /^image\/(png|jpe?g|gif|webp|avif|bmp)$/.test(f.mime);
const isVideo = (f) => /^video\/(mp4|webm|ogg|quicktime)$/.test(f.mime);
const isAudio = (f) => f.mime.startsWith("audio/");

// ---------- Messages ----------

function renderMessage(m) {
  const mine = state.me && m.device === state.me.device;
  const when = new Date(m.created_at);
  const color = deviceColor(m.device);

  const head = el(
    "div",
    { class: "head" },
    el("span", { class: "sq" }),
    el("span", { class: "device" }, m.device),
    el("span", {}, timeFmt.format(when)),
  );

  let bubble;
  const actions = el("div", { class: "actions" });

  if (m.kind === "text") {
    bubble = el("div", { class: "bubble" }, renderText(m.body));
    bubble.addEventListener("click", (e) => {
      if (e.target.closest("a") || window.getSelection().toString()) return;
      const pre = e.target.closest("pre");
      copyText(pre ? pre.textContent : m.body, bubble);
    });
    actions.append(el("button", { type: "button", onclick: () => copyText(m.body, bubble) }, t.copy));
  } else {
    const f = m.file;
    if (isImage(f)) {
      const img = el("img", { src: fileURL(f), alt: f.name, loading: "lazy", decoding: "async" });
      img.addEventListener("load", keepPinned);
      img.addEventListener("click", () => openLightbox(fileURL(f)));
      bubble = el("div", { class: "bubble media" }, img);
      actions.append(el("button", { type: "button", onclick: () => copyImage(f, bubble) }, t.copyImage));
    } else if (isVideo(f)) {
      const video = el("video", { src: fileURL(f), controls: "", preload: "metadata", playsinline: "" });
      video.addEventListener("loadedmetadata", keepPinned);
      bubble = el("div", { class: "bubble media" }, video);
    } else if (isAudio(f)) {
      bubble = el("div", { class: "bubble media" }, el("audio", { src: fileURL(f), controls: "", preload: "metadata" }));
    } else {
      const ext = (f.name.match(/\.([a-z0-9]{1,5})$/i)?.[1] ?? "file").slice(0, 4);
      bubble = el(
        "a",
        { class: "bubble filebox", href: fileURL(f, true), download: f.name },
        el("span", { class: "ficon" }, ext),
        el("span", { class: "meta" }, el("span", { class: "name", title: f.name }, f.name), el("span", { class: "size" }, formatSize(f.size))),
      );
    }
    actions.append(el("a", { href: fileURL(f, true), download: f.name }, t.download));
  }

  actions.append(
    confirmButton(t.delete, t.confirmDelete, "button", async () => {
      await request("DELETE", `/api/messages/${m.id}`).catch(() => {});
      removeMessage(m.id);
    }),
  );

  return el("div", { class: `msg${mine ? " mine" : ""}`, "data-id": m.id, style: `--c:${color}` }, head, bubble, actions);
}

// A button that needs a second click within 3s to act.
function confirmButton(label, confirmLabel, cls, action) {
  let timer;
  const btn = el("button", { type: "button", class: cls }, label);
  const reset = () => {
    btn.classList.remove("confirm");
    btn.textContent = label;
  };
  btn.addEventListener("click", async (e) => {
    e.stopPropagation();
    if (!btn.classList.contains("confirm")) {
      btn.classList.add("confirm");
      btn.textContent = confirmLabel;
      timer = setTimeout(reset, 3000);
      return;
    }
    clearTimeout(timer);
    reset();
    await action();
  });
  return btn;
}

const dayNodes = new Map();

// Rebuilds the list from cached nodes: cheap, keeps media elements alive,
// and makes grouping/day separators trivially correct.
function render() {
  const ids = [...state.msgs.keys()].sort((a, b) => a - b);
  const children = [];
  let prev = null;
  for (const id of ids) {
    const m = state.msgs.get(id);
    let node = state.nodes.get(id);
    if (!node) {
      node = renderMessage(m);
      state.nodes.set(id, node);
    }
    const when = new Date(m.created_at);
    const day = when.toDateString();
    const newDay = !prev || new Date(prev.created_at).toDateString() !== day;
    if (newDay) {
      if (!dayNodes.has(day)) dayNodes.set(day, el("div", { class: "day" }, dayLabel(when)));
      children.push(dayNodes.get(day));
    }
    const first = newDay || prev.device !== m.device || when - new Date(prev.created_at) > 5 * 60 * 1000;
    node.classList.toggle("first", first);
    children.push(node);
    prev = m;
  }
  els.messages.replaceChildren(...children);
  els.empty.hidden = ids.length > 0 || els.uploads.children.length > 0;
}

function addMessages(list) {
  let changed = false;
  for (const m of list) {
    if (m.chat_id === state.chat && !state.msgs.has(m.id)) {
      state.msgs.set(m.id, m);
      changed = true;
    }
  }
  return changed;
}

function removeMessage(id) {
  if (!state.msgs.delete(id)) return;
  state.nodes.delete(id);
  render();
}

// ---------- Scrolling ----------

const nearBottom = () => els.log.scrollHeight - els.log.scrollTop - els.log.clientHeight < 120;
function scrollToBottom() {
  els.log.scrollTop = els.log.scrollHeight;
}
function keepPinned() {
  if (state.stick) scrollToBottom();
}

els.log.addEventListener("scroll", () => {
  state.stick = nearBottom();
  if (els.log.scrollTop < 200) loadOlder();
});

// ---------- Loading messages ----------

// Fetches the newest page of the current chat and reconciles it with what
// we have. Called on chat switch and after every reconnect, so missed
// messages and deletions heal.
async function syncLatest() {
  const chat = state.chat;
  // Only what we knew before asking can be stale; anything that arrives
  // while the request is in flight (our own sends, live events) is newer.
  const known = [...state.msgs.keys()];
  const latest = await getJSON(`/api/chats/${chat}/messages?limit=${PAGE}`);
  if (chat !== state.chat) return; // switched away meanwhile
  const ids = latest.map((m) => m.id);
  const newestKnown = known.length ? Math.max(...known) : 0;
  const gap = known.length && latest.length === PAGE && ids[0] > newestKnown;
  if (!known.length || gap) state.hasMore = latest.length === PAGE;
  const keep = new Set(ids);
  for (const id of known) {
    if (gap || ((!ids.length || id >= ids[0]) && !keep.has(id))) {
      state.msgs.delete(id);
      state.nodes.delete(id);
    }
  }
  addMessages(latest);
  const wasPinned = state.stick;
  render();
  if (wasPinned) scrollToBottom();
}

async function loadOlder() {
  if (state.loadingOlder || !state.hasMore || !state.msgs.size) return;
  state.loadingOlder = true;
  els.loader.hidden = false;
  const chat = state.chat;
  try {
    const oldest = Math.min(...state.msgs.keys());
    const older = await getJSON(`/api/chats/${chat}/messages?before=${oldest}&limit=${PAGE}`);
    if (chat !== state.chat) return;
    state.hasMore = older.length === PAGE;
    const before = els.log.scrollHeight;
    if (addMessages(older)) {
      render();
      els.log.scrollTop += els.log.scrollHeight - before;
    }
  } catch (err) {
    console.error(err);
  } finally {
    els.loader.hidden = true;
    state.loadingOlder = false;
  }
}

// ---------- Chats ----------

function sortedChats() {
  return [...state.chats.values()].sort((a, b) => new Date(b.last_at) - new Date(a.last_at) || b.id - a.id);
}

function renderChats() {
  const items = sortedChats().map((c) => {
    const unread = state.unread.get(c.id);
    return el(
      "li",
      { class: `chat-item${c.id === state.chat ? " current" : ""}`, onclick: () => selectChat(c.id) },
      el("span", { class: "name" }, c.name),
      unread ? el("span", { class: "unread" }, unread > 99 ? "99+" : String(unread)) : el("span", { class: "when" }, shortWhen(new Date(c.last_at))),
    );
  });
  els.chatList.replaceChildren(...items);
  const current = state.chats.get(state.chat);
  els.chatTitle.textContent = current ? current.name : "";
  document.title = current ? `${current.name} · pneuma` : "pneuma";
}

async function loadChats() {
  const chats = await getJSON("/api/chats");
  state.chats = new Map(chats.map((c) => [c.id, c]));
  if (!state.chats.has(state.chat)) {
    const saved = Number(storage.get("pneuma.chat"));
    state.chat = state.chats.has(saved) ? saved : sortedChats()[0]?.id;
    state.msgs.clear();
    state.nodes.clear();
  }
  renderChats();
}

function selectChat(id) {
  els.app.classList.remove("drawer");
  els.scrim.hidden = true;
  if (id === state.chat) return;
  state.chat = id;
  state.unread.delete(id);
  storage.set("pneuma.chat", String(id));
  state.msgs.clear();
  state.nodes.clear();
  state.hasMore = true;
  state.stick = true;
  els.messages.replaceChildren();
  renderChats();
  syncLatest().catch(console.error);
  if (!coarse) els.input.focus();
}

function upsertChat(c) {
  state.chats.set(c.id, c);
  renderChats();
}

// Bumps a chat's activity when a message arrives, without refetching.
function touchChat(m) {
  const c = state.chats.get(m.chat_id);
  if (!c) return;
  c.last_at = m.created_at;
  c.count++;
  if (m.chat_id !== state.chat && m.device !== state.me?.device) {
    state.unread.set(m.chat_id, (state.unread.get(m.chat_id) ?? 0) + 1);
  }
  renderChats();
}

function chatDeleted(id) {
  state.chats.delete(id);
  state.unread.delete(id);
  if (id === state.chat) {
    const next = sortedChats()[0];
    state.chat = null;
    if (next) selectChat(next.id);
  }
  renderChats();
}

els.newChat.addEventListener("click", () => {
  els.newChatForm.hidden = false;
  els.newChatInput.value = "";
  els.newChatInput.focus();
});
els.newChatInput.addEventListener("keydown", (e) => {
  if (e.key === "Escape") els.newChatForm.hidden = true;
});
els.newChatInput.addEventListener("blur", () => {
  if (!els.newChatInput.value.trim()) els.newChatForm.hidden = true;
});
els.newChatForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const name = els.newChatInput.value.trim();
  if (!name) return;
  try {
    const c = await request("POST", "/api/chats", { name });
    els.newChatForm.hidden = true;
    upsertChat(c);
    selectChat(c.id);
  } catch (err) {
    toast(String(err.message), true);
  }
});

function startRename() {
  const c = state.chats.get(state.chat);
  if (!c) return;
  const input = el("input", { class: "chat-title-input", maxlength: "80", value: c.name, spellcheck: "false" });
  let done = false;
  const finish = async (save) => {
    if (done) return;
    done = true;
    input.remove();
    const name = input.value.trim();
    if (save && name && name !== c.name) {
      try {
        upsertChat(await request("PATCH", `/api/chats/${c.id}`, { name }));
      } catch (err) {
        toast(String(err.message), true);
      }
    }
  };
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") finish(true);
    if (e.key === "Escape") finish(false);
  });
  input.addEventListener("blur", () => finish(true));
  els.chatTitle.after(input);
  input.select();
}
els.chatTitle.addEventListener("dblclick", startRename);
els.renameChat.addEventListener("click", startRename);
els.deleteChat.replaceWith(
  (els.deleteChat = confirmButton(t.delete, t.deleteChatConfirm, "tool danger", async () => {
    const id = state.chat;
    try {
      await request("DELETE", `/api/chats/${id}`);
      chatDeleted(id);
    } catch (err) {
      toast(String(err.message), true);
    }
  })),
);

// ---------- Devices ----------

async function loadDevices() {
  try {
    state.devices = await getJSON("/api/devices");
    state.active = new Set(state.devices.filter((d) => d.active).map((d) => d.name));
    renderDevices();
  } catch (err) {
    console.error(err);
  }
}

function renderDevices() {
  const me = state.me?.device;
  const rank = (d) => (d.name === me ? 0 : state.active.has(d.name) ? 1 : d.online ? 2 : 3);
  const list = [...state.devices].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
  els.deviceList.replaceChildren(
    ...list.map((d) => {
      const active = state.active.has(d.name);
      const label = d.name === me ? t.here : active ? t.open : d.online ? t.idle : t.away;
      const cls = active ? "active" : d.online ? "" : "offline";
      return el(
        "li",
        { class: `device ${cls}`, style: `--c:${deviceColor(d.name)}`, title: d.os || "" },
        el("span", { class: "sq" }),
        el("span", { class: "name" }, d.name),
        el("span", { class: "state" }, label),
      );
    }),
  );
}

// ---------- Live updates ----------

function connect() {
  const es = new EventSource(api("/api/events"));
  es.addEventListener("open", async () => {
    setOnline(true);
    try {
      await loadChats();
      await syncLatest();
      loadDevices();
    } catch (err) {
      console.error(err);
    }
  });
  es.addEventListener("error", () => setOnline(false));
  es.addEventListener("message", (e) => {
    const m = JSON.parse(e.data);
    touchChat(m);
    const pinned = nearBottom() || (state.me && m.device === state.me.device);
    if (addMessages([m])) {
      render();
      if (pinned) scrollToBottom();
    }
  });
  es.addEventListener("delete", (e) => removeMessage(JSON.parse(e.data).id));
  es.addEventListener("chat", (e) => upsertChat(JSON.parse(e.data)));
  es.addEventListener("chat_delete", (e) => chatDeleted(JSON.parse(e.data).id));
  es.addEventListener("presence", (e) => {
    const active = JSON.parse(e.data).active;
    const unknown = active.some((name) => !state.devices.some((d) => d.name === name));
    state.active = new Set(active);
    if (unknown) loadDevices();
    else renderDevices();
  });
}

function setOnline(on) {
  els.status.className = `status ${on ? "online" : "offline"}`;
  els.status.textContent = on ? `● ${t.online}` : `○ ${t.offline}`;
}

// ---------- Sending ----------

async function sendText(text) {
  if (!text.trim()) return false;
  try {
    const m = await request("POST", `/api/chats/${state.chat}/messages`, { text });
    if (addMessages([m])) render();
    scrollToBottom();
    return true;
  } catch (err) {
    console.error(err);
    toast(t.sendFailed, true);
    return false;
  }
}

let uploadQueue = Promise.resolve();

function uploadFiles(files) {
  const chat = state.chat;
  for (const file of files) {
    if (state.maxUpload && file.size > state.maxUpload) {
      toast(`${file.name}: ${t.tooLarge(formatSize(state.maxUpload))}`, true);
      continue;
    }
    const bar = el("span");
    const item = el("div", { class: "upload" }, el("div", { class: "name" }, file.name), el("div", { class: "bar" }, bar));
    els.uploads.append(item);
    els.empty.hidden = true;
    scrollToBottom();
    // One at a time keeps progress readable and the server's disk writes sequential.
    uploadQueue = uploadQueue.then(() => uploadOne(chat, file, item, bar));
  }
}

function uploadOne(chat, file, item, bar) {
  return new Promise((resolve) => {
    const form = new FormData();
    form.append("file", file, file.name);
    const xhr = new XMLHttpRequest();
    xhr.open("POST", api(`/api/chats/${chat}/files`));
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) bar.style.width = `${(e.loaded / e.total) * 100}%`;
    });
    xhr.addEventListener("load", () => {
      if (xhr.status === 201) {
        item.remove();
        if (addMessages(JSON.parse(xhr.responseText))) render();
        scrollToBottom();
      } else {
        fail(xhr.status === 413 ? t.tooLarge(formatSize(state.maxUpload)) : t.uploadFailed);
      }
      resolve();
    });
    xhr.addEventListener("error", () => {
      fail(t.uploadFailed);
      resolve();
    });
    xhr.send(form);

    function fail(msg) {
      item.classList.add("failed");
      item.firstChild.textContent = `${file.name} — ${msg}`;
      setTimeout(() => {
        item.remove();
        render();
      }, 6000);
    }
  });
}

// Screenshots arrive as "image.png"; give them a useful, unique name.
function nameClipboardFile(file) {
  if (!/^image\.\w+$/.test(file.name)) return file;
  const d = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  const stamp = `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`;
  const ext = file.name.split(".").pop();
  return new File([file], `${lang === "es" ? "captura" : "screenshot"}-${stamp}.${ext}`, { type: file.type });
}

// ---------- Copying ----------

async function copyText(text, bubble) {
  try {
    await navigator.clipboard.writeText(text);
    flashCopied(bubble);
  } catch {
    toast(t.copyFailed, true);
  }
}

async function copyImage(f, bubble) {
  try {
    // Browsers only reliably accept PNG on the clipboard, so convert others.
    const png = fetch(fileURL(f))
      .then((r) => r.blob())
      .then((blob) => (blob.type === "image/png" ? blob : toPNG(blob)));
    await navigator.clipboard.write([new ClipboardItem({ "image/png": png })]);
    flashCopied(bubble);
  } catch (err) {
    console.error(err);
    toast(t.copyFailed, true);
  }
}

async function toPNG(blob) {
  const bmp = await createImageBitmap(blob);
  const canvas = document.createElement("canvas");
  canvas.width = bmp.width;
  canvas.height = bmp.height;
  canvas.getContext("2d").drawImage(bmp, 0, 0);
  return new Promise((resolve) => canvas.toBlob(resolve, "image/png"));
}

function flashCopied(bubble) {
  toast(t.copied);
  if (!bubble) return;
  bubble.classList.add("copied");
  setTimeout(() => bubble.classList.remove("copied"), 700);
}

let toastTimer;
function toast(text, error = false) {
  els.toast.textContent = text;
  els.toast.className = `toast${error ? " error" : ""}`;
  els.toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (els.toast.hidden = true), error ? 3500 : 1200);
}

function openLightbox(src) {
  const box = el("div", { class: "lightbox" }, el("img", { src }));
  const close = () => {
    box.remove();
    document.removeEventListener("keydown", onKey);
  };
  const onKey = (e) => e.key === "Escape" && close();
  box.addEventListener("click", close);
  document.addEventListener("keydown", onKey);
  document.body.append(box);
}

// ---------- Composer ----------

const coarse = matchMedia("(pointer: coarse)").matches;

function autosize() {
  els.input.style.height = "auto";
  els.input.style.height = `${els.input.scrollHeight}px`;
}

els.input.addEventListener("input", autosize);
els.input.addEventListener("keydown", (e) => {
  // On touch keyboards Enter is a newline; the send button sends.
  if (e.key === "Enter" && !e.shiftKey && !coarse && !e.isComposing) {
    e.preventDefault();
    els.composer.requestSubmit();
  }
});

els.composer.addEventListener("submit", async (e) => {
  e.preventDefault();
  const text = els.input.value;
  if (!text.trim()) return;
  els.input.value = "";
  autosize();
  if (!(await sendText(text))) {
    els.input.value = text;
    autosize();
  }
  if (!coarse) els.input.focus();
});

els.attach.addEventListener("click", () => els.fileInput.click());
els.fileInput.addEventListener("change", () => {
  uploadFiles([...els.fileInput.files]);
  els.fileInput.value = "";
});

const typing = (target) => target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement;

// Ctrl+V anywhere: files and screenshots upload, text lands in the composer.
document.addEventListener("paste", (e) => {
  const files = [...(e.clipboardData?.files ?? [])];
  if (files.length) {
    e.preventDefault();
    uploadFiles(files.map(nameClipboardFile));
    return;
  }
  if (!typing(e.target)) {
    const text = e.clipboardData?.getData("text/plain");
    if (!text) return;
    e.preventDefault();
    els.input.focus();
    els.input.setRangeText(text, els.input.selectionStart, els.input.selectionEnd, "end");
    autosize();
  }
});

// Typing anywhere goes to the composer.
document.addEventListener("keydown", (e) => {
  if (typing(e.target) || e.ctrlKey || e.metaKey || e.altKey || e.key.length !== 1) return;
  if (document.querySelector(".lightbox")) return;
  els.input.focus();
});

// Drag and drop anywhere in the window.
let dragDepth = 0;
const hasFiles = (e) => [...(e.dataTransfer?.types ?? [])].includes("Files");
window.addEventListener("dragenter", (e) => {
  if (!hasFiles(e)) return;
  e.preventDefault();
  dragDepth++;
  els.drop.hidden = false;
});
window.addEventListener("dragover", (e) => {
  if (hasFiles(e)) e.preventDefault();
});
window.addEventListener("dragleave", () => {
  if (--dragDepth <= 0) {
    dragDepth = 0;
    els.drop.hidden = true;
  }
});
window.addEventListener("drop", (e) => {
  if (!hasFiles(e)) return;
  e.preventDefault();
  dragDepth = 0;
  els.drop.hidden = true;
  uploadFiles([...e.dataTransfer.files]);
});

// Mobile drawer.
els.menuBtn.addEventListener("click", () => {
  els.app.classList.add("drawer");
  els.scrim.hidden = false;
});
els.scrim.addEventListener("click", () => {
  els.app.classList.remove("drawer");
  els.scrim.hidden = true;
});

// ---------- Boot ----------

async function boot() {
  try {
    const me = await getJSON("/api/me");
    state.me = me.sender;
    state.maxUpload = me.max_upload;
  } catch (err) {
    els.empty.hidden = false;
    els.emptyText.textContent = String(err.message);
    return;
  }
  connect();
  // Tailnet online/offline changes aren't pushed; refresh now and then.
  setInterval(loadDevices, 30000);
  if (!coarse) els.input.focus();
  if ("serviceWorker" in navigator && !AS) navigator.serviceWorker.register("/sw.js").catch(console.error);
}

boot();
