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
  chatMembers: $("chat-members"),
  deviceList: $("device-list"),
  menuBtn: $("menu-btn"),
  scrim: $("scrim"),
  ctx: $("ctx"),
  settings: $("settings"),
  settingsBtn: $("settings-btn"),
  tabMessages: $("tab-messages"),
  tabFiles: $("tab-files"),
  filesCount: $("files-count"),
  filesView: $("files"),
  mediaGrid: $("media-grid"),
  docList: $("doc-list"),
  mediaCount: $("media-count"),
  docsCount: $("docs-count"),
  mediaSection: $("files-media-section"),
  docsSection: $("files-docs-section"),
  filesEmpty: $("files-empty"),
};

// ---------- Local preferences (this device only) ----------

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

const coarse = matchMedia("(pointer: coarse)").matches;
const PREF_DEFAULTS = { lang: "auto", size: 13, enterSends: !coarse, notify: false, anim: true };
const prefs = { ...PREF_DEFAULTS };
try {
  Object.assign(prefs, JSON.parse(storage.get("pneuma.prefs") || "{}"));
} catch {}
const savePrefs = () => storage.set("pneuma.prefs", JSON.stringify(prefs));
document.documentElement.style.setProperty("--fs", `${prefs.size}px`);
const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;
const applyAnim = () => document.documentElement.classList.toggle("anim", prefs.anim && !reducedMotion);
applyAnim();

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
    settings: "settings",
    thisDevice: "this device",
    language: "language",
    textSize: "text size",
    enterSends: "enter sends",
    notifications: "notifications",
    animations: "animations",
    allDevices: "all devices",
    retention: "delete messages after",
    forever: "never",
    maxUpload: "max upload",
    storage: "storage",
    usage: (m, f, b) => `${m} messages · ${f} files · ${b}`,
    security: "security",
    securityNote:
      "In transit: WireGuard (Tailscale) + HTTPS, end to end.\nAt rest: messages and files are stored unencrypted on the server running pneuma.",
    renameChat: "rename chat",
    deleteChat: "delete chat",
    renameDevice: "rename device",
    resetName: "restore name",
    notifyBlocked: "notifications are blocked in this browser",
    startChat: "start private chat",
    addToChat: (name) => `add to “${name}”`,
    privateChat: "private chat",
    expired: "expired",
    fileRetention: "delete files after",
    messagesTab: "messages",
    filesTab: "files",
    mediaTab: "images & videos",
    docsTab: "documents",
    noFiles: "No files in this chat yet.",
    close: "close",
    previous: "previous",
    next: "next",
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
    settings: "ajustes",
    thisDevice: "este dispositivo",
    language: "idioma",
    textSize: "tamaño de texto",
    enterSends: "enter envía",
    notifications: "notificaciones",
    animations: "animaciones",
    allDevices: "todos los dispositivos",
    retention: "borrar mensajes tras",
    forever: "nunca",
    maxUpload: "subida máxima",
    storage: "almacenamiento",
    usage: (m, f, b) => `${m} mensajes · ${f} archivos · ${b}`,
    security: "seguridad",
    securityNote:
      "En tránsito: WireGuard (Tailscale) + HTTPS, de extremo a extremo.\nEn reposo: los mensajes y archivos se guardan sin cifrar en el servidor donde corre pneuma.",
    renameChat: "renombrar chat",
    deleteChat: "borrar chat",
    renameDevice: "renombrar dispositivo",
    resetName: "restaurar nombre",
    notifyBlocked: "las notificaciones están bloqueadas en este navegador",
    startChat: "empezar chat privado",
    addToChat: (name) => `añadir a «${name}»`,
    privateChat: "chat privado",
    expired: "caducado",
    fileRetention: "borrar archivos tras",
    messagesTab: "mensajes",
    filesTab: "archivos",
    mediaTab: "imágenes y vídeos",
    docsTab: "documentos",
    noFiles: "Todavía no hay archivos en este chat.",
    close: "cerrar",
    previous: "anterior",
    next: "siguiente",
  },
};
const lang = prefs.lang !== "auto" ? prefs.lang : navigator.language?.toLowerCase().startsWith("es") ? "es" : "en";
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
  aliases: {}, // tailnet name -> display name
  view: "messages", // or "files"
  files: [], // file messages of the current chat, newest first
};

const displayName = (device) => state.aliases[device] || device;

// Chat members are stable node IDs; show them by (aliased) device name.
function memberName(id) {
  if (id === state.me?.id) return displayName(state.me.device);
  const d = state.devices.find((x) => x.id === id);
  return d ? displayName(d.name) : "?";
}

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

const LOCK_SVG = '<svg viewBox="0 0 8 9" width="9" height="10" shape-rendering="crispEdges" aria-hidden="true"><path fill="currentColor" d="M2 0h4v1h1v3h1v5H0V4h1V1h1Zm0 1v3h4V1Z"/></svg>';
function lockIcon(cls = "lock") {
  const span = el("span", { class: cls, title: t.privateChat });
  span.innerHTML = LOCK_SVG; // constant markup, no user data
  return span;
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
    el("span", { class: "device", title: m.device }, displayName(m.device)),
    el("span", {}, timeFmt.format(when)),
  );

  let bubble;
  const actions = el("div", { class: "actions" });

  if (m.kind === "expired") {
    bubble = el("div", { class: "bubble expired" }, `📎 ${m.body} — ${t.expired}`);
  } else if (m.kind === "text") {
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
      img.addEventListener("click", () => openGallery(m.id));
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
  if (state.files.some((m) => m.id === id)) {
    state.files = state.files.filter((m) => m.id !== id);
    renderFiles();
  }
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
    const item = el(
      "li",
      {
        class: `chat-item${c.id === state.chat ? " current" : ""}`,
        title: c.members ? c.members.map(memberName).join(", ") : "",
        onclick: () => selectChat(c.id),
      },
      el("span", { class: "name" }, c.name),
      c.members ? lockIcon() : null,
      unread ? el("span", { class: "unread" }, unread > 99 ? "99+" : String(unread)) : el("span", { class: "when" }, shortWhen(new Date(c.last_at))),
    );
    onMenu(item, () => [
      { label: t.renameChat, action: () => renameChat(c.id, item.querySelector(".name")) },
      { label: t.deleteChat, confirm: t.deleteChatConfirm, danger: true, action: () => deleteChat(c.id) },
    ]);
    return item;
  });
  els.chatList.replaceChildren(...items);
  const current = state.chats.get(state.chat);
  els.chatTitle.textContent = current ? current.name : "";
  els.chatMembers.hidden = !current?.members;
  if (current?.members) {
    els.chatMembers.replaceChildren(lockIcon("lock-h"), current.members.map(memberName).join(", "));
    els.chatMembers.title = t.privateChat;
  }
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
  state.files = [];
  renderFiles();
  renderChats();
  syncLatest().catch(console.error);
  loadFiles();
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

// Replaces target's text with an input; calls save(newValue) on Enter or
// blur, restores on Escape.
function inlineEdit(target, value, save, cls = "chat-title-input") {
  const input = el("input", { class: cls, maxlength: "80", spellcheck: "false" });
  input.value = value;
  let done = false;
  const finish = async (commit) => {
    if (done) return;
    done = true;
    input.remove();
    target.hidden = false;
    const v = input.value.trim();
    if (commit && v !== value) await save(v);
  };
  input.addEventListener("keydown", (e) => {
    e.stopPropagation();
    if (e.key === "Enter") finish(true);
    if (e.key === "Escape") finish(false);
  });
  input.addEventListener("click", (e) => e.stopPropagation());
  input.addEventListener("blur", () => finish(true));
  target.hidden = true;
  target.after(input);
  input.focus();
  input.select();
}

function renameChat(id, target = els.chatTitle) {
  const c = state.chats.get(id);
  if (!c) return;
  inlineEdit(target, c.name, async (name) => {
    if (!name) return;
    try {
      upsertChat(await request("PATCH", `/api/chats/${id}`, { name }));
    } catch (err) {
      toast(String(err.message), true);
    }
  });
}

async function deleteChat(id) {
  try {
    await request("DELETE", `/api/chats/${id}`);
    chatDeleted(id);
  } catch (err) {
    toast(String(err.message), true);
  }
}

els.chatTitle.addEventListener("dblclick", () => renameChat(state.chat));
els.chatTitle.addEventListener("contextmenu", (e) => {
  e.preventDefault();
  const c = state.chats.get(state.chat);
  if (!c) return;
  openMenu(e.clientX, e.clientY, [
    { label: t.renameChat, action: () => renameChat(c.id) },
    { label: t.deleteChat, confirm: t.deleteChatConfirm, danger: true, action: () => deleteChat(c.id) },
  ]);
});

// ---------- Context menu ----------

// Right click (or long press on touch) opens a small menu built by items().
function onMenu(node, items) {
  node.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    openMenu(e.clientX, e.clientY, items());
  });
  let timer;
  node.addEventListener("touchstart", (e) => {
    const touch = e.touches[0];
    timer = setTimeout(() => openMenu(touch.clientX, touch.clientY, items()), 500);
  }, { passive: true });
  for (const ev of ["touchend", "touchmove", "touchcancel"]) node.addEventListener(ev, () => clearTimeout(timer));
}

function openMenu(x, y, items) {
  const buttons = items.map((it) => {
    const btn = el("button", { type: "button", role: "menuitem", class: it.danger ? "danger" : "" }, it.label);
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (it.confirm && !btn.dataset.armed) {
        btn.dataset.armed = "1";
        btn.textContent = it.confirm;
        return;
      }
      closeMenu();
      it.action();
    });
    return btn;
  });
  els.ctx.replaceChildren(...buttons);
  els.ctx.hidden = false;
  const r = els.ctx.getBoundingClientRect();
  els.ctx.style.left = `${Math.min(x, innerWidth - r.width - 4)}px`;
  els.ctx.style.top = `${Math.min(y, innerHeight - r.height - 4)}px`;
  buttons[0]?.focus();
}

function closeMenu() {
  els.ctx.hidden = true;
}
document.addEventListener("click", (e) => {
  if (!els.ctx.contains(e.target)) closeMenu();
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") closeMenu();
});
window.addEventListener("blur", closeMenu);
window.addEventListener("resize", closeMenu);

// ---------- Devices ----------

async function loadDevices() {
  try {
    state.devices = await getJSON("/api/devices");
    state.active = new Set(state.devices.filter((d) => d.active).map((d) => d.name));
    renderDevices();
    renderChats(); // member names come from the device list
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
      const here = d.name === me;
      const active = here || state.active.has(d.name);
      const label = here ? t.here : active ? t.open : d.online ? t.idle : t.away;
      const cls = `${active ? "active" : d.online ? "" : "offline"}${here ? " here" : ""}`;
      const name = el("span", { class: "name", title: [d.name, d.os].filter(Boolean).join(" · ") }, displayName(d.name));
      const item = el("li", { class: `dev ${cls}` }, el("span", { class: "sq" }), name, el("span", { class: "state" }, label));
      const rename = () =>
        inlineEdit(name, displayName(d.name), (alias) => setAlias(d.name, alias || d.name), "device-rename");
      const items = () => {
        const current = state.chats.get(state.chat);
        return [
          ...(here ? [] : [{ label: t.startChat, action: () => startPrivateChat(d) }]),
          ...(!here && current?.members && !current.members.includes(d.id)
            ? [{ label: t.addToChat(current.name), action: () => addMember(current.id, d.id) }]
            : []),
          { label: t.renameDevice, action: rename },
          ...(state.aliases[d.name] ? [{ label: `${t.resetName} (${d.name})`, action: () => setAlias(d.name, "") }] : []),
        ];
      };
      onMenu(item, items);
      // A plain click opens the same menu, anchored to the row.
      item.addEventListener("click", (e) => {
        if (e.target.closest("input")) return;
        e.stopPropagation();
        const r = item.getBoundingClientRect();
        openMenu(Math.min(e.clientX, r.right - 180), r.bottom, items());
      });
      return item;
    }),
  );
}

// Opens the private chat with exactly {me, device}, creating it if needed.
async function startPrivateChat(d) {
  const want = [state.me.id, d.id].sort().join("\n");
  const existing = [...state.chats.values()].find((c) => c.members && [...c.members].sort().join("\n") === want);
  if (existing) return selectChat(existing.id);
  try {
    const c = await request("POST", "/api/chats", { name: displayName(d.name), members: [d.id] });
    upsertChat(c);
    selectChat(c.id);
  } catch (err) {
    toast(String(err.message), true);
  }
}

async function addMember(chat, device) {
  try {
    upsertChat(await request("POST", `/api/chats/${chat}/members`, { device }));
  } catch (err) {
    toast(String(err.message), true);
  }
}

async function setAlias(device, alias) {
  try {
    applyAliases(await request("PUT", `/api/devices/${encodeURIComponent(device)}/alias`, { alias }));
  } catch (err) {
    toast(String(err.message), true);
  }
}

function applyAliases(aliases) {
  state.aliases = aliases || {};
  state.nodes.clear(); // message heads show the alias
  render();
  renderDevices();
  renderChats();
  renderFiles();
}

// ---------- Settings ----------

const ui = {
  lang: $("set-lang"),
  size: $("set-size"),
  enter: $("set-enter"),
  notify: $("set-notify"),
  anim: $("set-anim"),
  retention: $("set-retention"),
  fileRetention: $("set-file-retention"),
  max: $("set-max"),
  usage: $("set-usage"),
};

async function openSettings() {
  ui.lang.value = prefs.lang;
  ui.size.value = String(prefs.size);
  ui.enter.checked = prefs.enterSends;
  ui.anim.checked = prefs.anim;
  ui.notify.checked = prefs.notify && "Notification" in window && Notification.permission === "granted";
  els.settings.showModal();
  try {
    const { settings, usage, aliases } = await getJSON("/api/settings");
    ui.retention.value = String(settings.retention_days);
    setSelect(ui.fileRetention, settings.file_retention_days, (d) => `${d} d`);
    setSelect(ui.max, settings.max_upload, formatSize);
    ui.usage.textContent = t.usage(usage.messages, usage.files, formatSize(usage.bytes));
    state.aliases = aliases;
  } catch (err) {
    console.error(err);
  }
}

// Selects value, adding an option for it if it isn't one of the presets.
function setSelect(select, value, label) {
  const v = String(value);
  if (![...select.options].some((o) => o.value === v)) select.append(el("option", { value: v }, label(value)));
  select.value = v;
}

els.settingsBtn.addEventListener("click", openSettings);
ui.lang.addEventListener("change", () => {
  prefs.lang = ui.lang.value;
  savePrefs();
  location.reload();
});
ui.size.addEventListener("change", () => {
  prefs.size = Number(ui.size.value);
  savePrefs();
  document.documentElement.style.setProperty("--fs", `${prefs.size}px`);
});
ui.anim.addEventListener("change", () => {
  prefs.anim = ui.anim.checked;
  savePrefs();
  applyAnim();
  scheduleLook();
});
ui.enter.addEventListener("change", () => {
  prefs.enterSends = ui.enter.checked;
  savePrefs();
});
ui.notify.addEventListener("change", async () => {
  if (ui.notify.checked && "Notification" in window && Notification.permission !== "granted") {
    const perm = await Notification.requestPermission();
    if (perm !== "granted") {
      ui.notify.checked = false;
      toast(t.notifyBlocked, true);
    }
  }
  prefs.notify = ui.notify.checked;
  savePrefs();
});
async function patchSettings(body) {
  try {
    const st = await request("PATCH", "/api/settings", body);
    state.maxUpload = st.max_upload;
  } catch (err) {
    toast(String(err.message), true);
  }
}
ui.retention.addEventListener("change", () => patchSettings({ retention_days: Number(ui.retention.value) }));
ui.fileRetention.addEventListener("change", () => patchSettings({ file_retention_days: Number(ui.fileRetention.value) }));
ui.max.addEventListener("change", () => patchSettings({ max_upload: Number(ui.max.value) }));

// ---------- Notifications ----------

function notify(m) {
  if (!prefs.notify || !("Notification" in window) || Notification.permission !== "granted") return;
  if (m.device === state.me?.device) return;
  if (!document.hidden && m.chat_id === state.chat) return;
  const chat = state.chats.get(m.chat_id);
  const n = new Notification(`${displayName(m.device)} · ${chat?.name ?? "pneuma"}`, {
    body: m.kind === "text" ? m.body.slice(0, 200) : `📎 ${m.file.name}`,
    icon: "/icons/icon-192.png",
    tag: `pneuma-${m.chat_id}`,
  });
  n.addEventListener("click", () => {
    window.focus();
    selectChat(m.chat_id);
    n.close();
  });
}

// ---------- Live updates ----------

function connect() {
  const es = new EventSource(api("/api/events"));
  es.addEventListener("open", async () => {
    setOnline(true);
    try {
      await loadChats();
      const { aliases } = await getJSON("/api/settings");
      state.aliases = aliases;
      state.nodes.clear();
      await syncLatest();
      loadFiles();
      loadDevices();
    } catch (err) {
      console.error(err);
    }
  });
  es.addEventListener("error", () => setOnline(false));
  es.addEventListener("message", (e) => {
    const m = JSON.parse(e.data);
    touchChat(m);
    notify(m);
    if (m.kind === "file" && m.chat_id === state.chat && !state.files.some((x) => x.id === m.id)) {
      state.files.unshift(m);
      renderFiles();
    }
    const pinned = nearBottom() || (state.me && m.device === state.me.device);
    if (addMessages([m])) {
      render();
      if (pinned) scrollToBottom();
    }
  });
  es.addEventListener("delete", (e) => removeMessage(JSON.parse(e.data).id));
  es.addEventListener("chat", (e) => upsertChat(JSON.parse(e.data)));
  es.addEventListener("aliases", (e) => applyAliases(JSON.parse(e.data)));
  es.addEventListener("settings", (e) => (state.maxUpload = JSON.parse(e.data).max_upload));
  es.addEventListener("resync", () => loadChats().then(syncLatest).catch(console.error));
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
  els.status.replaceChildren(el("span", { class: "dot" }), on ? t.online : t.offline);
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
        const created = JSON.parse(xhr.responseText);
        for (const m of created) if (m.chat_id === state.chat && !state.files.some((x) => x.id === m.id)) state.files.unshift(m);
        renderFiles();
        if (addMessages(created)) render();
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

// ---------- Files view ----------

const isMedia = (f) => isImage(f) || isVideo(f);

let filesTimer;
function loadFiles() {
  clearTimeout(filesTimer);
  filesTimer = setTimeout(async () => {
    const chat = state.chat;
    if (chat == null) return;
    try {
      const files = await getJSON(`/api/chats/${chat}/files`);
      if (chat !== state.chat) return;
      state.files = files;
      renderFiles();
    } catch (err) {
      console.error(err);
    }
  }, 150);
}

function setView(view) {
  state.view = view;
  els.tabMessages.classList.toggle("current", view === "messages");
  els.tabFiles.classList.toggle("current", view === "files");
  els.tabMessages.setAttribute("aria-selected", view === "messages");
  els.tabFiles.setAttribute("aria-selected", view === "files");
  els.log.hidden = view !== "messages";
  els.filesView.hidden = view !== "files";
  if (view === "files") loadFiles();
  else keepPinned();
}
els.tabMessages.addEventListener("click", () => setView("messages"));
els.tabFiles.addEventListener("click", () => setView("files"));

async function deleteMessage(id) {
  await request("DELETE", `/api/messages/${id}`).catch(() => {});
  removeMessage(id);
}

function renderFiles() {
  const media = state.files.filter((m) => isMedia(m.file));
  const docs = state.files.filter((m) => !isMedia(m.file));
  els.filesCount.textContent = state.files.length ? String(state.files.length) : "";
  els.mediaCount.textContent = String(media.length);
  els.docsCount.textContent = String(docs.length);
  els.mediaSection.hidden = !media.length;
  els.docsSection.hidden = !docs.length;
  els.filesEmpty.hidden = state.files.length > 0;

  els.mediaGrid.replaceChildren(
    ...media.map((m) => {
      const f = m.file;
      const thumb = isImage(f)
        ? el("img", { src: fileURL(f), alt: f.name, loading: "lazy", decoding: "async" })
        : el("video", { src: fileURL(f), preload: "metadata", muted: "", playsinline: "" });
      const tile = el(
        "div",
        { class: "tile", title: f.name, tabindex: "0" },
        thumb,
        isVideo(f) ? el("span", { class: "badge" }, "▶ video") : null,
        el(
          "div",
          { class: "tile-actions" },
          el("span", { class: "who" }, displayName(m.device)),
          el(
            "span",
            { class: "mini" },
            el("a", { href: fileURL(f, true), download: f.name, onclick: (e) => e.stopPropagation(), title: t.download }, "↓"),
            confirmButton("×", "¿?", "", () => deleteMessage(m.id)),
          ),
        ),
      );
      tile.addEventListener("click", () => openGallery(m.id));
      tile.addEventListener("keydown", (e) => e.key === "Enter" && openGallery(m.id));
      return tile;
    }),
  );

  els.docList.replaceChildren(
    ...docs.map((m) => {
      const f = m.file;
      const ext = (f.name.match(/\.([a-z0-9]{1,5})$/i)?.[1] ?? "file").slice(0, 4);
      return el(
        "li",
        { class: "doc" },
        el("span", { class: "ficon" }, ext),
        el("a", { class: "name", href: fileURL(f, true), download: f.name, title: f.name }, f.name),
        el("span", { class: "meta who" }, `${displayName(m.device)} · ${shortWhen(new Date(m.created_at))}`),
        el("span", { class: "meta" }, formatSize(f.size)),
        el(
          "span",
          { class: "mini" },
          el("a", { href: fileURL(f, true), download: f.name }, t.download),
          confirmButton(t.delete, t.confirmDelete, "", () => deleteMessage(m.id)),
        ),
      );
    }),
  );
}

// ---------- Gallery ----------

// Every image and video of the current chat we know of, oldest first.
function galleryItems() {
  const byId = new Map();
  for (const m of state.files) if (isMedia(m.file)) byId.set(m.id, m);
  for (const m of state.msgs.values()) if (m.kind === "file" && isMedia(m.file)) byId.set(m.id, m);
  return [...byId.values()].sort((a, b) => a.id - b.id);
}

function openGallery(id) {
  let items = galleryItems();
  let i = Math.max(0, items.findIndex((m) => m.id === id));

  const stage = el("div", { class: "stage" });
  const prev = el("button", { type: "button", class: "nav prev", title: `${t.previous} (←)`, "aria-label": t.previous }, "<");
  const next = el("button", { type: "button", class: "nav next", title: `${t.next} (→)`, "aria-label": t.next }, ">");
  const closeBtn = el("button", { type: "button", class: "close", title: `${t.close} (esc)`, "aria-label": t.close }, "×");
  const caption = el("div", { class: "caption" });
  const box = el("div", { class: "lightbox", role: "dialog", "aria-modal": "true" }, stage, prev, next, closeBtn, caption);

  function show() {
    const m = items[i];
    if (!m) return close();
    const f = m.file;
    const media = isImage(f)
      ? el("img", { src: fileURL(f), alt: f.name })
      : el("video", { src: fileURL(f), controls: "", autoplay: "", playsinline: "" });
    stage.replaceChildren(media);
    prev.disabled = i === 0;
    next.disabled = i === items.length - 1;
    caption.replaceChildren(
      el("span", { class: "pos" }, `${i + 1} / ${items.length}`),
      el("span", { class: "name", title: f.name }, f.name),
      el("span", {}, `${displayName(m.device)} · ${shortWhen(new Date(m.created_at))} · ${formatSize(f.size)}`),
      el("a", { href: fileURL(f, true), download: f.name }, t.download),
      isImage(f) ? el("button", { type: "button", onclick: () => copyImage(f) }, t.copyImage) : null,
    );
    // Warm the neighbours so arrowing through is instant.
    for (const n of [items[i - 1], items[i + 1]]) if (n && isImage(n.file)) new Image().src = fileURL(n.file);
  }
  const go = (d) => {
    const j = i + d;
    if (j < 0 || j >= items.length) return;
    i = j;
    show();
  };
  function close() {
    box.remove();
    document.removeEventListener("keydown", onKey, true);
  }
  function onKey(e) {
    if (e.key === "Escape") close();
    else if (e.key === "ArrowLeft") go(-1);
    else if (e.key === "ArrowRight") go(1);
    else return;
    e.preventDefault();
    e.stopPropagation();
  }

  prev.addEventListener("click", () => go(-1));
  next.addEventListener("click", () => go(1));
  closeBtn.addEventListener("click", close);
  // A click on the dark backdrop closes; clicks on the media don't.
  stage.addEventListener("click", (e) => e.target === stage && close());
  let x0 = null;
  stage.addEventListener("touchstart", (e) => (x0 = e.touches[0].clientX), { passive: true });
  stage.addEventListener("touchend", (e) => {
    if (x0 == null) return;
    const dx = e.changedTouches[0].clientX - x0;
    x0 = null;
    if (Math.abs(dx) > 50) go(dx < 0 ? 1 : -1);
  });
  document.addEventListener("keydown", onKey, true);
  document.body.append(box);
  show();

  // The list may grow once the chat's full file list arrives.
  if (!state.files.length) {
    getJSON(`/api/chats/${state.chat}/files`)
      .then((files) => {
        state.files = files;
        renderFiles();
        const current = items[i]?.id;
        items = galleryItems();
        i = Math.max(0, items.findIndex((m) => m.id === current));
        if (box.isConnected) show();
      })
      .catch(console.error);
  }
}

// ---------- Composer ----------

function autosize() {
  els.input.style.height = "auto";
  els.input.style.height = `${els.input.scrollHeight}px`;
}

els.input.addEventListener("input", autosize);
els.input.addEventListener("keydown", (e) => {
  // By default Enter sends on desktops and is a newline on touch keyboards.
  if (e.key === "Enter" && !e.shiftKey && prefs.enterSends && !e.isComposing) {
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

// ---------- Logo: the eye watches the pointer ----------

// The 16×16 icon with the eye left empty (W); the iris and pupil are drawn
// as a separate layer, clipped to the white of the eye, so they can move.
const LOGO = [
  ".......P........",
  ".......PP.......",
  "......PBP.......",
  "..P...PBBP...P..",
  "..PP.PBGBP..PP..",
  "..PBPPBGBP.PBP..",
  "..PBBPBGGBPPBP..",
  ".PBGBBGYYGBBGBP.",
  ".PBGYYRRRRYYGBP.",
  ".PBGRRWWWWRRGBP.",
  "PBGRWWWWWWWWRGBP",
  "PBGRWWWWWWWWRGBP",
  ".PBGRRWWWWRRGBP.",
  "..PBGYRRRRYGBP..",
  "...PBBGGGGBBP...",
  "....PPPPPPPP....",
];
const LOGO_COLORS = { P: "#7c6aa8", B: "#a8bfe6", G: "#b9de6e", Y: "#f4ea5a", R: "#a33a24", W: "#ffffff" };
const IRIS = [
  [6, 10, "#a8bfe6"], [9, 10, "#a8bfe6"], [6, 11, "#a8bfe6"], [9, 11, "#a8bfe6"],
  [7, 10, "#1a0804"], [8, 10, "#1a0804"], [7, 11, "#1a0804"], [8, 11, "#1a0804"],
];
const EYE = { x: 8, y: 11 }; // centre of the eye in grid units
const SVG_NS = "http://www.w3.org/2000/svg";
let logoSeq = 0;

function svgEl(tag, attrs) {
  const n = document.createElementNS(SVG_NS, tag);
  for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v);
  return n;
}

function pixelLogo(size, cls) {
  const id = `eye-clip-${++logoSeq}`;
  const svg = svgEl("svg", { viewBox: "0 0 16 16", width: size, height: size, "shape-rendering": "crispEdges", class: cls, "aria-hidden": "true" });
  const clip = svgEl("clipPath", { id });
  LOGO.forEach((row, y) => {
    for (const [x, ch] of [...row].entries()) {
      if (!LOGO_COLORS[ch]) continue;
      svg.append(svgEl("rect", { x, y, width: 1, height: 1, fill: LOGO_COLORS[ch] }));
      if (ch === "W") clip.append(svgEl("rect", { x, y, width: 1, height: 1 }));
    }
  });
  const iris = svgEl("g", { class: "iris" });
  for (const [x, y, fill] of IRIS) iris.append(svgEl("rect", { x, y, width: 1, height: 1, fill }));
  const clipped = svgEl("g", { "clip-path": `url(#${id})` });
  clipped.append(iris);
  svg.append(svgEl("defs", {}), clipped);
  svg.querySelector("defs").append(clip);
  return svg;
}

const watchers = [];
for (const img of document.querySelectorAll("img.logo")) {
  const svg = pixelLogo(img.getAttribute("width"), "logo");
  img.replaceWith(svg);
  watchers.push({ svg, iris: svg.querySelector(".iris"), dx: 0, dy: 0 });
}

let pointer = null;
let lookFrame = 0;
function look() {
  lookFrame = 0;
  for (const w of watchers) {
    let dx = 0;
    let dy = 0;
    const r = w.svg.getBoundingClientRect();
    if (pointer && r.width && prefs.anim && !reducedMotion) {
      const cx = r.left + (EYE.x / 16) * r.width;
      const cy = r.top + (EYE.y / 16) * r.height;
      const vx = pointer.x - cx;
      const vy = pointer.y - cy;
      const dist = Math.hypot(vx, vy);
      // Close to the eye it looks straight ahead; further away it turns
      // fully: up to 2 pixels sideways and 1 up or down.
      const reach = Math.min(1, dist / (r.width * 1.5));
      if (dist > r.width / 8) {
        dx = Math.max(-2, Math.min(2, Math.round((vx / dist) * 2.4 * reach)));
        dy = Math.max(-1, Math.min(1, Math.round((vy / dist) * 1.6 * reach)));
        // Looking up or down, the lids leave less room sideways.
        if (dy) dx = Math.max(-1, Math.min(1, dx));
      }
    }
    if (dx !== w.dx || dy !== w.dy) {
      w.dx = dx;
      w.dy = dy;
      w.iris.setAttribute("transform", `translate(${dx} ${dy})`);
    }
  }
}
const scheduleLook = () => (lookFrame ||= requestAnimationFrame(look));
document.addEventListener("pointermove", (e) => {
  pointer = { x: e.clientX, y: e.clientY };
  scheduleLook();
}, { passive: true });
document.documentElement.addEventListener("pointerleave", () => {
  pointer = null;
  scheduleLook();
});
window.addEventListener("blur", () => {
  pointer = null;
  scheduleLook();
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
