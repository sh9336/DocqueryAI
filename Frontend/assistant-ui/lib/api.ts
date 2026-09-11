/**
 * API client for the DocQuery AI RAG backend.
 *
 * Error handling strategy:
 *  - Full technical details → console.error (for developers / debugging)
 *  - Short, human-readable message → thrown as Error (shown in the UI)
 *
 * The backend is a public, session-leased demo (max concurrent visitors,
 * per-session document isolation, per-session quotas) — every request
 * carries the session cookie cross-site, hence `credentials: "include"`
 * throughout.
 *
 * TODO(security): For production, consider a BFF proxy so the backend is not
 * directly reachable from the public internet.
 */

const BASE_URL =
  (process.env.NEXT_PUBLIC_API_URL ?? "https://docqueryai-yad7.onrender.com").replace(/\/$/, "");

export interface Source {
  id: number;
  filename: string;
  content: string;
  page_number: number;
  section: string;
  chunk_index: number;
}

/** Resolved once an /ask SSE stream's "done" event arrives — the answer
 *  text itself is delivered incrementally via askQuestionStream's onDelta
 *  callback instead of being included here. */
export interface AskStreamResult {
  sources: Source[];
  truncated: boolean;
}

export interface SearchResult {
  results: Source[];
}

/** Thrown by handleResponse/uploadPDF. `message` is already a friendly,
 *  UI-ready string; `code` is the backend's machine-readable error code
 *  (e.g. "demo_full", "session_expired") when present, for callers that
 *  need to branch on the specific condition rather than string-match. */
export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/* == Error classification ===========================================
   Takes the raw server payload + HTTP status and returns a short,
   readable message for the UI bubble. Logs full detail to console so
   developers can debug. `code` is the backend's machine-readable error
   code (e.g. "demo_full", "session_expired") when present — prefer it
   over string-sniffing rawMessage.
   =================================================================== */
function classifyError(status: number, rawMessage: string, stage?: string, code?: string): string {
  // Log the full message to console for developers
  console.error(`[DocQuery API] HTTP ${status} — ${rawMessage}${stage ? ` (stage: ${stage})` : ""}${code ? ` [${code}]` : ""}`);

  // Session-lease states — these are the backend's own codes, not string-sniffed
  if (code === "demo_full") {
    return "This demo is at capacity right now (too many visitors). Please try again in a minute or two.";
  }
  if (code === "ip_limit") {
    return "Only one active session per visitor is allowed.";
  }
  if (code === "session_required" || code === "session_expired") {
    return "Your session expired from inactivity. Reconnecting…";
  }

  // A request is already in flight for this session (upload/ask concurrency = 1)
  if (status === 409) {
    return "Please wait for the current request to finish before starting another.";
  }

  // Quota / rate-limit
  if (
    status === 429 ||
    rawMessage.includes("RESOURCE_EXHAUSTED") ||
    rawMessage.includes("quota") ||
    rawMessage.includes("Quota exceeded") ||
    /rate.?limit/i.test(rawMessage)
  ) {
    if (stage === "embeddings" || stage === "answer") {
      return "The AI service is busy right now (shared demo quota). Please wait a moment and try again.";
    }
    return rawMessage || "You've hit a usage limit for this demo session.";
  }

  // Auth / API key issues
  if (
    status === 401 ||
    status === 403 ||
    rawMessage.includes("API_KEY") ||
    rawMessage.includes("PERMISSION_DENIED") ||
    rawMessage.includes("authentication")
  ) {
    return "Authentication error. Please check the API configuration.";
  }

  // Backend / network unreachable
  if (status === 502 || status === 503 || status === 504) {
    return "The assistant backend is unreachable right now. Please try again in a moment.";
  }

  // Empty retrieval / no documents indexed
  if (
    rawMessage.toLowerCase().includes("no documents") ||
    rawMessage.toLowerCase().includes("no text") ||
    rawMessage.toLowerCase().includes("empty embedding")
  ) {
    return "No documents found to search. Try uploading a PDF first.";
  }

  // Database errors
  if (stage === "database" || rawMessage.toLowerCase().includes("database")) {
    return "Database error: Failed to store document chunks. Please try uploading again.";
  }

  // Bad request — the Go backend already writes specific, user-facing text
  // for these (file too large, too many pages, doc/question limits, etc.),
  // so pass it through rather than a generic fallback.
  if (status === 400 && rawMessage) {
    return rawMessage;
  }

  // Server-side processing error
  if (status >= 500) {
    return "Something went wrong on the server. Please try again.";
  }

  // Fallback — don't expose raw server text to the user
  return "An unexpected error occurred. Please try again.";
}

/* == Fetch helper - parses error body and throws a friendly message == */
async function handleResponse(res: Response): Promise<unknown> {
  if (res.ok) return res.json();

  // Try to parse the error body
  let rawMessage = `HTTP ${res.status}`;
  let stage: string | undefined;
  let code: string | undefined;
  try {
    const body = await res.json();
    rawMessage = body.error ?? JSON.stringify(body);
    stage = body.stage; // Extract stage info from backend (embeddings, database, etc.)
    code = body.code; // Machine-readable error code (demo_full, session_expired, etc.)
  } catch {
    // body isn't JSON — keep the status text
    rawMessage = res.statusText || rawMessage;
  }

  throw new ApiError(classifyError(res.status, rawMessage, stage, code), res.status, code);
}

/* == Session lease ===================================================
   The backend caps concurrent visitors and scopes documents per session.
   Call acquireSession() once on load, sendHeartbeat() every ~30s while
   the tab is open (server tells you the exact cadence), and
   releaseSession() on page unload.
   =================================================================== */

export interface SessionLimits {
  max_sessions: number;
  heartbeat_seconds: number;
  timeout_seconds: number;
  max_lifetime_seconds: number;
  max_documents: number;
  max_chunks: number;
  max_questions: number;
  max_file_size_mb: number;
  max_pages_per_doc: number;
  max_question_length: number;
}

/** POST /session — acquire a session lease (sets an HttpOnly cookie). Throws with code "demo_full" if at capacity. */
export async function acquireSession(): Promise<SessionLimits> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/session`, { method: "POST", credentials: "include" });
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /session:", networkErr);
    throw new Error("Can't reach the assistant. Is the backend running?");
  }
  return handleResponse(res) as Promise<SessionLimits>;
}

/** POST /session/heartbeat — keep the lease alive. Resolves false if the session has expired. */
export async function sendHeartbeat(): Promise<boolean> {
  try {
    const res = await fetch(`${BASE_URL}/session/heartbeat`, { method: "POST", credentials: "include" });
    return res.ok;
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /session/heartbeat:", networkErr);
    return false;
  }
}

/**
 * Releases the session lease on page unload via sendBeacon, which (unlike
 * fetch) is reliably delivered even as the page is being torn down. Cookies
 * attach automatically per normal browser rules — no credentials option to
 * set here.
 */
export function releaseSession(): void {
  if (typeof navigator === "undefined" || typeof navigator.sendBeacon !== "function") return;
  navigator.sendBeacon(`${BASE_URL}/session/release`);
}

export interface SessionStatus {
  documents_used: number;
  max_documents: number;
  chunks_used: number;
  max_chunks: number;
  questions_used: number;
  max_questions: number;
}

/** GET /session/status — current usage vs. limits, for quota UI. */
export async function getSessionStatus(): Promise<SessionStatus> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/session/status`, { method: "GET", credentials: "include" });
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /session/status:", networkErr);
    throw new Error("Can't reach the assistant. Is the backend running?");
  }
  return handleResponse(res) as Promise<SessionStatus>;
}

/* == Public API ======================================================= */

/**
 * POST /ask — RAG question answering, streamed as Server-Sent Events.
 * `onDelta` fires with each chunk of answer text as it's generated, so the
 * caller can render it live instead of waiting for the full answer.
 * Resolves once the stream's terminal "done" event arrives, with the
 * sources and whether the answer was cut short by the token limit.
 *
 * Pass `continueAnswer` (the prior truncated answer) to have the model
 * pick up where it left off rather than re-answering from scratch.
 *
 * Pre-stream failures (rate limit, validation, session state, etc.) still
 * arrive as a normal JSON error body with a non-2xx status, exactly like
 * every other endpoint here — only once the stream itself has started can
 * a failure only be reported as an "error" SSE event instead.
 */
export async function askQuestionStream(
  question: string,
  limit: number,
  continueAnswer: string | undefined,
  onDelta: (text: string) => void
): Promise<AskStreamResult> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/ask`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({
        question,
        limit,
        ...(continueAnswer ? { continue_answer: continueAnswer } : {}),
      }),
    });
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /ask:", networkErr);
    throw new Error("Can't reach the assistant. Is the backend running?");
  }

  if (!res.ok) {
    await handleResponse(res); // always throws a classified ApiError for a non-2xx status
    throw new Error("unreachable");
  }
  if (!res.body) {
    throw new Error("The assistant's response stream was empty.");
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let sources: Source[] = [];
  let truncated = false;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);

      const line = (frame.startsWith("data:") ? frame.slice(5) : frame).trim();
      if (!line) continue;

      let event: { type: string; text?: string; sources?: Source[]; truncated?: boolean; message?: string };
      try {
        event = JSON.parse(line);
      } catch {
        continue; // ignore malformed/keepalive frames
      }

      if (event.type === "delta" && event.text) {
        onDelta(event.text);
      } else if (event.type === "done") {
        sources = event.sources ?? [];
        truncated = !!event.truncated;
      } else if (event.type === "error") {
        console.error(`[DocQuery API] /ask stream error: ${event.message}`);
        throw new Error(event.message || "The assistant hit an error while answering.");
      }
    }
  }

  return { sources, truncated };
}

/** POST /search — semantic document search */
export async function searchDocuments(
  query: string,
  limit = 5
): Promise<SearchResult> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/search`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ query, limit }),
    });
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /search:", networkErr);
    throw new Error("Can't reach the assistant. Is the backend running?");
  }
  return handleResponse(res) as Promise<SearchResult>;
}

/** POST /upload — multipart PDF upload with progress tracking */
export async function uploadPDF(
  file: File,
  onProgress?: (pct: number) => void
): Promise<{ message: string; chunks: number }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const form = new FormData();
    form.append("file", file);

    xhr.withCredentials = true;

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.round((e.loaded / e.total) * 100));
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch {
          console.error("[DocQuery API] /upload: invalid JSON in success response");
          reject(new Error("Upload succeeded but the server response was unexpected."));
        }
        return;
      }

      // Error path — classify and return friendly message
      let rawMessage = `HTTP ${xhr.status}`;
      let stage: string | undefined;
      let code: string | undefined;
      try {
        const body = JSON.parse(xhr.responseText);
        rawMessage = body.error ?? JSON.stringify(body);
        stage = body.stage;
        code = body.code;
      } catch {
        rawMessage = xhr.statusText || rawMessage;
      }
      reject(new ApiError(classifyError(xhr.status, rawMessage, stage, code), xhr.status, code));
    };

    xhr.onerror = () => {
      console.error("[DocQuery API] /upload: network error (XHR)");
      reject(new Error("Can't reach the backend. Is the server running?"));
    };

    xhr.open("POST", `${BASE_URL}/upload`);
    xhr.send(form);
  });
}

export interface DocumentItem {
  filename: string;
  chunks: number;
  created_at: string;
}

/** GET /documents — list documents in the caller's session */
export async function listDocuments(): Promise<DocumentItem[]> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/documents`, {
      method: "GET",
      credentials: "include",
    });
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /documents:", networkErr);
    throw new Error("Can't reach the assistant. Is the backend running?");
  }
  return handleResponse(res) as Promise<DocumentItem[]>;
}

/** DELETE /documents/:filename — remove a document and its chunks (must belong to the caller's session) */
export async function deleteDocument(filename: string): Promise<{ message: string; filename: string; chunks: number }> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/documents/${encodeURIComponent(filename)}`, {
      method: "DELETE",
      credentials: "include",
    });
  } catch (networkErr) {
    console.error("[DocQuery API] Network error on /documents/:filename:", networkErr);
    throw new Error("Can't reach the assistant. Is the backend running?");
  }
  return handleResponse(res) as Promise<{ message: string; filename: string; chunks: number }>;
}
