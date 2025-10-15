# WCE vs Firebase: Architecture Comparison

This document compares WCE (our Starlark/SQLite implementation) to Firebase, examining key design choices, trade-offs, and philosophical differences.

## Core Philosophy Comparison

### Firebase Philosophy
- **Backend-as-a-Service (BaaS)**: Centralized cloud platform
- **Real-time first**: Live data synchronization across clients
- **Client-side heavy**: Most logic runs in the browser (JavaScript)
- **Opinionated scaling**: Firebase handles infrastructure, you follow their patterns
- **Document-oriented**: NoSQL (Firestore) or real-time database
- **Proprietary platform**: Tied to Google's infrastructure

### WCE Philosophy
- **Database-is-the-Application**: Each cenv is a portable SQLite file
- **Server-side first**: Logic runs in sandboxed Starlark on the server
- **HTMX interactions**: Hypermedia-driven, not JavaScript-heavy SPAs
- **Self-hosted**: Single binary, runs anywhere
- **SQL-native**: Relational database with transactions and constraints
- **True portability**: Copy the .db file, you have the entire application

---

## Key Architectural Differences

### 1. Data Model

**Firebase (Firestore/Realtime DB):**
```javascript
// Document-oriented, nested JSON
{
  users: {
    user123: {
      name: "Alice",
      posts: {
        post1: { title: "Hello", timestamp: ... }
      }
    }
  }
}
```

**WCE (SQLite):**
```sql
-- Relational, normalized tables
CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE posts (
  id TEXT PRIMARY KEY,
  user_id TEXT REFERENCES users(id),
  title TEXT,
  timestamp INTEGER
);
```

**Trade-offs:**

| Aspect | Firebase NoSQL | WCE SQLite |
|--------|----------------|------------|
| **Querying** | Limited (no JOINs, shallow queries) | Full SQL with JOINs, CTEs, window functions |
| **Transactions** | Limited cross-document | Full ACID transactions |
| **Consistency** | Eventually consistent | Strongly consistent (SERIALIZABLE) |
| **Denormalization** | Required for performance | Optional, can normalize |
| **Schema** | Schemaless (flexible but risky) | Schema-enforced (safe but rigid) |
| **Data Integrity** | Application-level constraints | Database-level foreign keys, constraints |

**WCE Design Choice Rationale:**
- **Relational model** enables complex queries without denormalization
- **Transactions** ensure data integrity (critical for multi-user scenarios)
- **Foreign keys** prevent orphaned data and enforce referential integrity
- **SQL** is a 50+ year old standard, widely understood

---

### 2. Runtime & Scripting

**Firebase (Cloud Functions - JavaScript/TypeScript):**
```javascript
// Runs on Google's Node.js servers
exports.onUserCreate = functions.firestore
  .document('users/{userId}')
  .onCreate((snap, context) => {
    // Can access npm packages, full Node.js APIs
    const mailgun = require('mailgun-js');
    return mailgun.send(welcomeEmail);
  });
```

**WCE (Starlark - Sandboxed Python-like):**
```python
# Runs in Go-embedded Starlark interpreter
def handle_user_create(request):
    # NO filesystem, NO network, NO arbitrary packages
    # Only whitelisted APIs: db.query(), db.execute()
    user_id = request.params["user_id"]
    db.execute("INSERT INTO audit_log ...", [user_id])
    return {"status": "ok"}
```

**Trade-offs:**

| Aspect | Firebase (JavaScript) | WCE (Starlark) |
|--------|----------------------|----------------|
| **Language Features** | Full ES6+, async/await, npm ecosystem | Simple, deterministic, no I/O |
| **Security** | Relies on Cloud Functions sandbox | Sandboxed interpreter, no escape |
| **Performance** | Cold starts (ms to seconds) | In-process, instant execution |
| **External Services** | Can call any API (HTTP, email, etc.) | Isolated, can only access cenv DB |
| **Dependencies** | npm packages (security risk) | No external dependencies allowed |
| **Learning Curve** | JavaScript developers ready | Learn Starlark (Python-like, simpler) |
| **Debugging** | Console logs, Cloud debugger | Standard output, structured errors |

**WCE Design Choice Rationale:**
- **Starlark is deterministic**: Same inputs = same outputs (great for testing)
- **No I/O** means no accidental data leaks (can't call external APIs)
- **Sandboxing** prevents privilege escalation (can't read /etc/passwd)
- **In-process** execution is faster than cold-starting containers
- **Simplicity**: Starlark is intentionally limited (no async, threads, etc.)

**Limitation Acknowledged:**
- Cannot send emails, call external APIs, or perform I/O **by design**
- For these use cases, future enhancement could add **whitelisted functions** (e.g., `http.post()` with rate limiting)

---

### 3. Client Interaction Model

**Firebase (JavaScript SDK):**
```javascript
// Real-time listener in browser
const unsubscribe = firestore
  .collection('messages')
  .where('room', '==', 'general')
  .onSnapshot((snapshot) => {
    snapshot.docChanges().forEach((change) => {
      if (change.type === 'added') {
        renderMessage(change.doc.data());
      }
    });
  });
```

**WCE (HTMX + Server-Sent Events):**
```html
<!-- HTMX-driven interactions -->
<div hx-get="/123-uuid/messages?room=general"
     hx-trigger="every 2s"
     hx-swap="innerHTML">
  <!-- Server renders HTML, HTMX swaps it in -->
</div>

<!-- OR Server-Sent Events for real-time -->
<div hx-ext="sse" sse-connect="/123-uuid/stream?room=general">
  <div sse-swap="message" hx-swap="beforeend"></div>
</div>
```

**Trade-offs:**

| Aspect | Firebase (JS SDK) | WCE (HTMX) |
|--------|-------------------|------------|
| **Interactivity** | Rich client-side SPA | Hypermedia-driven, server-rendered |
| **Real-time** | WebSockets, auto-sync | Server-Sent Events or polling |
| **Offline Support** | Built-in offline persistence | Not built-in (would need service workers) |
| **JavaScript Bundle** | Large (Firebase SDK ~300KB+) | Minimal (HTMX ~14KB) |
| **Complexity** | State management, React/Vue/etc. | Simple HTML templates |
| **SEO** | Requires SSR or pre-rendering | Native server-rendered HTML |
| **Developer Experience** | Modern SPA tooling | Simpler, fewer build steps |

**WCE Design Choice Rationale:**
- **HTMX** reduces JavaScript complexity (no React/Vue needed)
- **Server-rendered HTML** means logic stays on the server (easier to secure)
- **Smaller bundle size** means faster load times
- **Hypermedia approach** aligns with original web architecture (REST)
- **SSE** provides real-time updates without full WebSocket complexity

**Limitation Acknowledged:**
- Rich client-side interactions (drag-drop, real-time collaboration) require more work
- Offline-first apps harder to build without client-side state management
- Could enhance with: **WebSocket support** for bidirectional real-time, **service worker** for offline

---

### 4. Event System

**Firebase (Triggers):**
```javascript
// Event-driven, distributed triggers
exports.onWrite = functions.firestore
  .document('posts/{postId}')
  .onWrite((change, context) => {
    // Runs on separate infrastructure
    // Can trigger multiple cloud functions
  });
```

**WCE (SQLite Commit Hooks):**
```go
// SQLite commit hook → Go callback
db.RegisterCommitHook(func(tx *sql.Tx) {
    // Callback runs after successful commit
    // Triggers template re-rendering
    // Updates page cache
    // All in-process, same transaction boundary
})
```

**Trade-offs:**

| Aspect | Firebase Triggers | WCE Commit Hooks |
|--------|------------------|------------------|
| **Event Source** | Firestore document changes | SQLite transaction commits |
| **Execution Context** | Separate Cloud Function instances | In-process Go callback |
| **Latency** | Network hop, cold start penalty | Immediate, no network |
| **Scalability** | Auto-scales to millions of events | Limited to single-process throughput |
| **Ordering** | No guarantees across documents | Sequential per-cenv (same process) |
| **Failure Handling** | Retries, dead-letter queues | Log error, continue (doesn't fail commit) |

**WCE Design Choice Rationale:**
- **Commit hooks** keep events simple: DB change → callback → re-render
- **In-process** means no distributed system complexity
- **Transactional boundary** clear: hook fires after commit succeeds
- **Render on write** pattern: pre-cache pages when data changes, not when requested

**Use Case Fit:**
- **WCE**: Small-to-medium apps where single-server performance is enough
- **Firebase**: Large-scale apps requiring distributed event processing

---

### 5. Multi-Tenancy & Isolation

**Firebase:**
```javascript
// Soft multi-tenancy via security rules
service cloud.firestore {
  match /databases/{database}/documents {
    match /tenants/{tenantId}/{document=**} {
      allow read, write: if request.auth.uid == resource.data.ownerId;
    }
  }
}
```
- All tenants share same Firestore database
- Isolation via security rules (application-level)
- Risk: Misconfigured rule = data leak across tenants

**WCE:**
```
./cenvs/tenant-a-uuid.db  (completely separate file)
./cenvs/tenant-b-uuid.db  (different process/memory space)
```
- Each cenv is a **separate SQLite database file**
- OS-level file permissions (600 - owner only)
- **Physical isolation**: No shared tables, no cross-cenv queries possible

**Trade-offs:**

| Aspect | Firebase (Shared DB) | WCE (Separate DBs) |
|--------|---------------------|-------------------|
| **Isolation** | Logical (security rules) | Physical (separate files) |
| **Security** | One bug = all tenants at risk | Breach limited to one cenv |
| **Backup** | All-or-nothing (entire Firestore) | Per-cenv (copy individual .db file) |
| **Portability** | Locked to Firebase platform | Copy .db file = copy entire app |
| **Cross-Tenant Queries** | Possible (analytics, aggregation) | Impossible by design |
| **Scaling** | Firebase auto-scales | One process per server (scale horizontally) |

**WCE Design Choice Rationale:**
- **Physical isolation** is strongest security guarantee
- **Portable cenvs**: User can download their .db file and self-host
- **Independent backup/restore**: Each cenv managed separately
- **Compliance**: Easier GDPR (delete one file = delete all user data)

**Trade-off Acknowledged:**
- Cannot aggregate across cenvs (by design)
- No global admin dashboard without external tooling
- Each cenv has overhead (separate DB file, connection pool)

---

### 6. Permissions & Authorization

**Firebase (Security Rules):**
```javascript
// Declarative rules, checked at query time
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    match /posts/{postId} {
      allow read: if request.auth != null;
      allow write: if request.auth.uid == resource.data.authorId;
    }
  }
}
```

**WCE (SQL Query Rewriting):**
```sql
-- User submits:
SELECT * FROM posts;

-- WCE rewrites to:
SELECT * FROM posts
WHERE id IN (
  SELECT id FROM posts
  WHERE author_id = $user_id  -- from _wce_row_policies
);
```

**Trade-offs:**

| Aspect | Firebase Rules | WCE Query Rewriting |
|--------|----------------|---------------------|
| **Granularity** | Document-level | Table-level + row-level |
| **Enforcement** | Client SDK + server validation | Server-side only (queries intercepted) |
| **Complexity** | Declarative (easier to understand) | Imperative (SQL WHERE clauses) |
| **Testing** | Firebase emulator + unit tests | SQL queries with test data |
| **Performance** | Indexed queries | Rewritten queries (may need indexes) |
| **Flexibility** | Limited (no arbitrary logic) | Full SQL expressions (e.g., `created_at > NOW() - 7 days`) |

**WCE Design Choice Rationale:**
- **Query rewriting** enforces permissions at DB level (can't bypass)
- **Row-level security** supports complex policies (time-based, role-based, data-based)
- **SQL expressions** more powerful than Firebase's rule language
- **Server-side only**: Client never sees raw data (no client-side filtering)

**Example WCE row policy:**
```sql
-- Only allow reading your own posts OR public posts
INSERT INTO _wce_row_policies (table_name, policy_type, sql_condition)
VALUES ('posts', 'read', 'author_id = $user_id OR visibility = "public"');
```

---

### 7. Deployment & Operations

**Firebase:**
- **Deployment**: `firebase deploy`
- **Hosting**: Google Cloud Platform (only)
- **Scaling**: Automatic, managed by Google
- **Cost**: Pay-per-usage (reads, writes, storage, bandwidth)
- **Monitoring**: Firebase Console, Google Cloud Monitoring
- **Backups**: Managed (daily exports to Google Cloud Storage)

**WCE:**
- **Deployment**: Single binary (`./wce`), systemd, Docker, or Kubernetes
- **Hosting**: Any Linux server, VPS, or cloud provider
- **Scaling**: Horizontal (load balancer → multiple WCE instances)
- **Cost**: Server costs only (fixed, predictable)
- **Monitoring**: Standard tools (Prometheus, Grafana, logs)
- **Backups**: Copy `.db` files (rsync, S3, etc.)

**Trade-offs:**

| Aspect | Firebase | WCE |
|--------|----------|-----|
| **Time to Deploy** | Minutes (cloud-based) | Minutes (binary + systemd) |
| **Operational Complexity** | Low (fully managed) | Medium (self-managed) |
| **Vendor Lock-in** | High (proprietary platform) | None (standard SQLite + HTTP) |
| **Cost Predictability** | Variable (usage-based) | Fixed (server cost) |
| **Data Sovereignty** | Google's data centers | Your infrastructure |
| **Customization** | Limited to Firebase features | Full control (edit source code) |
| **Performance Tuning** | Limited (trust Firebase) | Full (tune SQLite, indexing, etc.) |

**WCE Design Choice Rationale:**
- **Single binary** deployment is simple (no dependencies)
- **Self-hosted** gives full control over data and infrastructure
- **SQLite** is embedded (no separate database server to manage)
- **Portable** between environments (dev laptop → prod server, same binary)

**Cost Example:**
- Firebase: 50k writes/day = ~$1.50/month (but scales unpredictably)
- WCE: $5/month VPS (unlimited writes, predictable)

---

### 8. Real-Time Data & Subscriptions

**Firebase (Built-in Real-Time):**
```javascript
// Automatic WebSocket connection, live updates
db.collection('chat')
  .orderBy('timestamp')
  .limit(50)
  .onSnapshot((snapshot) => {
    snapshot.docChanges().forEach((change) => {
      // Automatic push to all connected clients
    });
  });
```

**WCE (Server-Sent Events via Commit Hooks):**
```html
<!-- SSE connection to cenv -->
<div hx-ext="sse" sse-connect="/uuid/events">
  <div sse-swap="chat-message" hx-swap="beforeend"></div>
</div>
```

```python
# Starlark: Commit hook triggers SSE push
def on_commit(event):
    # New chat message inserted
    # Trigger SSE event to all connected clients
    sse.broadcast("chat-message", render_message(event.data))
```

**Trade-offs:**

| Aspect | Firebase Real-Time | WCE SSE |
|--------|-------------------|---------|
| **Protocol** | WebSockets (bidirectional) | Server-Sent Events (unidirectional) |
| **Client Libraries** | Firebase SDK handles everything | Native browser `EventSource` or HTMX |
| **Offline Handling** | Built-in queue, sync when online | Not built-in (would need custom logic) |
| **Server Load** | Managed by Firebase (auto-scales) | Open connections = memory usage |
| **Latency** | Low (persistent connection) | Low (persistent connection) |
| **Fallback** | Long polling if WebSocket unavailable | Native browser support (no fallback needed) |

**WCE Design Choice Rationale:**
- **SSE** is simpler than WebSockets (server → client only)
- **Commit hooks** naturally trigger events (DB change → broadcast)
- **HTMX** handles SSE reconnection automatically
- **Stateless** server (clients reconnect if server restarts)

**When Firebase Wins:**
- Bidirectional real-time (e.g., collaborative editing, multiplayer games)
- Complex offline sync (Firebase handles conflict resolution)

**When WCE Wins:**
- Simple notifications (new message, status update)
- Server-driven updates (no need for client to send data back)

---

### 9. Template Rendering & Pages

**Firebase (Client-Side Rendering):**
```javascript
// JavaScript renders HTML in browser
fetch('/api/posts')
  .then(res => res.json())
  .then(posts => {
    const html = posts.map(p =>
      `<div class="post">${p.title}</div>`
    ).join('');
    document.getElementById('posts').innerHTML = html;
  });
```

**WCE (Server-Side Rendering with Caching):**
```go
// Go template rendered on commit, cached in SQLite
db.RegisterCommitHook(func() {
    // Detect which pages affected by this commit
    // Re-render templates to HTML
    // Store in _wce_page_cache table
})

// On request: serve from cache
func handlePage(w http.ResponseWriter, r *http.Request) {
    html := cache.Get(path) // Already rendered
    w.Write(html)
}
```

**Trade-offs:**

| Aspect | Firebase (CSR) | WCE (SSR + Cache) |
|--------|----------------|-------------------|
| **Rendering Location** | Client (browser) | Server (Go templates) |
| **Initial Load** | Slow (fetch data, then render) | Fast (HTML already ready) |
| **SEO** | Poor (requires SSR setup) | Excellent (real HTML) |
| **Caching** | Client-side (browser cache) | Server-side (SQLite cache table) |
| **Interactivity** | Rich (JavaScript frameworks) | Simpler (HTMX, progressive enhancement) |
| **Cache Invalidation** | Client must refetch | Automatic (commit hook re-renders) |

**WCE Design Choice Rationale:**
- **Render on write, not on read**: Pages pre-rendered when data changes
- **Commit hooks** trigger re-rendering (event-driven)
- **SQLite cache** stores rendered HTML (fast retrieval)
- **Server-side** means no JavaScript required for basic functionality

**The "Render on Write" Pattern:**
```
1. User updates data → INSERT/UPDATE SQL
2. Transaction commits → Commit hook fires
3. Hook detects affected templates → Re-renders to HTML
4. HTML stored in _wce_page_cache table
5. Next request → Serve cached HTML (microseconds)
```

**Why This Matters:**
- Read-heavy apps (most web apps) serve pre-rendered content instantly
- Write operations slightly slower (re-render cost), but amortized across many reads
- Scales better: rendering once vs. rendering on every request

---

### 10. Testing & Development

**Firebase:**
```javascript
// Local emulator suite
import { initializeTestEnvironment } from '@firebase/rules-unit-testing';

const testEnv = await initializeTestEnvironment({
  projectId: "test-project",
  firestore: { rules: fs.readFileSync("firestore.rules", "utf8") }
});

const alice = testEnv.authenticatedContext("alice");
await assertSucceeds(alice.firestore().collection("posts").add({...}));
```

**WCE:**
```go
// Standard Go testing with in-memory SQLite
func TestPostCreation(t *testing.T) {
    manager := cenv.NewManager(t.TempDir())
    cenvID := "test-uuid"
    manager.Create(cenvID)

    db, _ := manager.Open(cenvID)
    defer db.Close()

    // Test with real SQLite (:memory: mode)
    _, err := db.Exec("INSERT INTO posts ...")
    if err != nil {
        t.Fatalf("Failed: %v", err)
    }
}
```

**Trade-offs:**

| Aspect | Firebase | WCE |
|--------|----------|-----|
| **Test Environment** | Firebase emulator suite | Standard Go `testing` package |
| **Setup Complexity** | Install emulator, configure | Create temp dir, open DB |
| **Test Speed** | Slower (emulator overhead) | Fast (in-process SQLite) |
| **Isolation** | Emulator instance per test suite | `:memory:` DB per test |
| **Mocking** | Mock Firebase SDK | No mocking needed (real SQLite) |
| **CI/CD** | Requires emulator in pipeline | Standard Go test, no special setup |

**WCE Design Choice Rationale:**
- **Standard library testing**: No special frameworks or assertion libraries
- **In-memory SQLite**: Each test gets clean database (`t.TempDir()`)
- **Fast tests**: No network, no emulators, just Go and SQLite
- **Real behavior**: Tests use actual SQLite, not mocks

---

## Summary: Pros & Cons

### Firebase Strengths
✅ Fully managed (no ops required)
✅ Auto-scaling (handles millions of users)
✅ Real-time built-in (WebSockets, offline sync)
✅ Large ecosystem (extensions, integrations)
✅ Fast prototyping (deploy in minutes)
✅ Rich client SDKs (JS, iOS, Android)

### Firebase Weaknesses
❌ Vendor lock-in (proprietary platform)
❌ Cost unpredictability (usage-based pricing)
❌ Limited querying (no JOINs, complex queries)
❌ NoSQL constraints (denormalization required)
❌ Data sovereignty issues (Google's servers)
❌ Security rules complexity (easy to misconfigure)

---

### WCE Strengths
✅ True portability (copy .db file = copy app)
✅ Physical isolation (separate database per cenv)
✅ Full SQL power (JOINs, transactions, constraints)
✅ Predictable costs (fixed server costs)
✅ Data sovereignty (self-hosted)
✅ Simple deployment (single binary)
✅ No vendor lock-in (standard SQLite + HTTP)
✅ Minimal JavaScript (HTMX-driven, not SPA)
✅ Strong consistency (ACID transactions)
✅ Fast testing (standard Go tests, no emulator)

### WCE Weaknesses
❌ Single-server scalability limits
❌ Manual operations (self-managed)
❌ No built-in offline sync
❌ Starlark learning curve
❌ Limited external integrations (sandboxed)
❌ Real-time requires SSE implementation
❌ No native mobile SDKs (HTTP API only)
❌ Horizontal scaling more complex (need load balancer)

---

## Ideal Use Cases

### Choose Firebase When:
- Building consumer apps with millions of users
- Need real-time collaboration (Google Docs-style)
- Want zero operational overhead
- Mobile-first with offline support required
- Rapid prototyping, iterate quickly
- Team lacks infrastructure expertise

### Choose WCE When:
- Building B2B SaaS with isolated tenants
- Need true data portability (export = .db file)
- Want full control over infrastructure
- Prefer SQL and relational data modeling
- Require strong consistency guarantees
- Cost predictability matters
- Data sovereignty/compliance requirements (GDPR, HIPAA)
- Small-to-medium scale (thousands, not millions)
- Prefer server-side rendering over SPAs

---

## Hybrid Approach: What Could Be Added to WCE

To close the gap with Firebase, WCE could add:

1. **WebSocket Support**: Full bidirectional real-time (beyond SSE)
2. **Offline Sync**: Service worker + IndexedDB for offline-first
3. **Managed Hosting**: Optional WCE Cloud (like Supabase for Postgres)
4. **Mobile SDKs**: Native iOS/Android libraries for HTTP API
5. **External Function Calls**: Whitelisted HTTP APIs with rate limiting
6. **Background Jobs**: Cron-like scheduling within cenvs
7. **File Storage**: Blob storage per-cenv (or integrate with S3)
8. **Analytics**: Built-in query logs and performance metrics
9. **Global Admin Dashboard**: Cross-cenv management (optional)

---

## Philosophical Differences

| Dimension | Firebase | WCE |
|-----------|----------|-----|
| **Data Ownership** | Google owns infrastructure | You own the .db file |
| **Scaling Philosophy** | Scale to infinity (at cost) | Scale appropriately (bounded) |
| **Complexity** | Hide complexity (magic) | Explicit simplicity (transparent) |
| **Client/Server** | Client-heavy (SPAs) | Server-heavy (hypermedia) |
| **Dependencies** | npm ecosystem (risky) | No external deps (secure) |
| **Lock-in** | Platform lock-in | No lock-in (open formats) |
| **Cost Model** | Usage-based (variable) | Infrastructure-based (fixed) |

---

## Conclusion

**Firebase is a Ferrari**: Fast, powerful, handles massive scale, but you're renting it from Google and fueling it with usage-based pricing.

**WCE is a Honda Civic you own**: Reliable, efficient, you have the keys, and you know exactly what it costs to run. It won't win a drag race against Firebase at 10M users, but for 99% of apps, it's the right tool.

**Core WCE Bet:**
> Most web apps don't need Firebase-scale infrastructure. They need simple, portable, self-hosted solutions with strong guarantees (SQL, ACID, isolation). By choosing constraints (Starlark sandboxing, SQLite storage, HTMX interactivity), WCE trades ultimate scale for simplicity, portability, and control.

Firebase optimizes for **"what if we go viral?"**
WCE optimizes for **"what if we need to own our data?"**

Both are valid. Choose based on your needs.
