# RAG Assistant Debugging Guide: "I don't know" Problem

## Problem Summary
Your assistant returns "I don't know" for every query despite documents being uploaded. This happens because:

1. **Documents are uploaded** ✓ (files saved to disk)
2. **But chunks aren't stored in the database** ✗ (no embeddings generated)
3. **So queries find no context to answer from** → "I don't know"

---

## Root Causes (Most → Least Common)

### 1. 🛑 **Gemini API Quota Exhausted** (MOST COMMON)
**Error**: `[Grove API] HTTP 500 — Failed to generate embeddings: Gemini API error: ... RESOURCE_EXHAUSTED ...`

**Why it happens:**
- Free tier Gemini API has monthly limits (~1 million tokens)
- Each document generates embeddings for all chunks
- Quota resets monthly or with paid plan

**Solution:**
- Upgrade to a paid Gemini API plan
- OR wait until next month for free tier reset
- Check your quota: https://ai.google.dev/rate-limits

### 2. 🔐 **Missing or Invalid API Key**
**Error**: `[Grove API] HTTP 500 — Failed to generate embeddings: Gemini API error: ... PERMISSION_DENIED ...`

**Solution:**
```bash
# Check if GEMINI_API_KEY is set in your environment
echo $GEMINI_API_KEY  # Should print your key

# Or check Docker logs
docker compose logs backend | grep "GEMINI_API_KEY"
```

### 3. 📦 **Database Connection Issue**
**Error**: `[Grove API] HTTP 500 — Failed to store document chunks: database error`

**Solution:**
```bash
# Check if database is healthy
docker compose ps  # postgres should show (healthy)
```

---

## New Debugging Tools (Just Added!)

### 1️⃣ **Check Database State**
```bash
curl http://localhost:8081/debug/state
```

**Response shows:**
```json
{
  "total_chunks": 0,
  "unique_files": 0,
  "documents": []
}
```

- `total_chunks: 0` = No documents stored yet
- `unique_files > 0` = Documents are stored
- If database seems empty but you uploaded files → Embedding generation failed

### 2️⃣ **Read Backend Logs**
```bash
# Watch logs in real-time as you upload/query
docker compose logs -f backend

# Look for these indicators:
# ✓ [UPLOAD] Created N chunks from PDF
# ✓ [UPLOAD] ✓ Generated N embeddings      ← Success
# ❌ [UPLOAD] ❌ Embedding generation failed ← Quota/key issue
# ✓ [UPLOAD] Stored N chunks, committing transaction
```

### 3️⃣ **Clear Database for Testing**
```bash
# Reset database (removes all uploaded documents)
curl -X POST http://localhost:8081/debug/clear

# Then re-upload a test document and watch logs
docker compose logs -f backend | grep "\[UPLOAD\]"
```

---

## Step-by-Step Diagnosis

### **Case 1: Check if Documents Are Stored**

```bash
# Step 1: Call debug endpoint
curl http://localhost:8081/debug/state

# Step 2: Interpret results
if total_chunks > 0:
    echo "✓ Documents ARE in database"
    echo "Problem: Query embedding generation failed"
    # → Check logs for [ASK] ❌ Embedding generation failed
else:
    echo "❌ Database is empty"
    echo "Problem: Upload embedding generation failed"
    # → Check logs for [UPLOAD] ❌ Embedding generation failed
fi
```

### **Case 2: Trace Upload Flow**

```bash
# 1. Clear database
curl -X POST http://localhost:8081/debug/clear

# 2. Watch logs
docker compose logs -f backend > /tmp/logs.txt &

# 3. Upload a small test PDF
# Use the web UI or API:
# curl -X POST -F "file=@test.pdf" http://localhost:8081/upload

# 4. Check logs for each stage:
grep "\[UPLOAD\]" /tmp/logs.txt

# Should see:
# [UPLOAD] Starting upload of: test.pdf
# [UPLOAD] Created N chunks
# [UPLOAD] Generating embeddings...
# [UPLOAD] ✓ Generated N embeddings        ← Key indicator
# [UPLOAD] Stored N chunks, committing...
```

### **Case 3: Test Query After Upload**

```bash
# After successful upload with embeddings:

# 1. Check database has chunks
curl http://localhost:8081/debug/state
# Should show: total_chunks > 0

# 2. Try a query through UI or API:
curl -X POST http://localhost:8081/ask \
  -H "Content-Type: application/json" \
  -d '{"question":"What is in the document?"}'

# 3. Check logs for query trace:
docker compose logs backend | grep "\[ASK\]"

# Should see:
# [ASK] Question: "What is..."
# [ASK] Generating embedding for question...
# [ASK] ✓ Generated embedding successfully
# [ASK] Searching database for 5 most relevant chunks...
# [ASK] Found N relevant chunks
```

---

## Quick Reference: What Each Error Means

| Error | Stage | Likely Cause | Fix |
|-------|-------|--------------|-----|
| `Embedding generation failed: RESOURCE_EXHAUSTED` | Upload or Ask | API quota hit | Upgrade plan or wait |
| `Embedding generation failed: PERMISSION_DENIED` | Upload or Ask | Bad API key | Check GEMINI_API_KEY |
| `Retrieval failed` | Ask | Database error | Check postgres is running |
| `No chunks found in database` | Ask | Empty database | Upload documents first |
| `Failed to commit chunks` | Upload | Database issue | Check postgres logs |

---

## Frontend Error Messages (Updated)

The UI now shows clearer errors:

### If quota exhausted:
```
🛑 AI API quota exhausted: Gemini API rate limit reached. 
Documents uploaded but cannot be queried until the quota resets 
(usually next month for free tier).
```

### If API key missing:
```
🔐 API Configuration Error: Gemini API key is missing or invalid. 
Check your GEMINI_API_KEY environment variable.
```

### If database error:
```
Database error: Failed to store document chunks. 
Please try uploading again.
```

---

## Code Changes Made

### Backend
- ✅ Added detailed logging to `upload.go` (stages: extraction, chunking, embedding, storage)
- ✅ Added detailed logging to `ask.go` (stages: embedding, retrieval, answer generation)
- ✅ Created `debug.go` with two new endpoints:
  - `GET /debug/state` - Shows database contents
  - `POST /debug/clear` - Clears all documents

### Frontend  
- ✅ Enhanced error classification in `lib/api.ts`
- ✅ Added "stage" field parsing (embeddings, database, etc.)
- ✅ Improved error messages to explain which stage failed

---

## Immediate Actions

1. **Build and Deploy**
   ```bash
   cd /home/saurabh/sideproject/assistant
   docker compose build --no-cache  # This rebuilds with new code
   docker compose down -v
   docker compose up -d
   ```

2. **Test Upload Flow**
   ```bash
   # Watch logs
   docker compose logs -f backend | grep "\["
   
   # Upload a test PDF through the UI
   # Look for [UPLOAD] messages showing progress
   ```

3. **Check Database**
   ```bash
   curl http://localhost:8081/debug/state
   # If total_chunks == 0 → embedding generation failed
   ```

4. **Test Query**
   ```bash
   # Ask a question through UI
   # Check logs for [ASK] messages
   docker compose logs backend | tail -50
   ```

---

## Common Scenarios & Solutions

### Scenario A: "I can upload documents but queries always fail"
```
→ Documents in DB (✓ embeddings worked)
→ But queries return "I don't know"
→ Likely: Query embedding generation failed (quota exhausted)
→ Check: docker compose logs | grep "[ASK] ❌"
```

### Scenario B: "Upload succeeds but database stays empty"
```
→ File accepted but no chunks stored
→ Likely: Upload embedding generation failed
→ Check: curl http://localhost:8081/debug/state
→ Check: docker compose logs | grep "[UPLOAD] ❌"
```

### Scenario C: "Everything fails immediately"
```
→ Likely: Database not running or API key not set
→ Check: docker compose ps (postgres should be healthy)
→ Check: docker compose logs | grep "ERROR\|WARNING"
```

---

## For Questions
- Check the backend logs first (most informative)
- Use `/debug/state` to validate database state
- Error messages in browser console show which stage failed
- Look for 🛑 ✓ ❌ ⚠️ symbols in logs to spot issues quickly
