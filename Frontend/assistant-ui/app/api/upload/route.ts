import { NextRequest, NextResponse } from "next/server";

const BACKEND = process.env.BACKEND_URL ?? "http://localhost:8081";

/** Max allowed upload size — must match the Go backend limit (50 MB) */
const MAX_BYTES = 50 * 1024 * 1024;

/**
 * Proxy POST /api/upload → backend POST /upload (multipart/form-data)
 * Streaming the body through avoids buffering the whole file in memory.
 * TODO(security): Add authentication middleware before forwarding in production.
 */
export async function POST(req: NextRequest) {
  try {
    const contentLength = Number(req.headers.get("content-length") ?? 0);
    if (contentLength > MAX_BYTES) {
      return NextResponse.json({ error: "File too large (max 50 MB)" }, { status: 413 });
    }

    // Stream the multipart body straight through to the backend
    const upstream = await fetch(`${BACKEND}/upload`, {
      method: "POST",
      headers: {
        // Forward content-type so the boundary token is preserved
        "content-type": req.headers.get("content-type") ?? "multipart/form-data",
      },
      // @ts-expect-error — duplex is required for streaming request bodies in Node 18+
      duplex: "half",
      body: req.body,
    });

    const data = await upstream.json();
    return NextResponse.json(data, { status: upstream.status });
  } catch (err) {
    console.error("[/api/upload] upstream error:", err instanceof Error ? err.message : err);
    return NextResponse.json({ error: "Failed to reach assistant backend" }, { status: 502 });
  }
}

// Disable Next.js body parsing — we stream the raw multipart body to the backend
export const config = {
  api: { bodyParser: false },
};
