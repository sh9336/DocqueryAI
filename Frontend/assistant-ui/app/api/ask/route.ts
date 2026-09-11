import { NextRequest, NextResponse } from "next/server";

const BACKEND = process.env.BACKEND_URL ?? "http://localhost:8081";

/**
 * Proxy POST /api/ask → backend POST /ask
 * Runs server-side, so no CORS headers needed on the backend.
 * TODO(security): Add authentication middleware before forwarding in production.
 */
export async function POST(req: NextRequest) {
  try {
    const body = await req.json();

    // Input validation — question must be a non-empty string
    if (typeof body.question !== "string" || body.question.trim().length === 0) {
      return NextResponse.json({ error: "question is required" }, { status: 400 });
    }
    if (body.question.length > 4000) {
      return NextResponse.json({ error: "question too long (max 4000 chars)" }, { status: 400 });
    }

    const upstream = await fetch(`${BACKEND}/ask`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        question: body.question,
        limit: typeof body.limit === "number" ? body.limit : 5,
      }),
    });

    const data = await upstream.json();
    return NextResponse.json(data, { status: upstream.status });
  } catch (err) {
    // Log server-side only — generic message to client
    console.error("[/api/ask] upstream error:", err instanceof Error ? err.message : err);
    return NextResponse.json({ error: "Failed to reach assistant backend" }, { status: 502 });
  }
}
