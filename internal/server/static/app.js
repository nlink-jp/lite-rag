/* lite-rag Web UI */
(function () {
  'use strict';

  // ── DOM refs ──────────────────────────────────────────────────────────────
  const textarea    = document.getElementById('query');
  const submitBtn   = document.getElementById('submit');
  const statusBar   = document.getElementById('status');
  const answerPanel = document.getElementById('answer-panel');
  const answerBody  = document.getElementById('answer-body');
  const answerRaw   = document.getElementById('answer-raw');
  const sourcesDiv  = document.getElementById('sources');
  const btnRendered = document.getElementById('btn-rendered');
  const btnRaw      = document.getElementById('btn-raw');

  let currentView = 'rendered';

  // Prevent raw HTML blocks in LLM output from being injected into the DOM.
  // marked v15 removed the built-in sanitize option; we override the html
  // renderer to escape angle brackets instead of passing them through.
  marked.use({
    renderer: {
      html({ text }) { return escHtml(text); },
    },
  });

  btnRendered.addEventListener('click', () => setView('rendered'));
  btnRaw.addEventListener('click',      () => setView('raw'));

  function setView(view) {
    currentView = view;
    btnRendered.classList.toggle('active', view === 'rendered');
    btnRaw.classList.toggle('active',      view === 'raw');
    answerBody.hidden = (view !== 'rendered');
    answerRaw.hidden  = (view !== 'raw');
  }

  // ── Version fetch ─────────────────────────────────────────────────────────
  fetch('/api/status')
    .then(r => r.json())
    .then(d => {
      const el = document.getElementById('version');
      if (el && d.version) el.textContent = 'v' + d.version;
    })
    .catch(() => {});

  // ── Submit on Enter (Shift+Enter = newline) ───────────────────────────────
  textarea.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submitBtn.click();
    }
  });

  // ── Main ask handler ──────────────────────────────────────────────────────
  submitBtn.addEventListener('click', () => {
    const query = textarea.value.trim();
    if (!query) return;

    // Reset UI
    setStatus('<span class="spinner"></span>Searching…');
    statusBar.className = 'status-bar';
    answerPanel.classList.remove('visible');
    answerBody.innerHTML = '';
    answerRaw.textContent = '';
    sourcesDiv.innerHTML = '';
    submitBtn.disabled = true;

    let tokenBuffer = '';
    let sourcesReceived = false;

    const fetchOptions = {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query }),
    };

    fetch('/api/ask', fetchOptions)
      .then(response => {
        if (!response.ok) {
          return response.text().then(t => { throw new Error(t); });
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buf = '';

        function processSSE(text) {
          buf += text;
          const blocks = buf.split('\n\n');
          buf = blocks.pop(); // last (possibly incomplete) block stays in buffer

          for (const block of blocks) {
            if (!block.trim()) continue;

            let eventType = 'message';
            let dataLines = [];

            for (const line of block.split('\n')) {
              if (line.startsWith('event:')) {
                eventType = line.slice(6).trim();
              } else if (line.startsWith('data:')) {
                dataLines.push(line.slice(5).trim());
              }
            }

            const data = dataLines.join('\n');

            if (eventType === 'token') {
              try {
                tokenBuffer += JSON.parse(data);
              } catch (_) {
                tokenBuffer += data; // fallback for non-JSON fragments
              }
              showAnswer(tokenBuffer);
            } else if (eventType === 'sources') {
              try {
                const sources = JSON.parse(data);
                renderSources(sources);
                sourcesReceived = true;
              } catch (_) {}
            } else if (eventType === 'error') {
              try {
                const err = JSON.parse(data);
                showAnswer(err.message || 'Unknown error');
              } catch (_) {
                showAnswer(data);
              }
            } else if (eventType === 'done') {
              // finalize
            }
          }
        }

        function pump() {
          return reader.read().then(({ done, value }) => {
            if (done) return;
            processSSE(decoder.decode(value, { stream: true }));
            return pump();
          });
        }

        return pump();
      })
      .then(() => {
        if (!sourcesReceived) {
          sourcesDiv.innerHTML = '';
        }
        setStatus('');
        submitBtn.disabled = false;
      })
      .catch(err => {
        setStatus('Error: ' + err.message, true);
        submitBtn.disabled = false;
      });
  });

  // ── Helpers ───────────────────────────────────────────────────────────────
  function showAnswer(text) {
    answerPanel.classList.add('visible');
    answerRaw.textContent = text;
    answerBody.innerHTML = marked.parse(text);
  }

  function renderSources(sources) {
    if (!sources || sources.length === 0) return;
    let html = '<div class="sources-label">Sources</div>';
    sources.forEach((s, i) => {
      const scoreClass = s.score >= 0.80 ? 'score-high' : s.score >= 0.60 ? 'score-mid' : 'score-low';
      html += `<div class="source-item">
        <span class="num">${i + 1}.</span>
        <span class="filepath">${escHtml(s.file_path)}</span>
        ${s.heading_path ? `<span class="heading">— ${escHtml(s.heading_path)}</span>` : ''}
        <span class="score ${scoreClass}">${s.score.toFixed(3)}</span>
      </div>`;
    });
    sourcesDiv.innerHTML = html;
  }

  function setStatus(html, warn) {
    statusBar.innerHTML = html;
    statusBar.className = 'status-bar' + (warn ? ' warn' : '');
  }

  function escHtml(str) {
    return str.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }
})();
