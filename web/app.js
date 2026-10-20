// pneuma web client. No framework, no build step.

const $ = (id) => document.getElementById(id);
const els = {
  log: $("log"),
  messages: $("messages"),
  uploads: $("uploads"),
  empty: $("empty"),
  loader: $("loader"),
  input: $("input"),
  composer: $("composer"),
  attach: $("attach"),
  fileInput: $("file-input"),
  drop: $("drop"),
  toast: $("toast"),
  me: $("me"),
  meName: $("me-name"),
  status: $("status"),
};

// ---------- i18n ----------

const STRINGS = {
  en: {
    placeholder: "Message, paste or drop files…",
    empty: "Type, paste or drop anything. It shows up on all your devices.",
    drop: "Drop to send",
    copied: "Copied",
    copyFailed: "Couldn't copy",
    copy: "Copy",
    copyImage: "Copy image",
    download: "Download",
    delete: "Delete",
    confirmDelete: "Delete?",
    today: "Today",
    yesterday: "Yesterday",
    online: "Connected",
    offline: "Reconnecting…",
    tooLarge: (max) => `Too large (max ${max})`,
    uploadFailed: "Upload failed",
    sendFailed: "Couldn't send",
    you: "you",
  },
  es: {
    placeholder: "Escribe, pega o suelta archivos…",
    empty: "Escribe, pega o arrastra lo que sea. Aparecerá en todos tus equipos.",
    drop: "Suelta para enviar",
    copied: "Copiado",
    copyFailed: "No se pudo copiar",
    copy: "Copiar",
    copyImage: "Copiar imagen",
    download: "Descargar",
    delete: "Borrar",
    confirmDelete: "¿Borrar?",
    today: "Hoy",
    yesterday: "Ayer",
    online: "Conectado",
    offline: "Reconectando…",
    tooLarge: (max) => `Demasiado grande (máx. ${max})`,
    uploadFailed: "Error al subir",
    sendFailed: "No se pudo enviar",
    you: "tú",
  },
};
const lang = navigator.language?.toLowerCase().startsWith("es") ? "es" : "en";
const t = STRINGS[lang];
document.documentElement.lang = lang;
els.input.placeholder = t.placeholder;
$("empty-text").textContent = t.empty;
$("drop-text").textContent = t.drop;

// ---------- API ----------

// In -dev mode, ?as=<device> lets two tabs pose as two machines.
const AS = new URLSearchParams(location.search).get("as");
function api(path) {
  if (!AS) return path;
  return path + (path.includes("?") ? "&" : "?") + "as=" + encodeURIComponent(AS);
}

async function getJSON(path) {
  const res = await fetch(api(path));
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
  return res.json();
}

// ---------- State ----------

const PAGE = 50;
const state = {
  me: null,
  maxUpload: 0,
  msgs: new Map(), // id -> message
  nodes: new Map(), // id -> element
  hasMore: true,
  loadingOlder: false,
  stick: true, // keep the view pinned to the bottom
};

// ---------- Formatting ----------

const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const dateFmt = new Intl.DateTimeFormat(undefined, { weekday: "long", day: "numeric", month: "long" });

function dayLabel(d) {
  const today = new Date();
  const yesterday = new Date();
  yesterday.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return t.today;
  if (d.toDateString() === yesterday.toDateString()) return t.yesterday;
  return dateFmt.format(d);
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

function deviceColor(name) {
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return `hsl(${h % 360} 55% 55%)`;
}

const ICONS = {
  phone: '<svg viewBox="0 0 24 24" width="13" height="13"><rect x="7" y="2.5" width="10" height="19" rx="2.5" fill="none" stroke="currentColor" stroke-width="2"/></svg>',
  computer: '<svg viewBox="0 0 24 24" width="13" height="13"><rect x="3" y="4" width="18" height="12" rx="2" fill="none" stroke="currentColor" stroke-width="2"/><path d="M8 20h8" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>',
};
const deviceIcon = (os) => (os === "android" || os === "ios" ? ICONS.phone : ICONS.computer);

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === "class") node.className = v;
    else if (k === "html") node.innerHTML = v; // only ever used with our own constants
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

// ---------- Rendering ----------

function renderMessage(m) {
  const mine = state.me && m.device === state.me.device;
  const when = new Date(m.created_at);

  const head = el(
    "div",
    { class: "head" },
    el("span", { class: "dot", style: `background:${deviceColor(m.device)}` }),
    el("span", { class: "device" }, m.device),
    mine ? el("span", {}, `(${t.you})`) : null,
    el("span", { html: deviceIcon(m.os) }),
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
        { class: "bubble filebox file", href: fileURL(f, true), download: f.name },
        el("span", { class: "ficon" }, ext),
        el("span", { class: "meta" }, el("span", { class: "name", title: f.name }, f.name), el("span", { class: "size" }, formatSize(f.size))),
      );
    }
    actions.append(el("a", { href: fileURL(f, true), download: f.name }, t.download));
  }

  actions.append(deleteButton(m), el("span", { class: "time", title: when.toLocaleString() }, timeFmt.format(when)));

  return el("div", { class: `msg${mine ? " mine" : ""}`, "data-id": m.id }, head, bubble, actions);
}

function deleteButton(m) {
  let timer;
  const btn = el("button", { type: "button" }, t.delete);
  btn.addEventListener("click", async () => {
    if (!btn.classList.contains("confirm")) {
      btn.classList.add("confirm");
      btn.textContent = t.confirmDelete;
      timer = setTimeout(() => {
        btn.classList.remove("confirm");
        btn.textContent = t.delete;
      }, 3000);
      return;
    }
    clearTimeout(timer);
    const res = await fetch(api(`/api/messages/${m.id}`), { method: "DELETE" });
    if (res.ok || res.status === 404) removeMessage(m.id);
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
    if (!state.msgs.has(m.id)) {
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

// ---------- Loading ----------

// Fetches the newest page and reconciles it with what we have. Called on
// start and after every reconnect, so missed messages and deletions heal.
async function syncLatest() {
  const latest = await getJSON(`/api/messages?limit=${PAGE}`);
  const ids = latest.map((m) => m.id);
  const known = [...state.msgs.keys()];
  const newestKnown = known.length ? Math.max(...known) : 0;
  const gap = latest.length === PAGE && ids[0] > newestKnown;
  if (gap || !known.length) {
    state.msgs.clear();
    state.nodes.clear();
    state.hasMore = latest.length === PAGE;
  } else if (ids.length) {
    const keep = new Set(ids);
    for (const id of known) {
      if (id >= ids[0] && !keep.has(id)) {
        state.msgs.delete(id);
        state.nodes.delete(id);
      }
    }
  } else {
    state.msgs.clear();
    state.nodes.clear();
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
  try {
    const oldest = Math.min(...state.msgs.keys());
    const older = await getJSON(`/api/messages?before=${oldest}&limit=${PAGE}`);
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

// ---------- Live updates ----------

function connect() {
  const es = new EventSource(api("/api/events"));
  es.addEventListener("open", () => {
    setOnline(true);
    syncLatest().catch(console.error);
  });
  es.addEventListener("error", () => setOnline(false));
  es.addEventListener("message", (e) => {
    const m = JSON.parse(e.data);
    const pinned = nearBottom() || (state.me && m.device === state.me.device);
    if (addMessages([m])) {
      render();
      if (pinned) scrollToBottom();
    }
  });
  es.addEventListener("delete", (e) => removeMessage(JSON.parse(e.data).id));
}

function setOnline(on) {
  els.status.className = `status ${on ? "online" : "offline"}`;
  els.status.title = on ? t.online : t.offline;
}

// ---------- Sending ----------

async function sendText(text) {
  if (!text.trim()) return false;
  try {
    const res = await fetch(api("/api/messages"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    });
    if (!res.ok) throw new Error(await res.text());
    const m = await res.json();
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
    uploadQueue = uploadQueue.then(() => uploadOne(file, item, bar));
  }
}

function uploadOne(file, item, bar) {
  return new Promise((resolve) => {
    const form = new FormData();
    form.append("file", file, file.name);
    const xhr = new XMLHttpRequest();
    xhr.open("POST", api("/api/files"));
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
  setTimeout(() => bubble.classList.remove("copied"), 900);
}

let toastTimer;
function toast(text, error = false) {
  els.toast.textContent = text;
  els.toast.className = `toast${error ? " error" : ""}`;
  els.toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (els.toast.hidden = true), error ? 3500 : 1400);
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
  els.input.style.height = `${els.input.scrollHeight + 2}px`;
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

// Ctrl+V anywhere: files and screenshots upload, text lands in the composer.
document.addEventListener("paste", (e) => {
  const files = [...(e.clipboardData?.files ?? [])];
  if (files.length) {
    e.preventDefault();
    uploadFiles(files.map(nameClipboardFile));
    return;
  }
  if (e.target !== els.input) {
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
  if (e.target === els.input || e.ctrlKey || e.metaKey || e.altKey || e.key.length !== 1) return;
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

// ---------- Boot ----------

async function boot() {
  try {
    const me = await getJSON("/api/me");
    state.me = me.sender;
    state.maxUpload = me.max_upload;
    els.meName.textContent = state.me.device;
    els.me.hidden = false;
  } catch (err) {
    els.empty.hidden = false;
    $("empty-text").textContent = String(err.message);
    return;
  }
  connect();
  if (!coarse) els.input.focus();
  if ("serviceWorker" in navigator && !AS) navigator.serviceWorker.register("/sw.js").catch(console.error);
}

boot();
