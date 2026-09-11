"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { askQuestionStream, uploadPDF, listDocuments, deleteDocument, type Source } from "@/lib/api";
import SessionGate, { useSession } from "./session-gate";

/* == Types ========================================================== */
type ToastType = "success" | "error" | "info";

interface Message {
  id: string;
  role: "user" | "ai";
  content: string;
  sources?: Source[];
  isError?: boolean;
  hasNoRelevant?: boolean;
  truncated?: boolean;
  question?: string; // original question that produced this AI message — needed to continue it
  ts: Date;
}

interface Toast  { id: string; type: ToastType; message: string; }
interface Doc    { id: string; name: string; size: number; chunks: number; status: "indexed" | "indexing" | "error"; }

/* == Helpers ======================================================== */
const uid = () => Math.random().toString(36).slice(2, 10);
const fmtTime  = (d: Date) => d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

/* Fenced code block — its own <pre>, a language label, and a copy button.
   ReactMarkdown hands this the code element's className (e.g. "language-bash"),
   so we render it in place of the default <pre><code> pairing. */
function CodeBlock({ language, code }: { language: string; code: string }) {
  const [copied, setCopied] = useState(false);
  const onCopy = useCallback(() => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [code]);

  return (
    <div className="code-block">
      <div className="code-block-header">
        <span className="code-block-lang">{language || "text"}</span>
        <button
          type="button"
          className={`code-block-copy${copied ? " copied" : ""}`}
          onClick={onCopy}
          aria-label="Copy code"
        >
          {copied ? <><I.Check /> Copied</> : "Copy"}
        </button>
      </div>
      <pre><code>{code}</code></pre>
    </div>
  );
}

/* AI answers render through react-markdown (+ remark-gfm for tables/strikethrough).
   Headings, lists, links, tables, blockquotes and fenced code all get real
   semantic elements — styling is entirely CSS-driven (see .bubble-md rules),
   never dictated by the model. */
function AnswerMarkdown({ text }: { text: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        a: ({ href, children }) => (
          <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>
        ),
        pre: ({ children }) => <>{children}</>,
        code: ({ className, children }) => {
          const match = /language-(\w+)/.exec(className || "");
          const raw = String(children).replace(/\n$/, "");
          if (match) {
            return <CodeBlock language={match[1]} code={raw} />;
          }
          return <code className="inline-code">{raw}</code>;
        },
        table: ({ children }) => (
          <div className="table-scroll"><table>{children}</table></div>
        ),
      }}
    >
      {text}
    </ReactMarkdown>
  );
}

/* == Icons ========================================================== */
const I = {
  Lock: () => (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="11" width="18" height="11" rx="2"/>
      <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
    </svg>
  ),
  Bot: () => (
  <svg
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.8"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {/* Antenna */}
    <path d="M12 3v3" />
    <circle cx="12" cy="2.5" r="0.8" fill="currentColor" stroke="none" />

    {/* Head */}
    <rect x="4" y="6" width="16" height="13" rx="4" />

    {/* Eyes */}
    <circle cx="9" cy="12" r="1" fill="currentColor" stroke="none" />
    <circle cx="15" cy="12" r="1" fill="currentColor" stroke="none" />

    {/* Mouth */}
    <path d="M9 16h6" />

    {/* Side details */}
    <path d="M4 10H2.5M20 10h1.5" />
  </svg>
),
  Trash: () => (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="3 6 5 6 21 6"/>
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
      <path d="M10 11v6M14 11v6"/>
      <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/>
    </svg>
  ),
  Send: () => (
    <svg width="16" height="16" viewBox="0 0 20 20" fill="currentColor">
      <path d="M10.894 2.553a1 1 0 00-1.788 0l-7 14a1 1 0 001.169 1.409l5-1.429A1 1 0 009 15.571V11a1 1 0 112 0v4.571a1 1 0 00.725.962l5 1.428a1 1 0 001.17-1.408l-7-14z"/>
    </svg>
  ),
  File: () => (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
      <polyline points="14 2 14 8 20 8"/>
    </svg>
  ),
  Chevron: ({ open }: { open: boolean }) => (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"
      className={`sources-chevron${open ? " open" : ""}`}>
      <polyline points="6 9 12 15 18 9"/>
    </svg>
  ),
  Check: () => (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12"/>
    </svg>
  ),
  X: ({ size = 14 }: { size?: number }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
    </svg>
  ),
  Info: () => (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
    </svg>
  ),
  Upload: () => (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
      <polyline points="17 8 12 3 7 8"/>
      <line x1="12" y1="3" x2="12" y2="15"/>
    </svg>
  ),
  AlertCircle: () => (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10"/>
      <line x1="12" y1="8" x2="12" y2="12"/>
      <line x1="12" y1="16" x2="12.01" y2="16"/>
    </svg>
  ),
  RotateCcw: () => (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="1 4 1 10 7 10"/>
      <path d="M3.51 15a9 9 0 1 0 .49-3.36"/>
    </svg>
  ),
  Clock: () => (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
    </svg>
  ),
  MoreVertical: () => (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/>
    </svg>
  ),
  Menu: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="3" y1="6" x2="21" y2="6"/>
      <line x1="3" y1="12" x2="21" y2="12"/>
      <line x1="3" y1="18" x2="21" y2="18"/>
    </svg>
  ),
};

const SUGGESTIONS = [
  "Give me a concise summary of the uploaded documents.",
  "What are the main topics and themes covered?",
  "What does the document say about ",
  "Analyze and compare the main sections in detail.",
];

/* == Sources — flat list, click a row to reveal its excerpt =========== */
function SourcesAccordion({ sources }: { sources: Source[] }) {
  const [open, setOpen] = useState(false);
  const [expandedSource, setExpandedSource] = useState<number | null>(null);

  return (
    <div className="sources-container">
      <button className="sources-toggle" onClick={() => setOpen(p => !p)} aria-expanded={open}>
        Sources ({sources.length})
        <I.Chevron open={open} />
      </button>

      {open && (
        <div className="sources-list">
          {sources.map((s, i) => (
            <div
              key={i}
              className="source-item"
              onClick={() => setExpandedSource(expandedSource === i ? null : i)}
              role="button"
              tabIndex={0}
              aria-expanded={expandedSource === i}
            >
              <div className="source-header">
                <div className="source-info">
                  <span className="source-icon"><I.File /></span>
                  <span className="source-name">{s.filename}</span>
                </div>
                <span className="source-page">
                  {s.page_number > 0 ? `Page ${s.page_number}` : `Chunk ${s.chunk_index + 1}`}
                </span>
              </div>
              {expandedSource === i && (
                <div className="source-excerpt">
                  <div className="excerpt-text">{s.content}</div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/* == Toast stack ==================================================== */
function Toasts({ toasts, onDismiss }: { toasts: Toast[]; onDismiss: (id: string) => void }) {
  return (
    <div className="toast-stack" role="status" aria-live="polite">
      {toasts.map(t => (
        <div key={t.id} className={`toast ${t.type}`} role="alert">
          <span className="toast-icon">
            {t.type === "success" ? <I.Check /> : t.type === "error" ? <I.X /> : <I.Info />}
          </span>
          <span className="toast-text">{t.message}</span>
          <button className="toast-dismiss" onClick={() => onDismiss(t.id)} aria-label="Dismiss">
            <I.X size={11} />
          </button>
        </div>
      ))}
    </div>
  );
}

/* == Sidebar ======================================================== */
function Sidebar({
  onToast,
  docs,
  fetchDocs,
  handleDelete,
  isOpen,
  onClose,
}: {
  onToast: (type: ToastType, msg: string) => void;
  docs: Doc[];
  fetchDocs: () => Promise<void>;
  handleDelete: (filename: string) => Promise<void>;
  isOpen: boolean;
  onClose: () => void;
}) {
  const [dragging,  setDragging]  = useState(false);
  const [uploading, setUploading] = useState(false);
  const [progress,  setProgress]  = useState(0);
  const [fileName,  setFileName]  = useState("");
  const [menuOpenFor, setMenuOpenFor] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const { limits, status, refreshStatus } = useSession();

  const docLimitReached = !!status && status.documents_used >= limits.max_documents;

  const handleFile = useCallback(async (file: File) => {
    if (docLimitReached) {
      onToast("error", `Document limit reached (${limits.max_documents} max per session). Delete one to upload another.`);
      return;
    }
    if (!file.name.toLowerCase().endsWith(".pdf")) {
      onToast("error", "Only PDF files are supported."); return;
    }
    if (file.size > limits.max_file_size_mb * 1024 * 1024) {
      onToast("error", `File exceeds the ${limits.max_file_size_mb} MB limit.`); return;
    }
    setUploading(true); setFileName(file.name); setProgress(0);
    try {
      const res = await uploadPDF(file, setProgress);
      onToast("success", `"${file.name}" — ${res.chunks} chunks indexed.`);
      fetchDocs();
      refreshStatus();
    } catch (err) {
      onToast("error", err instanceof Error ? err.message : "Upload failed.");
    } finally {
      setUploading(false); setFileName(""); setProgress(0);
    }
  }, [onToast, fetchDocs, docLimitReached, limits, refreshStatus]);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault(); setDragging(false);
    const f = e.dataTransfer.files[0]; if (f) handleFile(f);
  }, [handleFile]);

  // Close the row overflow menu on any outside click
  useEffect(() => {
    if (!menuOpenFor) return;
    const close = () => setMenuOpenFor(null);
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [menuOpenFor]);

  return (
    <aside className={`sidebar${isOpen ? " mobile-open" : ""}`} aria-label="Document Library">
      <div className="sidebar-header">
        <div className="sidebar-logo">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/product_logo.png" alt="" />
        </div>
        <span>DocQuery AI</span>
        <button className="sidebar-close-btn" aria-label="Close knowledge base" onClick={onClose}>
          <I.X size={16} />
        </button>
      </div>
      <div className="sidebar-content">
        <span className="sidebar-section-label">Knowledge Base</span>

        {/* Upload */}
        <div
          className={`drop-zone${dragging ? " over" : ""}${docLimitReached ? " disabled" : ""}`}
          onDragOver={e => { if (!docLimitReached) { e.preventDefault(); setDragging(true); } }}
          onDragLeave={() => setDragging(false)}
          onDrop={docLimitReached ? undefined : onDrop}
          onClick={() => !docLimitReached && inputRef.current?.click()}
          role="button"
          tabIndex={docLimitReached ? -1 : 0}
          aria-disabled={docLimitReached}
          aria-label="Upload PDF"
          onKeyDown={e => !docLimitReached && e.key === "Enter" && inputRef.current?.click()}
        >
          <I.Upload />
          {docLimitReached ? "Document limit reached" : "Upload document"}
          <input
            ref={inputRef}
            type="file"
            accept="application/pdf"
            id="pdf-upload-input"
            aria-label="Select PDF"
            style={{ display: "none" }}
            disabled={docLimitReached}
            onChange={e => { const f = e.target.files?.[0]; if (f) handleFile(f); e.target.value = ""; }}
          />
        </div>
        <p className="upload-hint">
          {docLimitReached ? "Delete a document to upload another" : `PDF up to ${limits.max_file_size_mb} MB`}
        </p>

        {/* Progress */}
        {uploading && (
          <div className="progress-wrap">
            <div className="progress-label">
              <span className="progress-filename">{fileName}</span>
              <span className="progress-pct">{progress}%</span>
            </div>
            <div className="progress-track">
              <div className="progress-fill" style={{ width: `${progress}%` }} />
            </div>
          </div>
        )}

        {/* Indexed files */}
        <div className="sidebar-section-row">
          <span className="sidebar-section-label">Documents</span>
          {status && (
            <span className={`doc-count-badge${docLimitReached ? " full" : ""}`}>
              {status.documents_used}/{limits.max_documents}
            </span>
          )}
        </div>
        {docs.length > 0 && (
          <div className="indexed-list">
            {docs.map(d => (
              <div key={d.id} className={`indexed-item indexed-${d.status}`}>
                <span className="indexed-item-icon"><I.File /></span>
                <div className="indexed-item-content">
                  <div className="indexed-item-name" title={d.name}>{d.name}</div>
                  <div className="indexed-item-meta">
                    {d.status === "indexed" && (
                      <>
                        <I.Check />
                        <span className="indexed-item-status">Ready · {d.chunks} chunks</span>
                      </>
                    )}
                    {d.status === "indexing" && (
                      <>
                        <I.Clock />
                        <span className="indexed-item-status">Indexing...</span>
                      </>
                    )}
                    {d.status === "error" && (
                      <>
                        <I.AlertCircle />
                        <span className="indexed-item-status">Error</span>
                      </>
                    )}
                  </div>
                </div>
                <button
                  className="indexed-item-menu-btn"
                  aria-label={`Options for ${d.name}`}
                  onClick={e => { e.stopPropagation(); setMenuOpenFor(menuOpenFor === d.id ? null : d.id); }}
                >
                  <I.MoreVertical />
                </button>
                {menuOpenFor === d.id && (
                  <div className="indexed-item-menu" onClick={e => e.stopPropagation()}>
                    <button onClick={() => { setMenuOpenFor(null); handleDelete(d.name); }}>
                      Delete
                    </button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </aside>
  );
}


/* == Main page ====================================================== */
function AppShell() {
  const [messages,    setMessages]    = useState<Message[]>([]);
  const [input,       setInput]       = useState("");
  const [loading,     setLoading]     = useState(false);
  const [toasts,      setToasts]      = useState<Toast[]>([]);
  const [docs,        setDocs]        = useState<Doc[]>([]);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [continuingId, setContinuingId] = useState<string | null>(null);
  const { limits, status, refreshStatus } = useSession();
  const questionLimitReached = !!status && status.questions_used >= limits.max_questions;

  const bottomRef   = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  /* Scroll to bottom */
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, loading]);

  /* Auto-resize textarea */
  useEffect(() => {
    const el = textareaRef.current; if (!el) return;
    el.style.height = "auto";
    const h = Math.min(el.scrollHeight, 150);
    el.style.height = `${h}px`;
    el.style.overflowY = el.scrollHeight > 150 ? "scroll" : "hidden";
  }, [input]);

  /* Toast helpers */
  const pushToast = useCallback((type: ToastType, message: string) => {
    const id = uid();
    setToasts(p => [...p, { id, type, message }]);
    setTimeout(() => setToasts(p => p.filter(t => t.id !== id)), 5000);
  }, []);
  const dismissToast = useCallback((id: string) => setToasts(p => p.filter(t => t.id !== id)), []);

  /* Docs helpers */
  const fetchDocs = useCallback(async () => {
    try {
      const list = await listDocuments();
      setDocs(list.map(item => ({
        id: item.filename,
        name: item.filename,
        size: 0,
        chunks: item.chunks,
        status: "indexed" as const,
      })));
    } catch (err) {
      pushToast("error", err instanceof Error ? err.message : "Failed to load document list.");
    }
  }, [pushToast]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchDocs();
  }, [fetchDocs]);

  const handleDelete = useCallback(async (filename: string) => {
    try {
      await deleteDocument(filename);
      setDocs(p => p.filter(x => x.name !== filename));
      pushToast("success", `"${filename}" deleted successfully.`);
      refreshStatus();
    } catch (err) {
      pushToast("error", err instanceof Error ? err.message : "Failed to delete document.");
    }
  }, [pushToast, refreshStatus]);

  /* Send — streams the answer in as it's generated. The AI bubble doesn't
     appear until the first chunk of text arrives (the typing indicator
     covers retrieval + time-to-first-token), then grows in place. */
  const send = useCallback(async () => {
    const text = input.trim(); if (!text || loading || questionLimitReached) return;
    const userMsg: Message = { id: uid(), role: "user", content: text, ts: new Date() };
    setMessages(p => [...p, userMsg]);
    setInput(""); setLoading(true);

    const aiId = uid();
    // NOTE: deliberately no external "started" flag here. React Strict Mode
    // (on by default in Next.js dev) double-invokes setState updater
    // functions to catch impure ones — a plain closure variable mutated
    // inside the updater gets out of sync between the two invocations and
    // silently drops the update with no error. Deriving "has this message
    // started" from the state array itself keeps the updater pure and safe
    // to call twice.
    const exists = (p: Message[]) => p.some(m => m.id === aiId);

    try {
      const result = await askQuestionStream(text, 5, undefined, (delta) => {
        setMessages(p => exists(p)
          ? p.map(m => m.id === aiId ? { ...m, content: m.content + delta } : m)
          : [...p, { id: aiId, role: "ai", content: delta, question: text, ts: new Date() }]);
      });
      setMessages(p => exists(p)
        ? p.map(m => m.id === aiId ? { ...m, sources: result.sources, truncated: result.truncated } : m)
        // Stream completed with no delta events at all (empty model output) — still surface something.
        : [...p, { id: aiId, role: "ai", content: "No answer was generated for that question.", ts: new Date(), isError: true }]);
    } catch (err) {
      // classifyError() in api.ts already logged full details to console.
      // Show only the friendly message in the chat bubble — no extra toast.
      const friendly = err instanceof Error ? err.message : "Something went wrong.";
      setMessages(p => exists(p)
        ? p.map(m => m.id === aiId ? { ...m, content: friendly, isError: true } : m)
        : [...p, { id: aiId, role: "ai", content: friendly, ts: new Date(), isError: true }]);
    } finally {
      setLoading(false);
      refreshStatus();
    }
  }, [input, loading, questionLimitReached, refreshStatus]);

  /* Continue a truncated answer — resends the original question plus the
     partial answer so the model picks up exactly where it left off, then
     streams the continuation directly onto the same bubble (no separator:
     it's a continuation of the same sentence, not a new paragraph). */
  const continueAnswer = useCallback(async (msg: Message) => {
    if (loading || questionLimitReached || !msg.question) return;
    setLoading(true); setContinuingId(msg.id);
    try {
      const result = await askQuestionStream(msg.question, 5, msg.content, (delta) => {
        setMessages(p => p.map(m => m.id === msg.id ? { ...m, content: m.content + delta } : m));
      });
      setMessages(p => p.map(m => m.id === msg.id ? {
        ...m,
        sources: result.sources.length ? result.sources : m.sources,
        truncated: result.truncated,
      } : m));
    } catch (err) {
      const friendly = err instanceof Error ? err.message : "Something went wrong.";
      pushToast("error", friendly);
    } finally {
      setLoading(false); setContinuingId(null);
      refreshStatus();
    }
  }, [loading, questionLimitReached, refreshStatus, pushToast]);

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); }
  };

  const applySuggestion = (text: string) => {
    setInput(text);
    setTimeout(() => { textareaRef.current?.focus(); }, 0);
  };

  return (
    <div className="app-container">
      <div
        className={`mobile-backdrop${sidebarOpen ? " visible" : ""}`}
        onClick={() => setSidebarOpen(false)}
        aria-hidden={!sidebarOpen}
      />

      {/* Sidebar Knowledge Base */}
      <Sidebar
        docs={docs}
        fetchDocs={fetchDocs}
        handleDelete={handleDelete}
        onToast={pushToast}
        isOpen={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
      />

      <div className="layout">
        {/* == Header ======================================= */}
        <header className="chat-header" role="banner">
          <div className="header-brand">
            <button
              type="button"
              className="mobile-menu-btn"
              aria-label="Open knowledge base"
              onClick={() => setSidebarOpen(true)}
            >
              <I.Menu />
            </button>
            <div className="header-titles">
              <h1 className="header-title">DocQuery AI</h1>
              <span className="header-subtitle">Ask anything about your documents</span>
            </div>
          </div>
          <div className="header-actions">
            <div className={`header-status${loading ? " busy" : ""}`}>
              <span className="live-dot" />
              {loading ? "Answering…" : "Ready"}
            </div>
            <button
              id="clear-chat-btn"
              className="header-btn"
              title="Clear conversation"
              aria-label="Clear conversation"
              onClick={() => setMessages([])}
              disabled={messages.length === 0}
              style={messages.length === 0 ? { opacity: 0.4, cursor: "default" } : undefined}
            >
              <I.Trash />
            </button>
          </div>
        </header>

        {/* == Chat history ================================== */}
        <main
          className="chat-history"
          id="chat-history"
          aria-live="polite"
          aria-label="Conversation"
        >
          {messages.length === 0 && !loading ? (
            /* Empty state */
            <div className="empty-state">
              <div className="empty-copy">
                <h1 className="empty-title">Ask your documents anything</h1>
                <p className="empty-sub">
                  Upload a PDF to build a knowledge base, then ask questions to get answers grounded in its content — with sources cited.
                </p>
              </div>
              <div className="suggestion-grid" role="list" aria-label="Suggested prompts">
                {SUGGESTIONS.map((text, i) => (
                  <button
                    key={i}
                    id={`suggestion-${i}`}
                    className="suggestion-pill"
                    role="listitem"
                    onClick={() => applySuggestion(text)}
                    aria-label={`Try: ${text}`}
                  >
                    <span className="suggestion-pill-text">{text}</span>
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <>
              {messages.map(msg =>
                msg.role === "user" ? (
                  /* User bubble */
                  <div key={msg.id} className="msg-user" role="article" aria-label={`You at ${fmtTime(msg.ts)}`}>
                    <div className="bubble-user">{msg.content}</div>
                    <span className="bubble-timestamp-row">
                      <span className="bubble-timestamp">{fmtTime(msg.ts)}</span>
                      <I.Check />
                    </span>
                  </div>
                ) : (
                  /* AI bubble */
                  <div key={msg.id} className="msg-ai" role="article" aria-label={`${msg.isError ? "Error" : "AI"} at ${fmtTime(msg.ts)}`}>
                    <div className={`ai-avatar${msg.isError ? " ai-avatar-err" : ""}`} aria-label="Assistant bot" aria-hidden={false}>
                      {msg.isError ? <I.AlertCircle /> : <I.Bot />}
                    </div>
                    <div style={{ display: "flex", flexDirection: "column", gap: 6, minWidth: 0, flex: 1 }}>
                      {msg.isError ? (
                        <div className="bubble-error-card">
                          <div className="bubble-error-header">
                            <I.AlertCircle />
                            <span className="bubble-error-title">Couldn&apos;t complete request</span>
                          </div>
                          <p className="bubble-error-body">{msg.content}</p>
                          <button
                            className="bubble-error-retry"
                            onClick={() => {
                              // Put the last user message back in the input for retry
                              const lastUser = [...messages].reverse().find(m => m.role === "user");
                              if (lastUser) setInput(lastUser.content);
                            }}
                            aria-label="Retry last question"
                          >
                            <I.RotateCcw /> Try again
                          </button>
                        </div>
                      ) : (
                        <div className="bubble-ai">
                          <div className="bubble-md">
                            <AnswerMarkdown text={msg.content} />
                            {loading && msg.id === messages[messages.length - 1]?.id && (
                              <span className="streaming-cursor" aria-hidden />
                            )}
                          </div>
                          {msg.truncated && (
                            <div className="truncated-note">
                              <span>Cut short by the response length limit.</span>
                              <button
                                type="button"
                                className="truncated-continue-btn"
                                onClick={() => continueAnswer(msg)}
                                disabled={loading || questionLimitReached}
                                aria-label="Continue this answer"
                              >
                                {continuingId === msg.id ? "Continuing…" : "Continue"}
                              </button>
                            </div>
                          )}
                          {msg.hasNoRelevant && (
                            <div className="no-relevant-info">
                              <I.AlertCircle />
                              <span>No relevant information found in your documents.</span>
                            </div>
                          )}
                          {msg.sources && msg.sources.length > 0 && (
                            <SourcesAccordion sources={msg.sources} />
                          )}
                        </div>
                      )}
                      <span className="bubble-timestamp" style={{ paddingLeft: 4 }}>{fmtTime(msg.ts)}</span>
                    </div>
                  </div>
                )
              )}

              {/* Typing indicator — shown only before the first chunk of a new
                  answer arrives (covers retrieval + time-to-first-token).
                  Once the AI bubble exists it's already growing live, so this
                  hides itself rather than doubling up as a second indicator;
                  it also stays hidden during a Continue, since that appends
                  onto an existing "ai" bubble rather than starting a new one. */}
              {loading && (messages.length === 0 || messages[messages.length - 1].role !== "ai") && (
                <div className="typing-row" role="status" aria-label="AI is thinking">
                  <div className="ai-avatar" aria-label="Assistant bot" aria-hidden={false}><I.Bot /></div>
                  <div className="typing-dots">
                    <div className="tdot" /><div className="tdot" /><div className="tdot" />
                  </div>
                </div>
              )}
            </>
          )}
          <div ref={bottomRef} aria-hidden />
        </main>

        {/* == Input footer ================================== */}
        <footer className="input-footer" aria-label="Message input">
          <div className="input-shell">
            <div className="input-box">
              <div className="input-row">
                <textarea
                  ref={textareaRef}
                  id="chat-input"
                  className="chat-textarea"
                  placeholder={questionLimitReached ? "Question limit reached for this session" : "Ask anything about your documents..."}
                  value={input}
                  rows={1}
                  disabled={loading || questionLimitReached}
                  aria-label="Type your message"
                  aria-multiline="true"
                  maxLength={limits.max_question_length}
                  onChange={e => setInput(e.target.value)}
                  onKeyDown={onKeyDown}
                />

                <button
                  id="send-btn"
                  className="send-btn"
                  disabled={!input.trim() || loading || questionLimitReached}
                  aria-label="Send message"
                  title="Send (Enter)"
                  onClick={send}
                >
                  <I.Send />
                </button>
              </div>
            </div>

            <p className={`input-caption${questionLimitReached ? " warn" : ""}`}>
              {questionLimitReached
                ? `You've reached the ${limits.max_questions}-question limit for this session.`
                : status
                  ? `Answers are generated from your uploaded documents · ${status.questions_used}/${limits.max_questions} questions`
                  : "Answers are generated from your uploaded documents."}
            </p>
          </div>
        </footer>
      </div>

      {/* Toasts */}
      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}

export default function Home() {
  return (
    <SessionGate>
      <AppShell />
    </SessionGate>
  );
}
