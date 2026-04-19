# Sample Tracker — UI QA Report
**Date:** 2026-04-19  
**Tested by:** Claude (source-code audit — browser extension unavailable during session)  
**App:** http://localhost:8010  
**Credentials used:** colnio / 0803200119

---

## Summary

| Severity | Count |
|----------|-------|
| 🔴 Bug (broken behaviour) | 4 |
| 🟠 Performance / bloat | 3 |
| 🟡 Inconsistency / UX issue | 5 |
| ℹ️ Minor / cosmetic | 3 |

---

## 🔴 Bugs — Broken Behaviour

### BUG-1 · Login page always opens in dark mode (ignores system light-mode preference)
**File:** `templates/auth_base.html` lines 13 and 77-78  
**What happens:** The theme-init script has a typo — both branches of the ternary return `'dark'`:
```js
// Line 13 — WRONG:
const initialTheme = storedTheme || (prefersDark ? 'dark' : 'dark');

// Line 77-78 — WRONG:
const fallback = 'dark';
const prefers = prefersDark && prefersDark.matches ? 'dark' : fallback; // always 'dark'
```
Users with a light-mode system preference still see the login/register page in dark mode until they click the theme toggle.  
**Fix:** Change `'dark' : 'dark'` → `'dark' : 'light'` on line 13; change `fallback = 'dark'` → `fallback = 'light'` on line 78.

---

### BUG-2 · Wiki Edit page has wrong browser tab title
**File:** `templates/wiki_edit.html` line 1  
**What happens:**
```
{{define "title"}}wiki{{end}}
```
The browser tab always shows "wiki" regardless of whether the user is creating or editing an article.  
**Fix:** Change to:
```
{{define "title"}}{{if .Article}}Edit: {{.Article.Title}} · Sample Tracker{{else}}New Article · Sample Tracker{{end}}{{end}}
```

---

### BUG-3 · CSS and JS loaded twice on wiki edit page
**Files:** `templates/base.html` (lines 23-24, 29) and `templates/wiki_edit.html` (`additional_styles` block)  
**What happens:** `base.html` already loads `easymde.min.css`, Font Awesome, and `easymde.min.js` globally. `wiki_edit.html` re-defines `additional_styles` to load the same two stylesheets again:
```html
<!-- base.html always loads: -->
<link rel="stylesheet" href="/static/css/easymde.min.css">
<link rel="stylesheet" href=".../font-awesome/5.15.4/css/all.min.css">
<script src="/static/js/easymde.min.js"></script>

<!-- wiki_edit.html ALSO loads (duplicate): -->
<link rel="stylesheet" href="/static/css/easymde.min.css">
<link rel="stylesheet" href=".../font-awesome/5.15.4/css/all.min.css">
```
Duplicate stylesheet loads can cause style conflicts and waste bandwidth.  
**Fix:** Remove the `additional_styles` block from `wiki_edit.html` since the base already includes these assets (or better yet, move them out of base.html — see BUG-4).

---

### BUG-4 · Wiki Cancel (inline editor) uses a redirect path that can silently break HTMX partial swaps
**File:** `templates/wiki_view.html` — `article_content_panel` block, Cancel button  
**What happens:** The Cancel button inside the editor sends:
```html
hx-get="/wiki/{{.Article.Title}}"
```
The server responds with a **303 redirect** to `/wiki/view/{{.Article.Title}}`. In same-origin requests HTMX headers are usually preserved, but this is fragile — some browsers or proxy configurations strip headers on redirect, causing the full page HTML to be swapped into the partial `#article-content-panel` target instead of just the panel fragment.  
The *outer* Cancel button (article header) correctly uses `/wiki/view/{{.Article.Title}}` directly.  
**Fix:** Change the inner Cancel to use the direct path:
```html
hx-get="/wiki/view/{{.Article.Title}}"
```

---

## 🟠 Performance / Bloat

### PERF-1 · EasyMDE JS (heavy) and Font Awesome CSS loaded on every page
**File:** `templates/base.html` lines 23-24, 29  
`easymde.min.js` (~120 KB) and Font Awesome CSS (~80 KB) are loaded globally on every page — samples list, booking, admin, etc. — even though they are only used on the wiki edit page.  
**Impact:** Significant wasted load time on all non-wiki-edit pages.  
**Fix:** Move these assets into `wiki_edit.html`'s `additional_styles` / `footer_scripts` blocks only.

---

### PERF-2 · `github-markdown-css` loaded twice on wiki view pages
**Files:** `templates/base.html` (line 22) and `templates/wiki_view.html` (`additional_styles` block)  
The markdown stylesheet is loaded globally by `base.html` and again by `wiki_view.html`'s own `additional_styles` definition.

---

### PERF-3 · `gallery.css` loaded twice on wiki view and shared sample pages
**Files:** `templates/base.html` (line 27), `templates/wiki_view.html` and `templates/sample_shared.html` (`additional_styles` blocks)  
`gallery.css` is already included globally; wiki_view and sample_shared re-add it unnecessarily.

---

## 🟡 Inconsistency / UX Issues

### UX-1 · Revoke Token button uses non-standard CSS classes
**File:** `templates/profile_tokens.html` line 59  
```html
<button class="button button--danger button--sm" ...>Revoke</button>
```
The rest of the app uses `button--destructive` and `button--small`. The classes `button--danger` and `button--sm` are only defined inline in this template's `<style>` block, so they work visually but are inconsistent and won't respond to future global button style changes.  
**Fix:** Replace with `button--destructive button--small`.

---

### UX-2 · Report Modal form inputs have no `name` attributes
**File:** `templates/admin.html` (Report Modal, lines 101-108)  
```html
<input type="date" id="start_date" required>
<input type="date" id="end_date" required>
```
The `Download` button fires JavaScript rather than submitting the form, and both inputs lack `name` attributes. This means:
- The form is not keyboard/screen-reader accessible without JS.
- The HTML `required` attribute on nameless inputs may not validate correctly in all browsers.  
**Fix:** Add `name="start_date"` and `name="end_date"` for accessibility, even if JS continues to drive the download action.

---

### UX-3 · `/agents` and `/agents/examples` pages are not linked from the sidebar
**File:** `templates/header.html`  
The AI Agents guide (`/agents`, `/agents/examples`) is a publicly accessible section of the app but has no entry in the sidebar navigation. Users can only find it by knowing the URL.  
**Fix:** Add a sidebar link to `/agents` under the Workspace group, or at least in the footer.

---

### UX-4 · Equipment "Rules" link on Booking page is permanently commented out
**File:** `templates/booking.html` lines 61-63  
```html
<!-- <div class="equipment-meta">
    <a class="equipment-link" href="#" aria-disabled="true">Rules</a>
</div> -->
```
The feature is stubbed but never shipped. If it won't be implemented soon, the comment should be cleaned up to reduce confusion.

---

### UX-5 · Wiki attachment upload (non-HTMX fallback) redirects to `/wiki` list instead of the article
**File:** `templates/wiki_view.html` — upload form (`action="/wiki/upload/{{.Article.ID}}"`)  
If the upload is submitted as a standard form POST (no JS / HTMX), the server redirects to the Referer URL. If the Referer header is absent (stripped by a browser, privacy extension, or direct API call), the user is sent to `/wiki` (the article list) instead of back to their article. A minor UX surprise.  
**Fix:** Append the article title to the redirect fallback URL in `wiki/handlers.go → uploadAttachment()`.

---

## ℹ️ Minor / Cosmetic

### INFO-1 · Sample detail "Share Link" and "Edit Sample Details" panels are collapsed by default
**File:** `templates/sample_detail.html` (lines 49–57)  
```html
<details class="panel-details">
    <summary>Share Link</summary>
    ...
<details class="panel-details">
    <summary>Edit Sample Details</summary>
```
These `<details>` panels are closed on page load. New users may not discover editing or sharing features.  
**Suggestion:** Consider opening them by default (`<details open>`) or providing a more prominent edit button in the page header.

---

### INFO-2 · Login/Register "Request access" link goes to `/register`, but registration is approval-based
**File:** `templates/login.html` line 220  
The copy says "Request access" but the route is `/register`. Registered users must wait for an admin to approve them, which is not communicated until after submission. The success message only appears after the form is submitted.  
**Suggestion:** Add a note on the login page explaining the approval workflow before users register.

---

### INFO-3 · `base.html` loads `easymde.min.js` as a synchronous `<script>` (blocking)
**File:** `templates/base.html` line 29  
```html
<script src="/static/js/easymde.min.js"></script>
```
No `defer` or `async` attribute — this blocks HTML parsing until the script downloads and executes on every page, even those that don't use the editor.  
**Fix:** Add `defer` to the script tag (or better yet, move it to only wiki_edit pages per PERF-1).

---

## All Routes & Buttons — Status Matrix

| Page / Route | Buttons / Links | Status |
|---|---|---|
| `/login` | Login button, Theme toggle, "Request access" link | ✅ OK |
| `/register` | Register button, Theme toggle, "Sign in" link | ✅ OK |
| `/` (Samples list) | New Sample, Search bar, Search button, Date filters, Sample cards | ✅ OK |
| `/samples/new` | Create Sample, Back links | ✅ OK |
| `/samples/{id}` | Upload attachment, Gallery thumb (lightbox), Delete attachment, Add Step, Edit Step, Delete Step, Save Changes, Delete Sample (admin), Generate share link, Revoke share link, Update visibility | ✅ OK |
| `/wiki` | New Article, Article tile links | ✅ OK |
| `/wiki/new` | Create Article, Cancel, EasyMDE toolbar buttons, Insert Image picker | ✅ OK |
| `/wiki/view/{title}` | Edit button, Cancel button (outer), Delete article, Save (inline), Cancel (inline — see BUG-4), Upload attachment, Delete attachment | ⚠️ BUG-4 on inner Cancel |
| `/booking` | Prev Day, Next Day, Find (date picker), Timeline lane cells (open modal), Book, Cancel modal, Close (×) modal, Delete own booking block | ✅ OK |
| `/admin` | Add Equipment button, Usage button, Remove button, Report Modal (Download, Cancel), Add Group, Remove Group, Group select (auto-save), Approved toggle (auto-save), Equipment multi-select (auto-save), Make Admin / Remove Admin, Reset Password, Remove User | ⚠️ Report Modal inputs lack `name` attrs (UX-2) |
| `/change-password` | Update Password, Cancel (→ `/`) | ✅ OK |
| `/profile/tokens` | Generate token, Revoke token | ⚠️ UX-1 (class name inconsistency, functionally OK) |
| `/logout` | Logout icon link | ✅ OK |
| `/agents` | Theme toggle, anchor links in sidebar, "View examples" link | ✅ OK |
| `/agents/examples` | Theme toggle, "Back to AI Assistants Guide" link, tab buttons | ✅ OK |
| `/s/{token}` (shared view) | Gallery thumbs, Download file, "Open full view" (auth only) | ✅ OK |
| Sidebar (global) | Brand logo, New Sample, Samples, Wiki, Equipment, API Tokens, Change Password, Admin Panel, Theme toggle, Logout | ✅ OK (agents page not linked — UX-3) |
