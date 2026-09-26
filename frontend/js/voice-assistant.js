// voice-assistant.js - Multilingual Voice Assistant & Description Voice Input
// Supports: English (en-IN), Hindi (hi-IN), Tamil (ta-IN), Telugu (te-IN)

(function () {
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;

  const SUPPORTED_LANGUAGES = [
    { code: 'en-IN', name: 'English (IN)', native: 'English', badge: 'EN' },
    { code: 'hi-IN', name: 'Hindi', native: 'हिंदी', badge: 'HI' },
    { code: 'ta-IN', name: 'Tamil', native: 'தமிழ்', badge: 'TA' },
    { code: 'te-IN', name: 'Telugu', native: 'తెలుగు', badge: 'TE' }
  ];

  const SUGGESTION_CHIPS = {
    'en-IN': [
      "Workers present today?",
      "Today's coal production?",
      "Active violations count?",
      "Critical alerts summary"
    ],
    'hi-IN': [
      "आज कितने श्रमिक उपस्थित हैं?",
      "आज का कोयला उत्पादन कितना है?",
      "सक्रिय उल्लंघन कितने हैं?",
      "महत्वपूर्ण अलर्ट सारांश"
    ],
    'ta-IN': [
      "இன்றைய பணியாளர் வருகை?",
      "இன்றைய நிலக்கரி உற்பத்தி?",
      "செயலில் உள்ள மீறல்கள்?"
    ],
    'te-IN': [
      "నేడు ఎంతమంది కార్మికులు హాజరయ్యారు?",
      "నేటి బొగ్గు ఉత్పత్తి ఎంత?",
      "క్రియాశీల ఉల్లంఘనలు ఎన్ని?"
    ]
  };

  function getSavedLanguage() {
    const saved = localStorage.getItem('cg_voice_lang');
    if (saved && SUPPORTED_LANGUAGES.some(l => l.code === saved)) {
      return saved;
    }
    if (saved === 'en-US') return 'en-IN';
    return 'en-IN';
  }

  function setSavedLanguage(langCode) {
    localStorage.setItem('cg_voice_lang', langCode);
  }

  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  // Safe Text-to-Speech playback for AI voice assistant responses
  function speakResponse(text, langCode) {
    if (!('speechSynthesis' in window)) return;
    try {
      window.speechSynthesis.cancel();
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.lang = langCode;

      const voices = window.speechSynthesis.getVoices();
      if (voices && voices.length > 0) {
        const target = langCode.toLowerCase();
        const prefix = target.split('-')[0];
        const match = voices.find(v => v.lang.toLowerCase() === target || v.lang.toLowerCase().startsWith(prefix));
        if (match) utterance.voice = match;
      }

      utterance.onerror = (e) => {
        console.warn(`[TTS] Voice output unavailable for ${langCode}:`, e);
      };

      window.speechSynthesis.speak(utterance);
    } catch (err) {
      console.warn("[TTS] Speech synthesis error:", err);
    }
  }

  // Render quick suggestion chips based on active language
  function renderSuggestionChips(langCode) {
    const chipsContainer = document.getElementById('voice-quick-chips');
    if (!chipsContainer) return;
    const chips = SUGGESTION_CHIPS[langCode] || SUGGESTION_CHIPS['en-IN'] || [];
    chipsContainer.innerHTML = chips.map(chip => `
      <button type="button" class="voice-chip-btn" data-query="${escapeHtml(chip)}">
        ${escapeHtml(chip)}
      </button>
    `).join('');

    chipsContainer.querySelectorAll('.voice-chip-btn').forEach(btn => {
      btn.onclick = () => {
        const q = btn.getAttribute('data-query');
        submitAssistantQuery(q, langCode);
      };
    });
  }

  function updateLangBadge(langCode) {
    const badge = document.getElementById('voice-panel-lang-badge');
    if (!badge) return;
    const item = SUPPORTED_LANGUAGES.find(l => l.code === langCode);
    badge.textContent = item ? item.badge : 'EN';
  }

  function showAssistantGreeting(langCode) {
    const aiResponseText = document.getElementById('voice-response-text');
    if (!aiResponseText) return;

    let greeting = "Hello! Ask me anything about coal production, attendance, safety incidents, or compliance violations.";
    if (langCode === 'hi-IN') {
      greeting = "नमस्ते! कोयला उत्पादन, उपस्थिति, सुरक्षा घटनाओं या उल्लंघनों के बारे में कुछ भी पूछें।";
    } else if (langCode === 'ta-IN') {
      greeting = "வணக்கம்! நிலக்கரி உற்பத்தி, வருகை, பாதுகாப்பு சம்பவங்கள் அல்லது விதிமீறல்கள் பற்றி என்னிடம் கேளுங்கள்.";
    } else if (langCode === 'te-IN') {
      greeting = "నమస్కారం! బొగ్గు ఉత్పత్తి, హాజరు, భద్రతా ప్రమాదాలు లేదా నిబంధనల ఉల్లంఘనల గురించి ఏదైనా అడగండి.";
    }

    aiResponseText.innerHTML = `
      <div style="font-size: 13px; color: var(--color-ink, #1f2937); line-height: 1.5; margin-bottom: 6px;">
        ${greeting}
      </div>
      <div style="font-size: 11.5px; color: var(--color-ink-muted, #6b7280);">
        Click a quick query chip below, speak via the mic, or type your query in the box.
      </div>
    `;
    renderSuggestionChips(langCode);
  }

  // Unified Query Submission (Works for both voice transcript & typed text)
  async function submitAssistantQuery(queryText, langCode) {
    if (!queryText || !queryText.trim()) return;
    const cleanQuery = queryText.trim();

    const statusIndicator = document.getElementById('voice-status');
    const aiResponsePanel = document.getElementById('voice-response-panel');
    const aiResponseText = document.getElementById('voice-response-text');
    const queryInput = document.getElementById('voice-query-input');
    const btnSend = document.getElementById('btn-voice-query-send');

    if (queryInput) queryInput.value = '';
    if (aiResponsePanel) aiResponsePanel.classList.remove('hidden');

    if (statusIndicator) statusIndicator.textContent = "Thinking...";
    if (aiResponseText) {
      aiResponseText.innerHTML = `
        <div style="display:flex; align-items:center; gap:8px; color:var(--color-ink-muted, #6b7280); font-size:12.5px;">
          <svg style="animation: spin 1s infinite linear;" xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
          Analyzing query: "<strong>${escapeHtml(cleanQuery)}</strong>"...
        </div>
      `;
    }
    if (btnSend) btnSend.disabled = true;

    try {
      const token = localStorage.getItem('cg_token');
      const API_BASE = (window.APP_CONFIG?.API_BASE_URL || 'http://localhost:8080/api').replace(/\/+$/, '');

      const res = await fetch(`${API_BASE}/ai/voice-query`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {})
        },
        body: JSON.stringify({
          query: cleanQuery,
          language: langCode
        })
      });

      const json = await res.json();

      if (json.success && json.data && json.data.answer) {
        const answer = json.data.answer;
        if (aiResponseText) {
          aiResponseText.innerHTML = `
            <div style="font-size:11.5px; color:var(--color-ink-muted, #6b7280); margin-bottom:6px; border-bottom:1px dashed var(--color-border, #e5e7eb); padding-bottom:4px;">
              Query: "<strong>${escapeHtml(cleanQuery)}</strong>"
            </div>
            <div style="color:var(--color-ink, #1f2937); font-size:13px; line-height:1.55; white-space:pre-wrap;">${escapeHtml(answer)}</div>
          `;
        }
        if (statusIndicator) statusIndicator.textContent = "";

        // Speak answer using TTS when supported
        speakResponse(answer, langCode);
      } else {
        const msg = json.message || "Error processing query.";
        if (aiResponseText) {
          aiResponseText.innerHTML = `
            <div style="color:#b91c1c; font-size:12.5px;">
              ${escapeHtml(msg)}
            </div>
          `;
        }
        if (statusIndicator) statusIndicator.textContent = "Query error";
      }
    } catch (err) {
      console.error("AI Voice Query Error:", err);
      if (aiResponseText) {
        aiResponseText.innerHTML = `
          <div style="color:#b91c1c; font-size:12.5px;">
            Network error reaching AI service. Ensure backend is running.
          </div>
        `;
      }
      if (statusIndicator) statusIndicator.textContent = "Network error";
    } finally {
      if (btnSend) btnSend.disabled = false;
      renderSuggestionChips(langCode);
    }
  }

  // Handle Speech Recognition Errors with clear, actionable diagnostics
  function handleAssistantSpeechError(errorCode, activeLang) {
    const btnMic = document.getElementById('btn-voice-mic');
    if (btnMic) btnMic.classList.remove('recording');

    const statusIndicator = document.getElementById('voice-status');
    const aiResponsePanel = document.getElementById('voice-response-panel');
    const aiResponseText = document.getElementById('voice-response-text');

    let title = "Voice Input Notice";
    let messageHtml = "";
    let statusText = "Speech error";

    if (errorCode === 'audio-capture') {
      statusText = "Microphone unavailable";
      title = "Microphone Access Denied (audio-capture)";
      messageHtml = `
        <div style="background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; padding: 10px; margin-bottom: 8px;">
          <div style="font-weight: 600; color: #b91c1c; font-size: 13px; margin-bottom: 4px; display:flex; align-items:center; gap:6px;">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
            Microphone Capture Blocked
          </div>
          <div style="font-size: 12px; color: #7f1d1d; line-height: 1.45;">
            The browser could not capture audio from your microphone.
          </div>
        </div>
        <div style="font-size: 12px; color: #374151; line-height: 1.5; margin-bottom: 10px;">
          <strong>Quick Fix in Windows:</strong>
          <ol style="margin: 4px 0 0 16px; padding: 0;">
            <li>Press <kbd style="background:#f3f4f6; padding:1px 5px; border-radius:3px; border:1px solid #d1d5db; font-family:monospace;">Win + R</kbd>, type <code>ms-settings:privacy-microphone</code> and hit Enter.</li>
            <li>Turn <b>Microphone access</b> to <b>ON</b>.</li>
            <li>Ensure <b>"Let desktop apps access your microphone"</b> is <b>ON</b>.</li>
            <li>Check that your microphone or headset is connected and not muted.</li>
          </ol>
        </div>
        <div style="background: #f0fdf4; border: 1px solid #bbf7d0; border-radius: 6px; padding: 8px 10px; font-size: 12px; color: #166534;">
          ✨ <b>Text Fallback:</b> You can type your query in the box below and press Send!
        </div>
      `;
    } else if (errorCode === 'not-allowed' || errorCode === 'service-not-allowed') {
      statusText = "Mic permission blocked";
      title = "Microphone Permission Blocked";
      messageHtml = `
        <div style="font-size: 12.5px; color: #374151; line-height: 1.45; margin-bottom: 8px;">
          Microphone access was denied by your browser. Click the site settings or tune/lock icon in your browser address bar next to the URL, set <b>Microphone</b> to <b>Allow</b>, and reload the page.
        </div>
        <div style="font-size: 12px; color: #166534; background:#f0fdf4; padding:6px 10px; border-radius:6px; border:1px solid #bbf7d0;">
          💡 You can also type your question directly in the text box below.
        </div>
      `;
    } else if (errorCode === 'no-speech') {
      statusText = "No voice heard";
      title = "No Speech Detected";
      messageHtml = `
        <div style="font-size: 12.5px; color: #4b5563; line-height: 1.45; margin-bottom: 6px;">
          No voice was detected before the timeout. Please speak closer to your microphone or type your question in the text box below.
        </div>
      `;
    } else if (errorCode === 'network') {
      statusText = "Network error";
      title = "Speech Recognition Network Error";
      messageHtml = `
        <div style="font-size: 12.5px; color: #4b5563; line-height: 1.45; margin-bottom: 6px;">
          The browser speech recognition service could not be reached. Please check your internet connection or use the text box below.
        </div>
      `;
    } else if (errorCode === 'aborted') {
      if (statusIndicator) statusIndicator.textContent = "";
      return;
    } else {
      statusText = `Error: ${errorCode}`;
      title = "Voice Input Error";
      messageHtml = `
        <div style="font-size: 12.5px; color: #4b5563; line-height: 1.45; margin-bottom: 6px;">
          Voice recognition error: <code>${escapeHtml(errorCode)}</code>. You can type your query in the box below.
        </div>
      `;
    }

    if (statusIndicator) statusIndicator.textContent = statusText;

    if (aiResponsePanel && aiResponseText) {
      aiResponseText.innerHTML = messageHtml;
      aiResponsePanel.classList.remove('hidden');

      // Autofocus the text query input so user can type immediately
      const queryInput = document.getElementById('voice-query-input');
      if (queryInput) {
        setTimeout(() => queryInput.focus(), 120);
      }
    }

    if (window.showToast && errorCode !== 'no-speech') {
      window.showToast(title, 'warning');
    }
  }

  // Ensure bottom-right floating AI assistant UI exists on the page
  function ensureFloatingUI() {
    let ui = document.getElementById('voice-assistant-ui');
    if (!ui) {
      ui = document.createElement('div');
      ui.id = 'voice-assistant-ui';
      ui.style.cssText = 'position: fixed; bottom: 20px; right: 20px; z-index: 1000; display: flex; flex-direction: column; align-items: flex-end; gap: 10px; font-family: inherit;';
      const botIcon = window.ICONS ? ICONS.get('bot') : '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect width="16" height="12" x="4" y="8" rx="2"/></svg>';
      const micIcon = window.ICONS ? ICONS.get('mic') : '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/></svg>';
      
      ui.innerHTML = `
        <div id="voice-response-panel" class="card hidden" style="background: var(--color-surface, #fff); padding: 14px; width: 340px; border-left: 4px solid var(--color-primary, #1e40af); box-shadow: 0 10px 30px rgba(0,0,0,0.18); border-radius: 10px; z-index: 1001; transition: width 0.25s ease;">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; border-bottom: 1px solid var(--color-border, #f1f5f9); padding-bottom: 6px;">
            <div style="font-weight: 600; font-size: 13.5px; color: var(--color-ink, #1f2937); display:flex; align-items:center; gap:6px;">
              ${botIcon} AI Assistant
              <span id="voice-panel-lang-badge" style="font-size: 10px; font-weight: 600; background: #eff6ff; color: #1e40af; padding: 2px 7px; border-radius: 10px; border: 1px solid #bfdbfe;">EN</span>
            </div>
            <div style="display:flex; align-items:center; gap:6px;">
              <button type="button" id="voice-panel-expand-btn" title="Toggle larger view" style="background:none; border:none; font-size:13px; cursor:pointer; color:var(--color-ink-muted, #6b7280); line-height:1; padding:2px;">⤢</button>
              <button type="button" id="voice-response-close" title="Close" style="background:none; border:none; font-size:18px; cursor:pointer; color:var(--color-ink-muted, #6b7280); line-height:1; padding:2px;">&times;</button>
            </div>
          </div>
          
          <div id="voice-response-text" style="font-size: 13px; line-height: 1.5; color: var(--color-ink, #1f2937); max-height: 220px; overflow-y: auto; margin-bottom: 10px; padding-bottom: 4px;"></div>
          
          <!-- Quick Suggestion Chips -->
          <div id="voice-quick-chips" style="display: flex; flex-wrap: wrap; gap: 5px; margin-bottom: 10px;"></div>

          <!-- Text Query Fallback Input -->
          <form id="voice-text-form" style="display: flex; gap: 6px; margin: 0; align-items: center;">
            <input type="text" id="voice-query-input" placeholder="Ask AI a question or click mic..." autocomplete="off" style="flex: 1; padding: 7px 11px; font-size: 12.5px; border: 1px solid var(--color-border, #d1d5db); border-radius: 6px; outline: none; background: var(--color-bg, #fff); color: var(--color-ink, #1f2937);" />
            <button type="submit" id="btn-voice-query-send" class="btn btn-primary" title="Send Query" style="padding: 7px 12px; font-size: 12px; height: 33px; display: inline-flex; align-items: center; justify-content: center; gap: 4px; border-radius: 6px; cursor: pointer; white-space: nowrap;">
              <span>Send</span>
            </button>
          </form>
        </div>

        <!-- Floating Control Pill Bar -->
        <div style="display: flex; align-items: center; gap: 8px; background: white; padding: 6px 12px; border-radius: 30px; box-shadow: 0 4px 20px rgba(0,0,0,0.15); border: 1px solid var(--color-border, #e5e7eb);">
          <select id="voice-lang-select" title="Change Assistant Language" style="border: none; background: transparent; font-size: 12.5px; font-weight: 500; outline: none; cursor: pointer; color: var(--color-ink, #374151);">
          </select>
          <div id="voice-status" style="font-size: 12px; color: var(--color-ink-muted, #6b7280); max-width: 130px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"></div>
          
          <button id="btn-voice-toggle-panel" type="button" title="Type query / Open AI Chat" style="border:none; background: #f3f4f6; color: #4b5563; border-radius: 50%; width: 34px; height: 34px; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: all 0.2s;">
            <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="16" x="2" y="4" rx="2"/><path d="M6 8h.001"/><path d="M10 8h.001"/><path d="M14 8h.001"/><path d="M18 8h.001"/><path d="M8 12h.001"/><path d="M12 12h.001"/><path d="M16 12h.001"/><path d="M7 16h10"/></svg>
          </button>

          <button id="btn-voice-mic" class="btn btn-primary" title="Click to speak (Voice Recognition)" style="border-radius: 50%; width: 38px; height: 38px; padding: 0; display: flex; align-items: center; justify-content: center; font-size: 14px; cursor: pointer;">
            ${micIcon}
          </button>
        </div>
      `;
      document.body.appendChild(ui);
    }

    // Populate / update language select options
    const langSelect = document.getElementById('voice-lang-select');
    if (langSelect) {
      const currentVal = getSavedLanguage();
      langSelect.innerHTML = SUPPORTED_LANGUAGES.map(l => 
        `<option value="${l.code}" ${l.code === currentVal ? 'selected' : ''}>${l.native} (${l.name.split(' ')[0]})</option>`
      ).join('');

      langSelect.value = currentVal;
      updateLangBadge(currentVal);
      renderSuggestionChips(currentVal);

      langSelect.onchange = () => {
        setSavedLanguage(langSelect.value);
        updateLangBadge(langSelect.value);
        renderSuggestionChips(langSelect.value);
      };
    }

    // Bind close button on response panel
    const closeBtn = document.getElementById('voice-response-close');
    if (closeBtn) {
      closeBtn.onclick = () => {
        const panel = document.getElementById('voice-response-panel');
        if (panel) panel.classList.add('hidden');
      };
    }

    // Bind expand/contract button on response panel
    const expandBtn = document.getElementById('voice-panel-expand-btn');
    if (expandBtn) {
      expandBtn.onclick = () => {
        const panel = document.getElementById('voice-response-panel');
        if (!panel) return;
        panel.classList.toggle('expanded');
        expandBtn.textContent = panel.classList.contains('expanded') ? '⤡' : '⤢';
      };
    }

    // Bind text form submit
    const textForm = document.getElementById('voice-text-form');
    const textInput = document.getElementById('voice-query-input');
    if (textForm && textInput) {
      textForm.onsubmit = (e) => {
        e.preventDefault();
        const val = textInput.value.trim();
        if (!val) return;
        const currentLang = langSelect ? langSelect.value : getSavedLanguage();
        submitAssistantQuery(val, currentLang);
      };
    }

    // Bind toggle panel button
    const btnToggle = document.getElementById('btn-voice-toggle-panel');
    if (btnToggle) {
      btnToggle.onclick = () => {
        const panel = document.getElementById('voice-response-panel');
        if (!panel) return;
        const isHidden = panel.classList.contains('hidden');
        if (isHidden) {
          panel.classList.remove('hidden');
          const respText = document.getElementById('voice-response-text');
          if (respText && !respText.innerHTML.trim()) {
            const currentLang = langSelect ? langSelect.value : getSavedLanguage();
            showAssistantGreeting(currentLang);
          }
          if (textInput) textInput.focus();
        } else {
          panel.classList.add('hidden');
        }
      };
    }
  }

  // =========================================================================
  // 1. AI VOICE ASSISTANT (Bottom-Right Widget)
  // =========================================================================
  let assistantRecognition = null;

  function initAssistant() {
    const btnMic = document.getElementById('btn-voice-mic');
    const langSelect = document.getElementById('voice-lang-select');
    const statusIndicator = document.getElementById('voice-status');
    const aiResponsePanel = document.getElementById('voice-response-panel');

    if (!btnMic || !langSelect) return;

    if (!SpeechRecognition) {
      btnMic.title = "Speech recognition is not supported in this browser. Click to type questions.";
      btnMic.style.opacity = "0.75";
      btnMic.addEventListener('click', () => {
        if (aiResponsePanel) {
          aiResponsePanel.classList.remove('hidden');
          showAssistantGreeting(langSelect.value || getSavedLanguage());
          const input = document.getElementById('voice-query-input');
          if (input) input.focus();
        }
      });
      return;
    }

    btnMic.addEventListener('click', async () => {
      // Toggle stop if already recording
      if (btnMic.classList.contains('recording')) {
        if (assistantRecognition) assistantRecognition.stop();
        return;
      }

      const activeLang = langSelect.value || getSavedLanguage();
      setSavedLanguage(activeLang);
      updateLangBadge(activeLang);

      try {
        assistantRecognition = new SpeechRecognition();
      } catch (err) {
        console.warn("Could not instantiate SpeechRecognition:", err);
        handleAssistantSpeechError('audio-capture', activeLang);
        return;
      }

      assistantRecognition.continuous = false;
      assistantRecognition.interimResults = false;
      assistantRecognition.lang = activeLang;

      assistantRecognition.onstart = () => {
        btnMic.classList.add('recording');
        if (statusIndicator) statusIndicator.textContent = "Listening...";
      };

      assistantRecognition.onresult = async (event) => {
        const transcript = event.results[0][0].transcript;
        if (statusIndicator) statusIndicator.textContent = `Heard: "${transcript.substring(0, 18)}..."`;
        btnMic.classList.remove('recording');
        submitAssistantQuery(transcript, activeLang);
      };

      assistantRecognition.onerror = (event) => {
        console.warn("Assistant recognition error:", event.error);
        btnMic.classList.remove('recording');
        handleAssistantSpeechError(event.error, activeLang);
      };

      assistantRecognition.onend = () => {
        btnMic.classList.remove('recording');
      };

      try {
        assistantRecognition.start();
      } catch (e) {
        console.warn("Could not start recognition:", e);
        handleAssistantSpeechError('audio-capture', activeLang);
      }
    });
  }

  // =========================================================================
  // 2. MULTILINGUAL VOICE INPUT IN DESCRIPTION / OBSERVATION FIELDS
  // =========================================================================
  let activeDescRecognition = null;
  let activeDescButton = null;

  function handleDescVoiceClick(btn) {
    if (!SpeechRecognition) {
      alert("Web Speech recognition is not supported in this browser. Please use Chrome, Edge, or Safari.");
      return;
    }

    const targetId = btn.getAttribute('data-target');
    const targetEl = document.getElementById(targetId);
    if (!targetEl) {
      console.warn("Target field not found:", targetId);
      return;
    }

    // If currently recording on this button, stop it
    if (btn.classList.contains('recording')) {
      if (activeDescRecognition) {
        activeDescRecognition.stop();
      }
      return;
    }

    // If recording on another button, stop that one first
    if (activeDescRecognition) {
      activeDescRecognition.stop();
    }

    // Read active language from the floating language selector
    const langSelect = document.getElementById('voice-lang-select');
    const selectedLang = langSelect ? langSelect.value : getSavedLanguage();

    // Stop assistant recognition to prevent hardware conflict
    if (assistantRecognition) {
      try { assistantRecognition.abort(); } catch(e) {}
    }

    const langProfile = SUPPORTED_LANGUAGES.find(l => l.code === selectedLang) || SUPPORTED_LANGUAGES[0];

    const recognition = new SpeechRecognition();
    recognition.continuous = true;
    recognition.interimResults = true;
    recognition.lang = selectedLang;

    activeDescRecognition = recognition;
    activeDescButton = btn;

    const initialText = targetEl.value || '';
    const prefix = initialText && !initialText.endsWith(' ') && !initialText.endsWith('\n') ? initialText + ' ' : initialText;
    let accumulatedFinal = '';
    let hasInserted = false;

    recognition.onstart = () => {
      btn.classList.add('recording');
      const micSvg = window.ICONS ? ICONS.get('mic') : '';
      btn.innerHTML = `<span class="mic-icon" style="display:inline-flex; align-items:center;">${micSvg}</span> <span class="mic-status-text">Listening (${langProfile.native})... Click to Stop</span>`;
    };

    recognition.onresult = (event) => {
      let interim = '';
      for (let i = event.resultIndex; i < event.results.length; ++i) {
        const item = event.results[i];
        if (item.isFinal) {
          accumulatedFinal += item[0].transcript + ' ';
        } else {
          interim += item[0].transcript;
        }
      }

      const liveText = (accumulatedFinal + interim).trim();
      if (liveText) {
        targetEl.value = prefix + liveText;
        targetEl.dispatchEvent(new Event('input', { bubbles: true }));
        targetEl.scrollTop = targetEl.scrollHeight;
        hasInserted = true;
      }
    };

    recognition.onerror = (event) => {
      console.warn("Description voice input error:", event.error);
      const notify = window.showToast ? (msg, type) => window.showToast(msg, type) : (msg) => alert(msg);
      if (event.error === 'not-allowed' || event.error === 'service-not-allowed') {
        notify("Microphone access blocked. Click the lock/tune icon in your address bar to allow microphone.", "error");
      } else if (event.error === 'audio-capture') {
        notify("Microphone capture failed. Windows Privacy or mic settings may be blocking audio. You can type directly in the field.", "error");
        if (targetEl) targetEl.focus();
      } else if (event.error === 'network') {
        notify("Speech service connection error. Please check your network.", "error");
      } else if (event.error === 'no-speech') {
        notify("No voice detected. Please speak closer to your microphone.", "warning");
      }
      resetDescButton(btn);
    };

    recognition.onend = () => {
      if (hasInserted) {
        targetEl.value = (prefix + accumulatedFinal).trim();
        targetEl.dispatchEvent(new Event('input', { bubbles: true }));
        targetEl.dispatchEvent(new Event('change', { bubbles: true }));
        const checkSvg = window.ICONS ? ICONS.get('check') : '';
        btn.innerHTML = `<span class="mic-icon" style="display:inline-flex; align-items:center;">${checkSvg}</span> <span class="mic-status-text">Inserted!</span>`;
        setTimeout(() => resetDescButton(btn), 1200);
      } else {
        resetDescButton(btn);
      }
    };

    try {
      recognition.start();
    } catch (err) {
      console.warn("Error starting description speech recognition:", err);
      resetDescButton(btn);
    }
  }

  function resetDescButton(btn) {
    if (!btn) return;
    btn.classList.remove('recording');
    const micSvg = window.ICONS ? ICONS.get('mic') : '';
    btn.innerHTML = `<span class="mic-icon" style="display:inline-flex; align-items:center;">${micSvg}</span> <span class="mic-status-text">Voice Input</span>`;
    if (activeDescRecognition) {
      try { activeDescRecognition.stop(); } catch(e) {}
      activeDescRecognition = null;
      activeDescButton = null;
    }
  }

  // Delegated click listener for all Description/Observation mic buttons
  document.addEventListener('click', (e) => {
    const btn = e.target.closest('.btn-desc-mic');
    if (btn) {
      e.preventDefault();
      handleDescVoiceClick(btn);
      return;
    }

    // Delegated click listener for Translate buttons
    const transBtn = e.target.closest('.btn-desc-translate');
    if (transBtn) {
      e.preventDefault();
      handleTranslateClick(transBtn);
    }
  });

  async function handleTranslateClick(transBtn) {
    const targetId = transBtn.getAttribute('data-target');
    const targetEl = document.getElementById(targetId);
    if (!targetEl || !targetEl.value.trim()) {
      const notify = window.showToast ? (msg, type) => window.showToast(msg, type) : (msg) => alert(msg);
      notify("Please speak or type text into the field first before translating.", "warning");
      return;
    }

    const origHtml = transBtn.innerHTML;
    transBtn.disabled = true;
    const refreshSvg = window.ICONS ? ICONS.get('refresh') : '';
    transBtn.innerHTML = `<span class="trans-icon" style="display:inline-flex; align-items:center;">${refreshSvg}</span> Translating...`;

    try {
      const token = localStorage.getItem('cg_token');
      const API_BASE = (window.APP_CONFIG?.API_BASE_URL || 'http://localhost:8080/api').replace(/\/+$/, '');
      const res = await fetch(`${API_BASE}/ai/translate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': token ? `Bearer ${token}` : ''
        },
        body: JSON.stringify({
          text: targetEl.value.trim(),
          target_language: 'English'
        })
      });

      const data = await res.json();
      if (data.success && data.data && data.data.translated_text) {
        targetEl.value = data.data.translated_text;
        targetEl.dispatchEvent(new Event('input', { bubbles: true }));
        targetEl.dispatchEvent(new Event('change', { bubbles: true }));
        if (window.showToast) window.showToast("Translated to English successfully!", "success");
      } else {
        if (window.showToast) window.showToast("Translation failed. Keeping original text.", "error");
      }
    } catch (err) {
      if (window.showToast) window.showToast("Cannot reach translation service.", "error");
    } finally {
      transBtn.disabled = false;
      transBtn.innerHTML = origHtml;
    }
  }

  // Auto inject Translate button next to any .btn-desc-mic if not already there
  function ensureTranslateButtons() {
    document.querySelectorAll('.btn-desc-mic').forEach(micBtn => {
      const targetId = micBtn.getAttribute('data-target');
      if (!targetId) return;
      const parent = micBtn.parentElement;
      if (parent && !parent.querySelector(`.btn-desc-translate[data-target="${targetId}"]`)) {
        const transBtn = document.createElement('button');
        transBtn.type = 'button';
        transBtn.className = 'btn-desc-translate';
        transBtn.setAttribute('data-target', targetId);
        transBtn.setAttribute('title', 'Translate this text to English');
        const globeSvg = window.ICONS ? ICONS.get('globe') : '';
        transBtn.innerHTML = `<span class="trans-icon" style="display:inline-flex; align-items:center;">${globeSvg}</span> <span class="trans-text">Translate to EN</span>`;
        micBtn.after(transBtn);
      }
    });
  }

  // Pre-load voices for TTS if available
  if ('speechSynthesis' in window) {
    window.speechSynthesis.onvoiceschanged = () => {
      window.speechSynthesis.getVoices();
    };
  }

  // Initialize once DOM is ready
  function init() {
    ensureFloatingUI();
    initAssistant();
    ensureTranslateButtons();

    // Auto-inject Translate button whenever a modal or dynamic form is opened
    const observer = new MutationObserver(() => {
      ensureTranslateButtons();
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
