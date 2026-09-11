import { NextRequest, NextResponse } from "next/server";

const BACKEND = process.env.BACKEND_URL ?? "http://localhost:8081";

/**
 * Proxy POST /api/search → backend POST /search
 * Runs server-side, so no CORS headers needed on the backend.
 * TODO(security): Add authentication middleware before forwarding in production.
 */
export async function POST(req: NextRequest) {
  try {
    const body = await req.json();

    if (typeof body.query !== "string" || body.query.trim().length === 0) {
      return NextResponse.json({ error: "query is required" }, { status: 400 });
    }
    if (body.query.length > 4000) {
      return NextResponse.json({ error: "query too long (max 4000 chars)" }, { status: 400 });
    }

    const upstream = await fetch(`${BACKEND}/search`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: body.query,
        limit: typeof body.limit === "number" ? body.limit : 5,
      }),
    });

    const data = await upstream.json();
    return NextResponse.json(data, { status: upstream.status });
  } catch (err) {
    console.error("[/api/search] upstream error:", err instanceof Error ? err.message : err);
    return NextResponse.json({ error: "Failed to reach assistant backend" }, { status: 502 });
  }
}
