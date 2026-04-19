"use strict";

// ─── State ────────────────────────────────────────────────────────────────────
let inputData = null;       // Uint8Array — current input bytes
let compressedData = null;  // Uint8Array — last compressed result (with 4-byte size prefix)
let inputIsText = true;     // whether input is UTF-8 text (for decompress display)

// ─── Sample data ──────────────────────────────────────────────────────────────
const LOREM = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
  "Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. " +
  "Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi " +
  "ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit " +
  "in voluptate velit esse cillum dolore eu fugiat nulla pariatur. " +
  "Excepteur sint occaecat cupidatat non proident, sunt in culpa qui " +
  "officia deserunt mollit anim id est laborum. ";

const JSON_SAMPLE = JSON.stringify({
  server: { host: "api.example.com", port: 8443, tls: true, timeout: 30 },
  database: { host: "db.example.com", port: 5432, name: "appdb", pool: 10 },
  users: Array.from({ length: 25 }, (_, i) => ({
    id: i + 1,
    name: `User ${String(i + 1).padStart(3, "0")}`,
    email: `user${i + 1}@example.com`,
    role: ["admin", "user", "moderator"][i % 3],
    active: i % 5 !== 0,
    score: Math.round(Math.random() * 1000) / 10,
    tags: ["go", "wasm", "compression"].slice(0, (i % 3) + 1),
  })),
}, null, 2);

const REPETITIVE = "The quick brown fox jumps over the lazy dog. " .repeat(120);

function loadSample(name) {
  if (name === "random") {
    const arr = new Uint8Array(8192);
    crypto.getRandomValues(arr);
    inputData = arr;
    inputIsText = false;
    document.getElementById("input-text").value =
      `[Binary: ${arr.length} random bytes — not displayable as text]`;
  } else {
    const text = { lorem: LOREM.repeat(4), json: JSON_SAMPLE, repetitive: REPETITIVE }[name];
    const enc = new TextEncoder();
    inputData = enc.encode(text);
    inputIsText = true;
    document.getElementById("input-text").value = text;
  }
  compressedData = null;
  setDecompressEnabled(false);
  setStatus(`Loaded sample "${name}" — ${formatBytes(inputData.length)}`);
  hideResults();
}

// ─── File upload ──────────────────────────────────────────────────────────────
document.addEventListener("DOMContentLoaded", () => {
  document.getElementById("file-input").addEventListener("change", handleFileUpload);
  document.getElementById("input-text").addEventListener("input", handleTextInput);
});

function handleFileUpload(e) {
  const file = e.target.files[0];
  if (!file) return;
  document.getElementById("file-name").textContent = `${file.name} (${formatBytes(file.size)})`;
  const reader = new FileReader();
  reader.onload = (ev) => {
    inputData = new Uint8Array(ev.target.result);
    inputIsText = file.type.startsWith("text/");
    if (inputIsText) {
      document.getElementById("input-text").value = new TextDecoder().decode(inputData);
    } else {
      document.getElementById("input-text").value =
        `[Binary file: ${file.name}, ${formatBytes(inputData.length)}]`;
    }
    compressedData = null;
    setDecompressEnabled(false);
    setStatus(`Loaded file: ${file.name} (${formatBytes(inputData.length)})`);
    hideResults();
  };
  reader.readAsArrayBuffer(file);
}

function handleTextInput() {
  const text = document.getElementById("input-text").value;
  inputData = new TextEncoder().encode(text);
  inputIsText = true;
  compressedData = null;
  setDecompressEnabled(false);
}

// ─── Compress ─────────────────────────────────────────────────────────────────
function doCompress() {
  const text = document.getElementById("input-text").value;
  if (!inputData && text.length === 0) {
    setStatus("Nothing to compress — enter text or upload a file.");
    return;
  }
  if (inputIsText || !inputData) {
    inputData = new TextEncoder().encode(text);
  }
  if (inputData.length === 0) {
    setStatus("Input is empty.");
    return;
  }

  const level = parseInt(document.getElementById("level-select").value, 10);
  setStatus("Compressing…");
  document.getElementById("compress-btn").disabled = true;

  // Defer to let the status render before the blocking WASM call
  setTimeout(() => {
    try {
      const result = lz4Compress(inputData, level);
      if (result.error) {
        setStatus("Error: " + result.error);
        return;
      }
      compressedData = result.compressed;
      showResults(result);
      setDecompressEnabled(true);
      const levelName = document.getElementById("level-select").selectedOptions[0].text.split(" ")[0];
      setStatus(`Compressed with ${levelName} — ${formatBytes(result.originalSize)} → ${formatBytes(result.compressedSize)}`);
    } catch (err) {
      setStatus("Compression error: " + err.message);
    } finally {
      document.getElementById("compress-btn").disabled = false;
    }
  }, 10);
}

// ─── Decompress ───────────────────────────────────────────────────────────────
function doDecompress() {
  if (!compressedData) {
    setStatus("No compressed data — compress something first.");
    return;
  }
  setStatus("Decompressing…");

  setTimeout(() => {
    try {
      const result = lz4Decompress(compressedData);
      if (result.error) {
        setStatus("Error: " + result.error);
        return;
      }
      const decompressed = result.decompressed;
      if (inputIsText) {
        const text = new TextDecoder().decode(decompressed);
        document.getElementById("input-text").value = text;
        inputData = decompressed;
      } else {
        document.getElementById("input-text").value =
          `[Decompressed: ${formatBytes(decompressed.length)} of binary data]`;
        inputData = decompressed;
      }
      setStatus(`Decompressed in ${result.durationMs.toFixed(3)} ms — ${formatBytes(result.originalSize)}`);
    } catch (err) {
      setStatus("Decompression error: " + err.message);
    }
  }, 10);
}

// ─── Results display ──────────────────────────────────────────────────────────
function showResults(r) {
  document.getElementById("results-section").classList.remove("hidden");
  document.getElementById("stat-original").textContent = formatBytes(r.originalSize);
  document.getElementById("stat-compressed").textContent = formatBytes(r.compressedSize);

  const saved = (1 - r.ratio) * 100;
  document.getElementById("stat-saved").textContent = saved.toFixed(1) + "%";
  document.getElementById("stat-time").textContent = r.durationMs < 0.1
    ? "< 0.1 ms"
    : r.durationMs.toFixed(3) + " ms";

  // Ratio bar shows compressed size relative to original (smaller = better)
  document.getElementById("ratio-bar").style.width = (r.ratio * 100).toFixed(1) + "%";

  // Hex dump of first 128 bytes (skip the 4-byte size prefix)
  const bytes = r.compressed.slice(4, Math.min(r.compressed.length, 132));
  document.getElementById("hex-dump").textContent = hexDump(bytes);
}

function hideResults() {
  document.getElementById("results-section").classList.add("hidden");
}

function hexDump(bytes) {
  const lines = [];
  for (let i = 0; i < bytes.length; i += 16) {
    const chunk = bytes.slice(i, i + 16);
    const hex = Array.from(chunk).map(b => b.toString(16).padStart(2, "0"));
    const spaced = hex.map((h, j) => j === 7 ? h + "  " : h).join(" ");
    const offset = i.toString(16).padStart(4, "0");
    lines.push(`${offset}  ${spaced.padEnd(49)}`);
  }
  return lines.join("\n");
}

// ─── Benchmark ────────────────────────────────────────────────────────────────
function doBenchmark() {
  const text = document.getElementById("input-text").value;
  if (!inputData && text.length === 0) {
    setStatus("No input data — enter text or load a sample first.");
    return;
  }
  const data = inputData || new TextEncoder().encode(text);
  if (data.length < 64) {
    setStatus("Input too small for a meaningful benchmark (need at least 64 bytes).");
    return;
  }

  document.getElementById("bench-btn").disabled = true;
  document.getElementById("bench-progress").classList.remove("hidden");
  document.getElementById("bench-results").classList.add("hidden");
  document.getElementById("bench-progress-fill").style.width = "5%";
  setStatus(`Benchmarking ${formatBytes(data.length)} across 10 levels (~3 seconds)…`);

  setTimeout(() => {
    try {
      document.getElementById("bench-progress-fill").style.width = "50%";
      const json = lz4Benchmark(data);
      const results = JSON.parse(json);
      document.getElementById("bench-progress-fill").style.width = "100%";

      renderBenchTable(results);
      renderBenchChart(results);

      document.getElementById("bench-results").classList.remove("hidden");
      setStatus(`Benchmark complete — fastest compress: ${maxBy(results, "compressMBps").level} at ${maxBy(results, "compressMBps").compressMBps.toFixed(0)} MB/s`);
    } catch (err) {
      setStatus("Benchmark error: " + err.message);
    } finally {
      document.getElementById("bench-btn").disabled = false;
      document.getElementById("bench-progress").classList.add("hidden");
    }
  }, 20);
}

function renderBenchTable(results) {
  const tbody = document.getElementById("bench-tbody");
  tbody.innerHTML = "";
  results.forEach(r => {
    const tr = document.createElement("tr");
    const saved = ((1 - r.ratio) * 100).toFixed(1);
    const levelClass = r.level === "Fast" ? "level-fast" : "level-hc";
    tr.innerHTML = `
      <td><span class="${levelClass}">${r.level}</span></td>
      <td>${(r.ratio * 100).toFixed(1)}% <small style="color:var(--text2)">(${saved}% saved)</small></td>
      <td>${r.compressMBps.toFixed(0)}</td>
      <td>${r.decompressMBps.toFixed(0)}</td>
    `;
    tbody.appendChild(tr);
  });
}

function renderBenchChart(results) {
  const canvas = document.getElementById("bench-chart");
  // Use actual pixel size for crisp rendering
  const dpr = window.devicePixelRatio || 1;
  const cssW = canvas.parentElement.clientWidth || 700;
  const cssH = 300;
  canvas.width = Math.min(cssW, 700) * dpr;
  canvas.height = cssH * dpr;
  canvas.style.width = Math.min(cssW, 700) + "px";
  canvas.style.height = cssH + "px";

  const ctx = canvas.getContext("2d");
  ctx.scale(dpr, dpr);
  const W = canvas.width / dpr;
  const H = canvas.height / dpr;

  const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  const colors = {
    bg: isDark ? "#2c2c2e" : "#ffffff",
    grid: isDark ? "#3a3a3c" : "#e8e8ed",
    text: isDark ? "#98989d" : "#6e6e73",
    compress: "#0071e3",
    decompress: "#ff6b35",
  };

  ctx.fillStyle = colors.bg;
  ctx.fillRect(0, 0, W, H);

  const pad = { top: 44, right: 20, bottom: 52, left: 64 };
  const cW = W - pad.left - pad.right;
  const cH = H - pad.top - pad.bottom;

  // Scale: decompress speeds are much higher, so scale independently would hide compress bars.
  // Use a single scale but allow the Y axis to be annotated with MB/s.
  const allVals = results.flatMap(r => [r.compressMBps, r.decompressMBps]);
  const maxVal = Math.max(...allVals) * 1.1;

  // Grid lines
  const tickCount = 5;
  ctx.strokeStyle = colors.grid;
  ctx.lineWidth = 1;
  ctx.textAlign = "right";
  ctx.font = `${10 * dpr / dpr}px -apple-system, sans-serif`;
  ctx.fillStyle = colors.text;
  for (let i = 0; i <= tickCount; i++) {
    const y = pad.top + cH - (i / tickCount) * cH;
    const val = (maxVal * i / tickCount);
    const label = val >= 1000 ? (val / 1000).toFixed(1) + "k" : val.toFixed(0);
    ctx.fillText(label, pad.left - 6, y + 4);
    ctx.beginPath();
    ctx.moveTo(pad.left, y);
    ctx.lineTo(pad.left + cW, y);
    ctx.stroke();
  }

  // Y-axis label
  ctx.save();
  ctx.translate(14, pad.top + cH / 2);
  ctx.rotate(-Math.PI / 2);
  ctx.textAlign = "center";
  ctx.fillStyle = colors.text;
  ctx.font = `11px -apple-system, sans-serif`;
  ctx.fillText("MB/s", 0, 0);
  ctx.restore();

  // Axes
  ctx.strokeStyle = colors.grid;
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.moveTo(pad.left, pad.top);
  ctx.lineTo(pad.left, pad.top + cH);
  ctx.lineTo(pad.left + cW, pad.top + cH);
  ctx.stroke();

  // Bars
  const groupW = cW / results.length;
  const barW = groupW * 0.3;
  const gap = groupW * 0.06;

  results.forEach((r, i) => {
    const x0 = pad.left + i * groupW + gap;

    // Compress bar
    const ch = (r.compressMBps / maxVal) * cH;
    ctx.fillStyle = colors.compress;
    ctx.beginPath();
    ctx.roundRect(x0, pad.top + cH - ch, barW, ch, [3, 3, 0, 0]);
    ctx.fill();

    // Decompress bar
    const dh = (r.decompressMBps / maxVal) * cH;
    ctx.fillStyle = colors.decompress;
    ctx.beginPath();
    ctx.roundRect(x0 + barW + gap, pad.top + cH - dh, barW, dh, [3, 3, 0, 0]);
    ctx.fill();

    // X label
    ctx.fillStyle = colors.text;
    ctx.font = `11px -apple-system, sans-serif`;
    ctx.textAlign = "center";
    ctx.fillText(r.level, pad.left + i * groupW + groupW / 2, pad.top + cH + 18);

    // Ratio label below
    ctx.font = `9px -apple-system, sans-serif`;
    ctx.fillStyle = isDark ? "#5a5a60" : "#b0b0b8";
    ctx.fillText(`${(r.ratio * 100).toFixed(0)}%`, pad.left + i * groupW + groupW / 2, pad.top + cH + 32);
  });

  // Legend
  const legendX = pad.left;
  const legendY = 14;
  ctx.fillStyle = colors.compress;
  ctx.fillRect(legendX, legendY - 10, 14, 10);
  ctx.fillStyle = colors.text;
  ctx.font = `11px -apple-system, sans-serif`;
  ctx.textAlign = "left";
  ctx.fillText("Compress MB/s", legendX + 18, legendY);

  ctx.fillStyle = colors.decompress;
  ctx.fillRect(legendX + 140, legendY - 10, 14, 10);
  ctx.fillStyle = colors.text;
  ctx.fillText("Decompress MB/s", legendX + 158, legendY);
}

// ─── Helpers ──────────────────────────────────────────────────────────────────
function formatBytes(n) {
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
  return (n / (1024 * 1024)).toFixed(2) + " MB";
}

function setStatus(msg) {
  document.getElementById("status-bar").textContent = msg;
}

function setDecompressEnabled(enabled) {
  document.getElementById("decompress-btn").disabled = !enabled;
}

function maxBy(arr, key) {
  return arr.reduce((best, x) => x[key] > best[key] ? x : best, arr[0]);
}

// ─── WASM init ────────────────────────────────────────────────────────────────
const go = new Go();

async function initWasm() {
  try {
    const result = await WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject);
    go.run(result.instance).catch(err => setStatus("WASM runtime error: " + err.message));
    document.getElementById("loading").remove();
    setStatus("Ready — load a sample or type text above");
  } catch (err) {
    document.getElementById("loading").textContent = "Failed to load WebAssembly: " + err.message;
    console.error("WASM load error:", err);
  }
}

initWasm();
